// Package mcpserver 实现平台的 MCP Server(WorkBuddy 连接器)。
// 它是工作流引擎的对外门面:每个工具转调 workflow 服务的 REST API。
//
// 五个工具名是对外契约(产品说明书 §8.1),不得改名:
// upload_document / extract_fields / validate_document / save_database / query_document
//
// 「连接器即专家包」:专家提示词随服务器分发(docs/文档处理专家_发布包.md §3 为同源文本)——
//   - Server Instructions:支持该能力的客户端连上即自动获得专家行为;
//   - MCP Prompt(document-expert):不吃 instructions 的客户端可显式拉取。
//
// 修改提示词只需改本文件的 expertInstructions 并发版,无需在各 WorkBuddy 环境手工同步。
package mcpserver

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/yuanyuexiang/phoenix/internal/api"
	"github.com/yuanyuexiang/phoenix/internal/clients"
	"github.com/yuanyuexiang/phoenix/internal/identity"
	"github.com/yuanyuexiang/phoenix/internal/model"
	"github.com/yuanyuexiang/phoenix/internal/store"
)

// expertInstructions 是「文档处理专家」的系统提示词,与 docs/文档处理专家_发布包.md §3 同源。
const expertInstructions = `你是「文档处理专家」,通过 Phoenix 企业智能文档处理平台的工具处理企业文档。

工作流程:
1. 用户提供文档时,调用 upload_document 上传。单据类型(doc_type)按用户说明选择;
   用户未说明时不传该参数,平台会自动识别类型(识别不出会转人工审核定类型)。
   文件内容小的用 content_base64/content_text,大文件让用户提供可访问的 URL 走 file_url。
2. 上传成功后依次调用 extract_fields、validate_document。
3. 校验结果处理:
   - status 为 validated:直接调用 save_database 入库,然后把提取出的字段值
     以表格形式汇报给用户。
   - status 为 needs_review:把 issues(校验问题)和当前字段值列给用户,
     请用户确认或给出修正值;拿到修正后,把完整的 fields 数组传入 save_database。
     用户明确表示"直接入库"时才使用 force=true。
4. 用户查询历史文档时,调用 query_document(支持 doc_type/status/keyword/limit)。

原则:
- 不要编造或"补全"文档中不存在的字段值;提取不到就如实告知。
- 金额、日期等保持文档原始写法,不做换算。
- 每个关键步骤(上传成功、提取结果、校验问题、入库完成)都简要反馈给用户。
- 涉及删除、覆盖已入库数据的请求,一律引导用户到管理后台人工操作。`

// New 构建挂好全部工具与专家提示词的 MCP Server。
func New(wf *clients.Workflow, version string) *mcp.Server {
	srv := mcp.NewServer(&mcp.Implementation{
		Name:    "phoenix",
		Title:   "Phoenix 企业智能文档处理平台",
		Version: version,
	}, &mcp.ServerOptions{
		Instructions: expertInstructions, // 连接即获得专家行为(客户端支持 instructions 时)
	})

	srv.AddPrompt(&mcp.Prompt{
		Name:        "document-expert",
		Title:       "文档处理专家",
		Description: "企业单据的上传、识别、提取、校验、入库全流程处理(与本连接器五个工具配套)。",
	}, func(_ context.Context, _ *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		return &mcp.GetPromptResult{
			Description: "文档处理专家系统提示词",
			Messages: []*mcp.PromptMessage{
				{Role: "user", Content: &mcp.TextContent{Text: expertInstructions}},
			},
		}, nil
	})

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

// userFromRequest 从当次请求的 OAuth TokenInfo 提取操作人身份。
//
// 注意:必须取 req.Extra.TokenInfo,不能用 auth.TokenInfoFromContext(ctx)——
// Streamable HTTP 下工具 handler 的 ctx 源自 initialize 请求,不是当次 HTTP 请求;
// SDK 对每个 JSON-RPC 请求单独附上 RequestExtra,身份始终是当次请求的。
// OAuth 关闭或 optional 模式匿名调用时 TokenInfo 为 nil,返回 ok=false。
func userFromRequest(req *mcp.CallToolRequest) (identity.User, bool) {
	if req == nil || req.Extra == nil || req.Extra.TokenInfo == nil {
		return identity.User{}, false
	}
	ti := req.Extra.TokenInfo
	str := func(k string) string { s, _ := ti.Extra[k].(string); return s }
	u := identity.User{
		Sub:      ti.UserID,
		Username: str("username"),
		Email:    str("email"),
		Name:     str("name"),
	}
	return u, !u.IsZero()
}

// withUser 把当次请求的身份注入 ctx,随 workflow 客户端的出站请求头透传落库。
func withUser(ctx context.Context, req *mcp.CallToolRequest) context.Context {
	if u, ok := userFromRequest(req); ok {
		return identity.WithUser(ctx, u)
	}
	return ctx
}

// docResult 是各工具统一返回的文档视图。
type docResult struct {
	ID         string                  `json:"id" jsonschema:"文档 ID"`
	DocType    string                  `json:"doc_type" jsonschema:"单据类型"`
	Filename   string                  `json:"filename"`
	Status     string                  `json:"status" jsonschema:"uploaded|extracted|validated|needs_review|saved|failed"`
	Error      string                  `json:"error,omitempty"`
	Fields     []model.Field           `json:"fields,omitempty" jsonschema:"提取出的字段及置信度"`
	Issues     []model.ValidationIssue `json:"issues,omitempty" jsonschema:"校验问题列表"`
	UploadedBy string                  `json:"uploaded_by,omitempty" jsonschema:"上传人"`
	ReviewedBy string                  `json:"reviewed_by,omitempty" jsonschema:"入库确认人"`
}

func toResult(v api.DocumentView) docResult {
	return docResult{
		ID:         v.ID,
		DocType:    v.DocType,
		Filename:   v.Filename,
		Status:     v.Status,
		Error:      v.Error,
		Fields:     v.Fields,
		Issues:     v.Issues,
		UploadedBy: v.UploadedBy,
		ReviewedBy: v.ReviewedBy,
	}
}

type uploadInput struct {
	DocType       string `json:"doc_type,omitempty" jsonschema:"单据类型;不传或传 auto 时平台自动识别,识别失败会转人工审核"`
	Filename      string `json:"filename" jsonschema:"文件名,含扩展名,平台按扩展名选择解析路径"`
	ContentText   string `json:"content_text,omitempty" jsonschema:"纯文本内容,与 content_base64/file_url 三选一"`
	ContentBase64 string `json:"content_base64,omitempty" jsonschema:"base64 编码的文件内容,仅适合小文件"`
	FileURL       string `json:"file_url,omitempty" jsonschema:"可下载的文件 URL,大文件推荐方式"`
}

func uploadHandler(wf *clients.Workflow) mcp.ToolHandlerFor[uploadInput, docResult] {
	return func(ctx context.Context, req *mcp.CallToolRequest, in uploadInput) (*mcp.CallToolResult, docResult, error) {
		ctx = withUser(ctx, req)
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
	return func(ctx context.Context, req *mcp.CallToolRequest, in docIDInput) (*mcp.CallToolResult, docResult, error) {
		ctx = withUser(ctx, req)
		view, err := wf.Extract(ctx, in.DocumentID)
		if err != nil {
			return nil, docResult{}, err
		}
		return nil, toResult(view), nil
	}
}

func validateHandler(wf *clients.Workflow) mcp.ToolHandlerFor[docIDInput, docResult] {
	return func(ctx context.Context, req *mcp.CallToolRequest, in docIDInput) (*mcp.CallToolResult, docResult, error) {
		ctx = withUser(ctx, req)
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
	return func(ctx context.Context, req *mcp.CallToolRequest, in saveInput) (*mcp.CallToolResult, docResult, error) {
		ctx = withUser(ctx, req)
		view, err := wf.Save(ctx, in.DocumentID, api.SaveRequest{Fields: in.Fields, Force: in.Force})
		if err != nil {
			return nil, docResult{}, err
		}
		return nil, toResult(view), nil
	}
}

type queryInput struct {
	DocType    string `json:"doc_type,omitempty" jsonschema:"按单据类型过滤"`
	Status     string `json:"status,omitempty" jsonschema:"按状态过滤"`
	Keyword    string `json:"keyword,omitempty" jsonschema:"匹配文件名或正文的关键词"`
	UploadedBy string `json:"uploaded_by,omitempty" jsonschema:"按上传人过滤(操作人标识)"`
	Limit      int    `json:"limit,omitempty" jsonschema:"返回条数,默认 20,上限 100"`
}

type queryOutput struct {
	Total     int         `json:"total"`
	Documents []docResult `json:"documents"`
}

func queryHandler(wf *clients.Workflow) mcp.ToolHandlerFor[queryInput, queryOutput] {
	return func(ctx context.Context, req *mcp.CallToolRequest, in queryInput) (*mcp.CallToolResult, queryOutput, error) {
		ctx = withUser(ctx, req)
		// TODO: 「普通员工默认只查自己上传的文档」(方案 §8 Q5)未拍板,当前不做默认过滤
		res, err := wf.Query(ctx, store.QueryFilter{
			DocType:    in.DocType,
			Status:     in.Status,
			Keyword:    in.Keyword,
			UploadedBy: in.UploadedBy,
			Limit:      in.Limit,
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
