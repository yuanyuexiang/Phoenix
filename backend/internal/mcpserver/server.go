// Package mcpserver 实现平台的 MCP Server(WorkBuddy 连接器)。
// 它是工作流引擎的对外门面:每个工具转调 workflow 服务的 REST API。
//
// 五个工具名是对外契约(产品说明书 §8.1),不得改名:
// upload_document / extract_fields / validate_document / save_database / query_document
package mcpserver

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/yuanyuexiang/phoenix/internal/api"
	"github.com/yuanyuexiang/phoenix/internal/clients"
	"github.com/yuanyuexiang/phoenix/internal/model"
	"github.com/yuanyuexiang/phoenix/internal/store"
)

// New 构建挂好全部工具的 MCP Server。
func New(wf *clients.Workflow, version string) *mcp.Server {
	srv := mcp.NewServer(&mcp.Implementation{
		Name:    "phoenix",
		Title:   "Phoenix 企业智能文档处理平台",
		Version: version,
	}, nil)

	mcp.AddTool(srv, &mcp.Tool{
		Name: "upload_document",
		Description: "上传一份文档并归档,返回文档 ID。文件内容三选一:content_text(纯文本)、" +
			"content_base64(小文件直传)、file_url(推荐,平台自行下载)。",
	}, uploadHandler(wf))

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "extract_fields",
		Description: "对已上传的文档执行 OCR/解析与 AI 字段提取,返回字段键值对及置信度。",
	}, extractHandler(wf))

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "validate_document",
		Description: "对已提取字段的文档执行规则校验;通过则状态为 validated,否则为 needs_review 并返回问题列表。",
	}, validateHandler(wf))

	mcp.AddTool(srv, &mcp.Tool{
		Name: "save_database",
		Description: "确认结构化数据入库。文档须已通过校验;待人工审核的文档可传入修正后的 fields," +
			"或 force=true 强制入库。",
	}, saveHandler(wf))

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "query_document",
		Description: "按单据类型、状态、关键词查询已处理的文档及其提取结果。",
	}, queryHandler(wf))

	return srv
}

// docResult 是各工具统一返回的文档视图。
type docResult struct {
	ID       string                  `json:"id" jsonschema:"文档 ID"`
	DocType  string                  `json:"doc_type" jsonschema:"单据类型"`
	Filename string                  `json:"filename"`
	Status   string                  `json:"status" jsonschema:"uploaded|extracted|validated|needs_review|saved|failed"`
	Error    string                  `json:"error,omitempty"`
	Fields   []model.Field           `json:"fields,omitempty" jsonschema:"提取出的字段及置信度"`
	Issues   []model.ValidationIssue `json:"issues,omitempty" jsonschema:"校验问题列表"`
}

func toResult(v api.DocumentView) docResult {
	return docResult{
		ID:       v.ID,
		DocType:  v.DocType,
		Filename: v.Filename,
		Status:   v.Status,
		Error:    v.Error,
		Fields:   v.Fields,
		Issues:   v.Issues,
	}
}

type uploadInput struct {
	DocType       string `json:"doc_type" jsonschema:"单据类型,须与平台配置的 doctypes 一致"`
	Filename      string `json:"filename" jsonschema:"文件名,含扩展名,平台按扩展名选择解析路径"`
	ContentText   string `json:"content_text,omitempty" jsonschema:"纯文本内容,与 content_base64/file_url 三选一"`
	ContentBase64 string `json:"content_base64,omitempty" jsonschema:"base64 编码的文件内容,仅适合小文件"`
	FileURL       string `json:"file_url,omitempty" jsonschema:"可下载的文件 URL,大文件推荐方式"`
}

func uploadHandler(wf *clients.Workflow) mcp.ToolHandlerFor[uploadInput, docResult] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in uploadInput) (*mcp.CallToolResult, docResult, error) {
		view, err := wf.Upload(ctx, api.UploadRequest{
			DocType:       in.DocType,
			Filename:      in.Filename,
			ContentText:   in.ContentText,
			ContentBase64: in.ContentBase64,
			FileURL:       in.FileURL,
		})
		if err != nil {
			return nil, docResult{}, err
		}
		return nil, toResult(view), nil
	}
}

type docIDInput struct {
	DocumentID string `json:"document_id" jsonschema:"upload_document 返回的文档 ID"`
}

func extractHandler(wf *clients.Workflow) mcp.ToolHandlerFor[docIDInput, docResult] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in docIDInput) (*mcp.CallToolResult, docResult, error) {
		view, err := wf.Extract(ctx, in.DocumentID)
		if err != nil {
			return nil, docResult{}, err
		}
		return nil, toResult(view), nil
	}
}

func validateHandler(wf *clients.Workflow) mcp.ToolHandlerFor[docIDInput, docResult] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in docIDInput) (*mcp.CallToolResult, docResult, error) {
		view, err := wf.Validate(ctx, in.DocumentID)
		if err != nil {
			return nil, docResult{}, err
		}
		return nil, toResult(view), nil
	}
}

type saveInput struct {
	DocumentID string        `json:"document_id" jsonschema:"文档 ID"`
	Fields     []model.Field `json:"fields,omitempty" jsonschema:"人工审核修正后的字段;传入则覆盖提取结果"`
	Force      bool          `json:"force,omitempty" jsonschema:"true 时允许 needs_review 状态直接入库"`
}

func saveHandler(wf *clients.Workflow) mcp.ToolHandlerFor[saveInput, docResult] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in saveInput) (*mcp.CallToolResult, docResult, error) {
		view, err := wf.Save(ctx, in.DocumentID, api.SaveRequest{Fields: in.Fields, Force: in.Force})
		if err != nil {
			return nil, docResult{}, err
		}
		return nil, toResult(view), nil
	}
}

type queryInput struct {
	DocType string `json:"doc_type,omitempty" jsonschema:"按单据类型过滤"`
	Status  string `json:"status,omitempty" jsonschema:"按状态过滤"`
	Keyword string `json:"keyword,omitempty" jsonschema:"匹配文件名或正文的关键词"`
	Limit   int    `json:"limit,omitempty" jsonschema:"返回条数,默认 20,上限 100"`
}

type queryOutput struct {
	Total     int         `json:"total"`
	Documents []docResult `json:"documents"`
}

func queryHandler(wf *clients.Workflow) mcp.ToolHandlerFor[queryInput, queryOutput] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in queryInput) (*mcp.CallToolResult, queryOutput, error) {
		res, err := wf.Query(ctx, store.QueryFilter{
			DocType: in.DocType,
			Status:  in.Status,
			Keyword: in.Keyword,
			Limit:   in.Limit,
		})
		if err != nil {
			return nil, queryOutput{}, err
		}
		out := queryOutput{Total: res.Total, Documents: make([]docResult, 0, len(res.Documents))}
		for _, v := range res.Documents {
			out.Documents = append(out.Documents, toResult(v))
		}
		return nil, out, nil
	}
}
