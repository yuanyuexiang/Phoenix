// Package api 定义服务间 HTTP 契约的共享 DTO(Monorepo 内多服务共用)。
//
// 服务拓扑(说明书 §7 系统组成):内容识别/提取由 WorkBuddy(多模态大模型)完成,
// 后端只负责归档、校验、存储与检索。
//
//	WorkBuddy ─MCP→ services/mcp ──REST→ services/workflow
//	浏览器 ───→ services/admin ──────────┘   │     │
//	                                    PostgreSQL  MinIO
package api

import "github.com/yuanyuexiang/phoenix/internal/model"

// UploadRequest 是 workflow 服务 POST /api/documents 的请求体。
// 文件内容三选一:ContentText、ContentBase64、FileURL(由 workflow 下载)。
type UploadRequest struct {
	DocType       string `json:"doc_type"`
	Filename      string `json:"filename"`
	ContentText   string `json:"content_text,omitempty"`
	ContentBase64 string `json:"content_base64,omitempty"`
	FileURL       string `json:"file_url,omitempty"`
}

// SaveRequest 是 POST /api/documents/{id}/save 的请求体。
// 字段与正文均由 WorkBuddy 识别后回传;ContentText 落 documents.content_text
// 供检索与知识库使用。
type SaveRequest struct {
	Fields      []model.Field `json:"fields,omitempty"`       // WorkBuddy 抽取的字段
	ContentText string        `json:"content_text,omitempty"` // WorkBuddy 识别出的正文
	DocType     string        `json:"doc_type,omitempty"`     // WorkBuddy 定的单据类型(覆盖上传时的)
	Force       bool          `json:"force,omitempty"`        // 强制入库(跳过 needs_review)
}

// ValidateRequest 是 POST /api/documents/{id}/validate 的请求体。
type ValidateRequest struct {
	Fields  []model.Field `json:"fields,omitempty"`
	DocType string        `json:"doc_type,omitempty"`
}

// DocumentView 是对外(MCP/管理后台)统一的文档视图。
type DocumentView struct {
	ID         string                  `json:"id"`
	DocType    string                  `json:"doc_type"`
	Filename   string                  `json:"filename"`
	Status     string                  `json:"status"`
	Error      string                  `json:"error,omitempty"`
	Fields     []model.Field           `json:"fields,omitempty"`
	Issues     []model.ValidationIssue `json:"issues,omitempty"`
	UploadedBy string                  `json:"uploaded_by,omitempty"`
	ReviewedBy string                  `json:"reviewed_by,omitempty"`
	CreatedAt  string                  `json:"created_at,omitempty"`
}

// QueryResult 是 GET /api/documents 的响应体。
type QueryResult struct {
	Total     int            `json:"total"`
	Documents []DocumentView `json:"documents"`
}

// AskRequest 是 POST /api/ask 的请求体(知识库语义问答)。
type AskRequest struct {
	Question string `json:"question"`
	Limit    int    `json:"limit,omitempty"`
	DocType  string `json:"doc_type,omitempty"`
}

// ChunkHit 是知识库检索命中的一条正文片段(与 store.ChunkHit 对齐)。
type ChunkHit struct {
	DocumentID string  `json:"document_id"`
	Filename   string  `json:"filename"`
	DocType    string  `json:"doc_type"`
	Content    string  `json:"content"`
	Score      float64 `json:"score"`
}

// AskResult 是 POST /api/ask 的响应体。
type AskResult struct {
	Total  int        `json:"total"`
	Chunks []ChunkHit `json:"chunks"`
}

// FieldBrief 是 extract_fields 的返回:告诉 WorkBuddy 该抽哪些字段。
// 后端不再做识别,只下发抽取指令。DocType 为 auto/unknown 时 Fields 为空、
// Catalog 给出全部已配置类型供 WorkBuddy 选型。
type FieldBrief struct {
	DocType string          `json:"doc_type"`
	Title   string          `json:"title,omitempty"`
	Fields  []BriefField    `json:"fields,omitempty"`  // 该类型要抽的字段清单
	Catalog []DocTypeDigest `json:"catalog,omitempty"` // 类型未定时的可选单据类型目录
}

// BriefField 是下发给 WorkBuddy 的单个字段说明(含规则摘要,帮它抽对格式)。
type BriefField struct {
	Name        string   `json:"name"`
	Label       string   `json:"label"`
	Description string   `json:"description,omitempty"`
	Aliases     []string `json:"aliases,omitempty"`
	Required    bool     `json:"required,omitempty"`
	Pattern     string   `json:"pattern,omitempty"`
	Enum        []string `json:"enum,omitempty"`
}

// DocTypeDigest 是单据类型目录项(供 WorkBuddy 在类型未定时选型)。
type DocTypeDigest struct {
	Name        string `json:"name"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
}

// ErrorResponse 是各服务统一的错误响应体。
type ErrorResponse struct {
	Error string `json:"error"`
}

// ToView 把领域模型转成对外视图。
func ToView(d *model.Document) DocumentView {
	return DocumentView{
		ID:         d.ID,
		DocType:    d.DocType,
		Filename:   d.Filename,
		Status:     string(d.Status),
		Error:      d.Error,
		Fields:     d.Fields,
		Issues:     d.Issues,
		UploadedBy: d.UploadedBy,
		ReviewedBy: d.ReviewedBy,
		CreatedAt:  d.CreatedAt.Format("2006-01-02 15:04:05"),
	}
}
