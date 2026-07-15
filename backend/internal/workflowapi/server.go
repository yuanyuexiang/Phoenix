// Package workflowapi 实现工作流引擎的 REST API 层(handler、鉴权、健康聚合)。
// cmd/workflow 只负责装配依赖并调用 NewHandler;路由一览:
//
//	POST /api/documents                上传(content_text/base64/file_url 三选一)
//	POST /api/documents/{id}/extract   文字识别/解析 + AI 字段提取
//	POST /api/documents/{id}/validate  规则校验
//	POST /api/documents/{id}/save      确认入库(可带人工修正 fields / force)
//	GET  /api/documents                查询(doc_type/status/keyword/uploaded_by/limit)
//	GET  /api/doctypes                 单据类型配置(管理后台用)
//	GET  /api/status                   组件健康聚合(服务状态页用)
//	GET  /api/auth/status|check        鉴权探测(开放)
//	GET  /healthz                      存活探测(开放)
//
// 除标注"开放"外,其余接口要求请求头 X-Access-Key(见 authMiddleware)。
package workflowapi

import (
	"context"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/yuanyuexiang/phoenix/internal/api"
	"github.com/yuanyuexiang/phoenix/internal/identity"
	"github.com/yuanyuexiang/phoenix/internal/pipeline"
	"github.com/yuanyuexiang/phoenix/internal/schema"
	"github.com/yuanyuexiang/phoenix/internal/store"
)

const maxFetchSize = 64 << 20 // file_url 下载上限 64MB

// HealthTarget 是 /api/status 要探测的一个下游服务。
type HealthTarget struct {
	Name string
	URL  string // 完整 healthz 地址
}

// Options 是构建 API 层所需的全部依赖。
type Options struct {
	Pipeline      *pipeline.Pipeline
	Registry      *schema.Registry
	DB            *store.DB
	AdminPassword string // 空 = 关闭鉴权
	HealthTargets []HealthTarget
}

// NewHandler 组装全部路由与鉴权中间件。
func NewHandler(opts Options) http.Handler {
	s := &server{opts: opts}

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

	// 鉴权探测接口(开放,参考 Atlas 的 entry-gate 模式)
	mux.HandleFunc("GET /api/auth/status", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]bool{"required": opts.AdminPassword != ""})
	})
	mux.HandleFunc("GET /api/auth/check", func(w http.ResponseWriter, r *http.Request) {
		if !keyOK(opts.AdminPassword, r) {
			writeError(w, http.StatusUnauthorized, "访问密码不正确")
			return
		}
		writeJSON(w, map[string]bool{"ok": true})
	})

	if opts.AdminPassword == "" {
		slog.Warn("PHX_ADMIN_PASSWORD 为空,workflow API 未启用鉴权(仅限本机联调)")
	} else if opts.AdminPassword == "phoenix123" {
		slog.Warn("正在使用默认访问密码 phoenix123,生产环境务必通过 PHX_ADMIN_PASSWORD 修改")
	}

	return authMiddleware(opts.AdminPassword, mux)
}

type server struct {
	opts Options
}

/* ---------- 鉴权 ---------- */

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

/* ---------- 操作人与审计 ---------- */

// operatorOf 解析当前请求的操作人。优先级:
//  1. X-Phx-User-* 身份头(mcp 服务从 OAuth token 透传)→ actor 为身份展示口径,source=oauth;
//  2. 无身份头但 X-Phx-Source=mcp(OAuth off/optional 且调用方未带 token)→ actor 空,source=anonymous;
//  3. 其余(管理后台/脚本,共享密码无法区分到人)→ actor='admin',source=admin。
//
// 身份头只是传输载体:请求已过 authMiddleware(X-Access-Key)才会到这里,
// 头的内容才可信。注意 PHX_ADMIN_PASSWORD 置空(鉴权关闭)时身份可被伪造,
// 这与现状"关闭鉴权仅限本机联调"的约束一致。
// TODO: 管理后台接入个人登录后,'admin' 应替换为真实用户。
func operatorOf(r *http.Request) (actor, source string, u identity.User) {
	if u, ok := identity.FromHeaders(r.Header); ok {
		return u.Display(), "oauth", u
	}
	if r.Header.Get(identity.HeaderSource) == "mcp" {
		return "", "anonymous", identity.User{}
	}
	return "admin", "admin", identity.User{}
}

// audit 记录一条审计日志;失败仅告警,不阻断业务。
func (s *server) audit(r *http.Request, action, docID string, extra map[string]any) {
	actor, source, u := operatorOf(r)
	detail := map[string]any{}
	if !u.IsZero() {
		detail["sub"], detail["username"], detail["email"] = u.Sub, u.Username, u.Email
	}
	for k, v := range extra {
		detail[k] = v
	}
	e := store.AuditEntry{Actor: actor, ActorSource: source, Action: action, DocumentID: docID, Detail: detail}
	if err := s.opts.DB.InsertAudit(r.Context(), e); err != nil {
		slog.Warn("审计日志写入失败", "action", action, "document_id", docID, "error", err)
	}
}

/* ---------- 文档处理 ---------- */

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
	actor, _, _ := operatorOf(r)
	doc, err := s.opts.Pipeline.Upload(r.Context(), req.DocType, req.Filename, data, actor)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	s.audit(r, "upload", doc.ID, map[string]any{"doc_type": doc.DocType, "filename": doc.Filename})
	writeJSON(w, api.ToView(doc))
}

func (s *server) extract(w http.ResponseWriter, r *http.Request) {
	doc, err := s.opts.Pipeline.ExtractFields(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	s.audit(r, "extract", doc.ID, nil)
	writeJSON(w, api.ToView(doc))
}

func (s *server) validate(w http.ResponseWriter, r *http.Request) {
	doc, err := s.opts.Pipeline.Validate(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	s.audit(r, "validate", doc.ID, nil)
	writeJSON(w, api.ToView(doc))
}

func (s *server) save(w http.ResponseWriter, r *http.Request) {
	var req api.SaveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "请求体解析失败: "+err.Error())
		return
	}
	actor, _, _ := operatorOf(r)
	doc, err := s.opts.Pipeline.Save(r.Context(), r.PathValue("id"), req.Fields, req.Force, actor)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	s.audit(r, "save", doc.ID, map[string]any{"force": req.Force, "fields_overridden": len(req.Fields) > 0})
	writeJSON(w, api.ToView(doc))
}

func (s *server) query(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	docs, err := s.opts.Pipeline.Query(r.Context(), store.QueryFilter{
		DocType:    q.Get("doc_type"),
		Status:     q.Get("status"),
		Keyword:    q.Get("keyword"),
		UploadedBy: q.Get("uploaded_by"),
		Limit:      limit,
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
	for _, name := range s.opts.Registry.Names() {
		dt, _ := s.opts.Registry.Get(name)
		types = append(types, dt)
	}
	writeJSON(w, types)
}

/* ---------- 健康聚合 ---------- */

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
		check("postgres 数据库", func() error { return s.opts.DB.Ping(ctx) }),
	}
	for _, t := range s.opts.HealthTargets {
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

/* ---------- 上传内容解析 ---------- */

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

/* ---------- 响应工具 ---------- */

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(api.ErrorResponse{Error: msg})
}
