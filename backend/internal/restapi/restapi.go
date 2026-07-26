// Package restapi 是「员工级」公网 REST 面(/pub/v1),自研账号体系(V1.3 取代 Keycloak)。
//
// 它服务 phoenix-doc-assistant 专家(WorkBuddy 内置 Python 脚本):员工用账号口令
// 登录换取本平台签发的 JWT(internal/userauth),之后每个请求携带 Bearer access token,
// 身份在这里校验后取得(**不信任任何客户端请求头**),据此落
// documents.uploaded_by/reviewed_by 与 audit_log —— 每个操作都能追溯到具体员工。
// 员工账号由管理后台维护(workflowapi /api/users)。
//
// 与既有的 internal/workflowapi(X-Access-Key,前端在用)完全独立、路由前缀不重叠。
// 业务逻辑全部复用 pipeline,本包只加薄薄一层 HTTP 解析 + 鉴权 + 审计。
// 装配见 cmd/workflow/main.go(PHX_AUTH_SECRET 未配置时本面不挂载)。
//
// 路由一览:
//
//	POST   /pub/v1/auth/login               账号口令 → access + refresh token(开放)
//	POST   /pub/v1/auth/refresh             refresh token 续期(开放)
//	GET    /pub/v1/me                       当前 token 对应的员工身份(客户端确认登录用)
//	POST   /pub/v1/documents                上传归档(content_text/base64/file_url 三选一)
//	POST   /pub/v1/documents/{id}/extract   返回该类型要抽的字段清单(WorkBuddy 据此识别)
//	POST   /pub/v1/documents/{id}/validate  对回传字段做 schema 预校验(不入库)
//	POST   /pub/v1/documents/{id}/save      落字段+正文并入库(权威校验)
//	GET    /pub/v1/documents                结构化查询(doc_type/status/keyword/uploaded_by/field_filters/limit)
//	POST   /pub/v1/ask                      知识库语义问答
//
// 除 auth/* 外全部要求有效 Bearer access token。
package restapi

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/yuanyuexiang/phoenix/internal/api"
	"github.com/yuanyuexiang/phoenix/internal/identity"
	"github.com/yuanyuexiang/phoenix/internal/pipeline"
	"github.com/yuanyuexiang/phoenix/internal/store"
	"github.com/yuanyuexiang/phoenix/internal/userauth"
)

const maxFetchSize = 64 << 20 // file_url 下载上限 64MB

// Options 是构建 /pub/v1 REST 面所需的依赖。
type Options struct {
	Pipeline *pipeline.Pipeline
	DB       *store.DB
	Auth     *userauth.Service
}

// NewHandler 组装 /pub/v1 路由:auth/* 开放,其余套 Bearer 鉴权中间件。
// 注意本面不暴露删除等破坏性操作(删除/覆盖引导管理后台人工),
// 员工侧只做登录/上传/识别/校验/入库/查询/问答。
func NewHandler(opts Options) http.Handler {
	s := &server{opts: opts}

	protected := http.NewServeMux()
	protected.HandleFunc("GET /pub/v1/me", s.me)
	protected.HandleFunc("POST /pub/v1/documents", s.upload)
	protected.HandleFunc("POST /pub/v1/documents/{id}/extract", s.extract)
	protected.HandleFunc("POST /pub/v1/documents/{id}/validate", s.validate)
	protected.HandleFunc("POST /pub/v1/documents/{id}/save", s.save)
	protected.HandleFunc("GET /pub/v1/documents", s.query)
	protected.HandleFunc("POST /pub/v1/ask", s.ask)

	root := http.NewServeMux()
	root.HandleFunc("POST /pub/v1/auth/login", s.login)
	root.HandleFunc("POST /pub/v1/auth/refresh", s.refresh)
	root.Handle("/pub/v1/", s.authMiddleware(protected))
	return root
}

type server struct {
	opts Options
}

/* ---------- 审计 ---------- */

// audit 记录一条审计日志;身份必为 oauth(已过 Bearer 鉴权)。失败仅告警,不阻断业务。
func (s *server) audit(r *http.Request, action, docID string, extra map[string]any) {
	u, _ := identity.FromContext(r.Context())
	detail := map[string]any{"sub": u.Sub, "username": u.Username, "email": u.Email}
	for k, v := range extra {
		detail[k] = v
	}
	e := store.AuditEntry{Actor: u.Display(), ActorSource: "oauth", Action: action, DocumentID: docID, Detail: detail}
	if err := s.opts.DB.InsertAudit(r.Context(), e); err != nil {
		slog.Warn("审计日志写入失败", "action", action, "document_id", docID, "error", err)
	}
}

/* ---------- 身份自省 ---------- */

func (s *server) me(w http.ResponseWriter, r *http.Request) {
	u, _ := identity.FromContext(r.Context())
	writeJSON(w, map[string]string{
		"sub":      u.Sub,
		"username": u.Username,
		"email":    u.Email,
		"name":     u.Name,
		"display":  u.Display(),
	})
}

/* ---------- 文档处理(全部复用 pipeline) ---------- */

func (s *server) upload(w http.ResponseWriter, r *http.Request) {
	var req api.UploadRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "请求体解析失败: "+err.Error())
		return
	}
	data, err := resolveContent(r.Context(), req)
	if err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}
	u, _ := identity.FromContext(r.Context())
	doc, err := s.opts.Pipeline.Upload(r.Context(), req.DocType, req.Filename, data, u.Display())
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "UNPROCESSABLE", err.Error())
		return
	}
	s.audit(r, "upload", doc.ID, map[string]any{"doc_type": doc.DocType, "filename": doc.Filename})
	writeJSON(w, api.ToView(doc))
}

func (s *server) extract(w http.ResponseWriter, r *http.Request) {
	brief, err := s.opts.Pipeline.FieldBrief(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "UNPROCESSABLE", err.Error())
		return
	}
	writeJSON(w, brief)
}

func (s *server) validate(w http.ResponseWriter, r *http.Request) {
	var req api.ValidateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "请求体解析失败: "+err.Error())
		return
	}
	doc, err := s.opts.Pipeline.Validate(r.Context(), r.PathValue("id"), req.Fields, req.DocType)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "UNPROCESSABLE", err.Error())
		return
	}
	s.audit(r, "validate", doc.ID, nil)
	writeJSON(w, api.ToView(doc))
}

func (s *server) save(w http.ResponseWriter, r *http.Request) {
	var req api.SaveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "请求体解析失败: "+err.Error())
		return
	}
	u, _ := identity.FromContext(r.Context())
	doc, err := s.opts.Pipeline.Save(r.Context(), r.PathValue("id"), req.Fields, req.ContentText, req.DocType, req.Force, u.Display())
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "UNPROCESSABLE", err.Error())
		return
	}
	s.audit(r, "save", doc.ID, map[string]any{"force": req.Force, "has_content": req.ContentText != ""})
	writeJSON(w, api.ToView(doc))
}

func (s *server) query(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	var fieldFilters []store.FieldFilter
	if raw := q.Get("field_filters"); raw != "" {
		if err := json.Unmarshal([]byte(raw), &fieldFilters); err != nil {
			writeError(w, http.StatusBadRequest, "INVALID_FIELD_FILTER", "field_filters 解析失败: "+err.Error())
			return
		}
	}
	docs, err := s.opts.Pipeline.Query(r.Context(), store.QueryFilter{
		DocType:      q.Get("doc_type"),
		Status:       q.Get("status"),
		Keyword:      q.Get("keyword"),
		UploadedBy:   q.Get("uploaded_by"),
		FieldFilters: fieldFilters,
		Limit:        limit,
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}
	out := api.QueryResult{Total: len(docs), Documents: make([]api.DocumentView, 0, len(docs))}
	for _, d := range docs {
		out.Documents = append(out.Documents, api.ToView(d))
	}
	writeJSON(w, out)
}

func (s *server) ask(w http.ResponseWriter, r *http.Request) {
	var req api.AskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "请求体解析失败: "+err.Error())
		return
	}
	hits, err := s.opts.Pipeline.Ask(r.Context(), req.Question, req.Limit, req.DocType)
	if err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}
	out := api.AskResult{Total: len(hits), Chunks: make([]api.ChunkHit, 0, len(hits))}
	for _, h := range hits {
		out.Chunks = append(out.Chunks, api.ChunkHit{
			DocumentID: h.DocumentID, Filename: h.Filename, DocType: h.DocType, Content: h.Content, Score: h.Score,
		})
	}
	writeJSON(w, out)
}

/* ---------- 上传内容解析(与 workflowapi 同规则,独立实现以免耦合旧包) ---------- */

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

// writeError 输出 {"error":"CODE","message":"..."};与 phoenix-doc-assistant 客户端
// 的错误约定一致(api_client.py 直接透传该 JSON 给模型)。
func writeError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": code, "message": message})
}
