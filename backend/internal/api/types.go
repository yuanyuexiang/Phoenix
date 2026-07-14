// Package api 定义服务间 HTTP 契约的共享 DTO(Monorepo 内多服务共用)。
//
// 服务拓扑(说明书 §7 系统组成):
//
//	WorkBuddy ─MCP→ services/mcp ──┐
//	                               ├─REST→ services/workflow ──→ services/parser
//	浏览器 ───→ services/admin ────┘        │      │    │        services/ai
//	                                        ▼      ▼    └──────→ services/ocr
//	                                  PostgreSQL  MinIO
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
type SaveRequest struct {
	Fields []model.Field `json:"fields,omitempty"` // 人工审核修正后的字段
	Force  bool          `json:"force,omitempty"`
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

// ParseResponse 是 parser 服务 POST /parse 的响应体。
type ParseResponse struct {
	Text string `json:"text"`
}

// ExtractRequest 是 ai 服务 POST /extract 的请求体。
// 字段定义随请求下发:单据类型配置归 workflow 管,ai 服务保持无状态。
// Fields 为空 = 开放提取模式:不套 schema,抽取文档中实际存在的键值对。
type ExtractRequest struct {
	Text    string          `json:"text"`
	DocType string          `json:"doc_type"`
	Fields  []FieldSpecView `json:"fields,omitempty"`
}

// DocTypeCandidate 是分类候选单据类型(Labels 为各字段中文标签)。
type DocTypeCandidate struct {
	Name        string   `json:"name"`
	Title       string   `json:"title"`
	Description string   `json:"description,omitempty"`
	Labels      []string `json:"labels"`
}

// ClassifyRequest 是 ai 服务 POST /classify 的请求体。
type ClassifyRequest struct {
	Text       string             `json:"text"`
	Candidates []DocTypeCandidate `json:"candidates"`
}

// ClassifyResponse 是 ai 服务 POST /classify 的响应体。
// 无法判断时 DocType 为空、Confidence 为 0。
type ClassifyResponse struct {
	DocType    string  `json:"doc_type"`
	Confidence float64 `json:"confidence"`
	Classifier string  `json:"classifier"`
}

// FieldSpecView 是下发给 ai 服务的字段定义(internal/schema.FieldSpec 的传输形态)。
type FieldSpecView struct {
	Name        string   `json:"name"`
	Label       string   `json:"label"`
	Description string   `json:"description,omitempty"`
	Aliases     []string `json:"aliases,omitempty"`
}

// ExtractResponse 是 ai 服务 POST /extract 的响应体。
type ExtractResponse struct {
	Extractor string        `json:"extractor"` // mock 或 llm:<model>
	Fields    []model.Field `json:"fields"`
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
