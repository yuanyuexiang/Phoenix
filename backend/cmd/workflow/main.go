// 工作流引擎服务(说明书 §7):编排文档处理流水线,持有 PostgreSQL 与 MinIO。
// MCP Server 与管理后台都通过本服务的 REST API 驱动流程:
//
//	POST /api/documents                上传(content_text/base64/file_url 三选一)
//	POST /api/documents/{id}/extract   OCR/解析 + AI 字段提取
//	POST /api/documents/{id}/validate  规则校验
//	POST /api/documents/{id}/save      确认入库(可带人工修正 fields / force)
//	GET  /api/documents                查询(doc_type/status/keyword/limit)
//	GET  /api/doctypes                 单据类型配置(管理后台用)
package main

import (
	"context"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/yuanyuexiang/phoenix/internal/api"
	"github.com/yuanyuexiang/phoenix/internal/clients"
	"github.com/yuanyuexiang/phoenix/internal/config"
	"github.com/yuanyuexiang/phoenix/internal/ocr"
	"github.com/yuanyuexiang/phoenix/internal/pipeline"
	"github.com/yuanyuexiang/phoenix/internal/schema"
	"github.com/yuanyuexiang/phoenix/internal/store"
)

const maxFetchSize = 64 << 20 // file_url 下载上限 64MB

func main() {
	if err := run(); err != nil {
		slog.Error("workflow 启动失败", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg := config.Load()
	addr := os.Getenv("PHX_WORKFLOW_ADDR")
	if addr == "" {
		addr = ":8081"
	}
	ctx := context.Background()

	registry, err := schema.Load(cfg.DoctypesDir)
	if err != nil {
		return err
	}
	slog.Info("单据类型已加载", "types", registry.Names())

	db, err := store.Open(ctx, cfg.DatabaseDSN)
	if err != nil {
		return err
	}
	defer db.Close()

	objects, err := store.OpenObjects(ctx, cfg.MinioEndpoint, cfg.MinioAccessKey, cfg.MinioSecretKey, cfg.MinioBucket, cfg.MinioUseSSL)
	if err != nil {
		return err
	}

	p := &pipeline.Pipeline{
		DB:            db,
		Objects:       objects,
		OCR:           ocr.New(cfg.OCRBaseURL),
		Parser:        clients.NewParser(cfg.ParserBaseURL),
		AI:            clients.NewAI(cfg.AIBaseURL),
		Registry:      registry,
		MinConfidence: cfg.MinConfidence,
	}

	s := &server{
		p:        p,
		registry: registry,
		db:       db,
		healthTargets: []healthTarget{ // 服务状态页(管理后台)聚合探测
			{"parser 文档解析", cfg.ParserBaseURL + "/healthz"},
			{"ai 字段提取", cfg.AIBaseURL + "/healthz"},
			{"ocr 识别", cfg.OCRBaseURL + "/healthz"},
		},
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("POST /api/documents", s.upload)
	mux.HandleFunc("POST /api/documents/{id}/extract", s.extract)
	mux.HandleFunc("POST /api/documents/{id}/validate", s.validate)
	mux.HandleFunc("POST /api/documents/{id}/save", s.save)
	mux.HandleFunc("GET /api/documents", s.query)
	mux.HandleFunc("GET /api/doctypes", s.doctypes)
	mux.HandleFunc("GET /api/status", s.status)

	// 鉴权:/api/auth/* 与 /healthz 开放,其余 /api 需要 X-Access-Key(参考 Atlas 的 entry-gate 模式)
	mux.HandleFunc("GET /api/auth/status", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]bool{"required": cfg.AdminPassword != ""})
	})
	mux.HandleFunc("GET /api/auth/check", func(w http.ResponseWriter, r *http.Request) {
		if !keyOK(cfg.AdminPassword, r) {
			writeError(w, http.StatusUnauthorized, "访问密码不正确")
			return
		}
		writeJSON(w, map[string]bool{"ok": true})
	})

	if cfg.AdminPassword == "" {
		slog.Warn("PHX_ADMIN_PASSWORD 为空,workflow API 未启用鉴权(仅限本机联调)")
	} else if cfg.AdminPassword == "phoenix123" {
		slog.Warn("正在使用默认访问密码 phoenix123,生产环境务必通过 PHX_ADMIN_PASSWORD 修改")
	}

	slog.Info("workflow 工作流引擎已启动", "addr", addr)
	return http.ListenAndServe(addr, authMiddleware(cfg.AdminPassword, mux))
}

// keyOK 常量时间比较请求头中的访问密钥。
func keyOK(password string, r *http.Request) bool {
	if password == "" {
		return true
	}
	got := r.Header.Get("X-Access-Key")
	return got != "" && subtle.ConstantTimeCompare([]byte(got), []byte(password)) == 1
}

// authMiddleware 保护除 /healthz 与 /api/auth/* 之外的全部接口。
func authMiddleware(password string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		open := r.URL.Path == "/healthz" || strings.HasPrefix(r.URL.Path, "/api/auth/")
		if open || keyOK(password, r) {
			next.ServeHTTP(w, r)
			return
		}
		writeError(w, http.StatusUnauthorized, "未授权:缺少或错误的访问密钥(X-Access-Key)")
	})
}

type healthTarget struct {
	Name string
	URL  string
}

type server struct {
	p             *pipeline.Pipeline
	registry      *schema.Registry
	db            *store.DB
	healthTargets []healthTarget
}

// status 聚合平台各组件的健康状态,供管理后台「服务状态」页展示。
func (s *server) status(w http.ResponseWriter, r *http.Request) {
	type component struct {
		Name      string `json:"name"`
		OK        bool   `json:"ok"`
		LatencyMS int64  `json:"latency_ms"`
		Error     string `json:"error,omitempty"`
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	check := func(name string, fn func() error) component {
		start := time.Now()
		err := fn()
		c := component{Name: name, OK: err == nil, LatencyMS: time.Since(start).Milliseconds()}
		if err != nil {
			c.Error = err.Error()
		}
		return c
	}

	client := &http.Client{Timeout: 3 * time.Second}
	components := []component{
		check("workflow 工作流引擎", func() error { return nil }),
		check("postgres 数据库", func() error { return s.db.Ping(ctx) }),
	}
	for _, t := range s.healthTargets {
		t := t
		components = append(components, check(t.Name, func() error {
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, t.URL, nil)
			if err != nil {
				return err
			}
			resp, err := client.Do(req)
			if err != nil {
				return fmt.Errorf("不可达: %w", err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				return fmt.Errorf("HTTP %d", resp.StatusCode)
			}
			return nil
		}))
	}
	writeJSON(w, map[string]any{"components": components})
}

func (s *server) upload(w http.ResponseWriter, r *http.Request) {
	var req api.UploadRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "请求体解析失败: "+err.Error())
		return
	}
	data, err := resolveContent(r.Context(), req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	doc, err := s.p.Upload(r.Context(), req.DocType, req.Filename, data)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	writeJSON(w, api.ToView(doc))
}

func (s *server) extract(w http.ResponseWriter, r *http.Request) {
	doc, err := s.p.ExtractFields(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	writeJSON(w, api.ToView(doc))
}

func (s *server) validate(w http.ResponseWriter, r *http.Request) {
	doc, err := s.p.Validate(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	writeJSON(w, api.ToView(doc))
}

func (s *server) save(w http.ResponseWriter, r *http.Request) {
	var req api.SaveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "请求体解析失败: "+err.Error())
		return
	}
	doc, err := s.p.Save(r.Context(), r.PathValue("id"), req.Fields, req.Force)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	writeJSON(w, api.ToView(doc))
}

func (s *server) query(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	docs, err := s.p.Query(r.Context(), store.QueryFilter{
		DocType: q.Get("doc_type"),
		Status:  q.Get("status"),
		Keyword: q.Get("keyword"),
		Limit:   limit,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := api.QueryResult{Total: len(docs), Documents: make([]api.DocumentView, 0, len(docs))}
	for _, d := range docs {
		out.Documents = append(out.Documents, api.ToView(d))
	}
	writeJSON(w, out)
}

func (s *server) doctypes(w http.ResponseWriter, _ *http.Request) {
	types := make([]*schema.DocType, 0)
	for _, name := range s.registry.Names() {
		dt, _ := s.registry.Get(name)
		types = append(types, dt)
	}
	writeJSON(w, types)
}

func resolveContent(ctx context.Context, in api.UploadRequest) ([]byte, error) {
	provided := 0
	for _, ok := range []bool{in.ContentText != "", in.ContentBase64 != "", in.FileURL != ""} {
		if ok {
			provided++
		}
	}
	if provided != 1 {
		return nil, fmt.Errorf("content_text、content_base64、file_url 必须且只能提供一个")
	}
	switch {
	case in.ContentText != "":
		return []byte(in.ContentText), nil
	case in.ContentBase64 != "":
		data, err := base64.StdEncoding.DecodeString(in.ContentBase64)
		if err != nil {
			return nil, fmt.Errorf("content_base64 解码失败: %w", err)
		}
		return data, nil
	default:
		return fetchURL(ctx, in.FileURL)
	}
}

func fetchURL(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	client := &http.Client{Timeout: 2 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("下载 file_url 失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("下载 file_url 失败: HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxFetchSize+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxFetchSize {
		return nil, fmt.Errorf("文件超过 %dMB 上限", maxFetchSize>>20)
	}
	return data, nil
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(api.ErrorResponse{Error: msg})
}
