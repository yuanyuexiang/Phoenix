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
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strconv"
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

	s := &server{p: p, registry: registry}
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

	slog.Info("workflow 工作流引擎已启动", "addr", addr)
	return http.ListenAndServe(addr, mux)
}

type server struct {
	p        *pipeline.Pipeline
	registry *schema.Registry
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
