// Package mcpserver 实现平台的 MCP Server(WorkBuddy 连接器)。
// 它是工作流引擎的对外门面:每个工具转调 workflow 服务的 REST API。
//
// 内容识别/字段提取由 WorkBuddy(多模态大模型)在客户端完成;后端负责归档、
// 校验、存储与检索。原五个工具名是对外契约(产品说明书 §8.1)不得改名,但行为已随
// 该架构调整,并新增 ask_document:
//   - extract_fields:返回该单据类型要抽的字段清单(下发抽取指令,后端不识别)
//   - validate_document:对 WorkBuddy 回传的字段做 schema 校验
//   - save_database:落字段+正文入库,并入知识库(切片+embedding)
//   - query_document:结构化查询,含 field_filters 字段级过滤
//   - ask_document(新增):知识库语义问答(向量检索正文)
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
// 识别/提取由你(WorkBuddy 多模态大模型)完成,平台负责归档、校验、存储与检索。
const expertInstructions = `你是「文档处理专家」,通过 Phoenix 企业智能文档处理平台处理企业文档。
文档的识别与字段提取由你(多模态大模型)完成,平台负责归档、校验、入库与检索。

录入流程:
1. 用户提供文档(图片/扫描件/PDF/Office 等)时,调用 upload_document 上传归档,拿到文档 ID。
   单据类型(doc_type)按用户说明选择;不确定时不传,后续再定。
   小文件用 content_base64/content_text,大文件让用户给可访问 URL 走 file_url。
2. 调用 extract_fields(文档 ID)获取该单据类型要抽取的字段清单(name/label/规则);
   若返回的是类型目录(catalog),你先判断文档属于哪种类型,再据此抽取。
3. 你亲自从原件中识别:① 按字段清单抽出各字段值;② 完整转写正文(保留关键信息)。
   不要编造或"补全"不存在的值;金额、日期保持原件写法,不做换算。
4. 调用 save_database 入库,传入 document_id、fields(抽好的字段)、content_text(转写正文)、
   doc_type(你判定的类型)。平台会做规则校验:
   - status 为 saved:入库成功,把字段值以表格汇报给用户。
   - status 为 needs_review:平台返回 issues(校验问题),把问题和当前值列给用户请其确认/修正,
     修正后重新 save_database;用户明确表示"直接入库"时才传 force=true。
   (可选)入库前想先看校验结果,可调 validate_document(document_id, fields, doc_type)预校验。
5. 用户查询历史文档时用 query_document:按类型/状态/关键词/上传人过滤;需要按字段值精确
   筛选或比较(如「金额超过 1 万的报销单」「某公司开的发票」)时,用 field_filters
   (field=字段名, op=eq/contains/gt/gte/lt/lte/in, value 或 values)。
6. 用户问的是文件正文内容(如「我传的合同里违约金怎么约定」这类开放问题)时,用 ask_document
   做语义检索,平台返回相关原文片段与来源文档,你据此作答并注明来自哪份文件。
   区分:要精确筛选/统计用 query_document;要理解正文内容用 ask_document。

原则:
- 每个关键步骤(上传、抽取结果、校验问题、入库完成)都简要反馈给用户。
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
		Name: "extract_fields",
		Description: "返回该文档类型需要抽取的字段清单(name/label/规则),供你(WorkBuddy)据此从原件中提取;" +
			"类型未定时返回可选单据类型目录(catalog)供你选型。后端不做识别。",
	}, extractHandler(wf))

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "validate_document",
		Description: "对你回传的 fields 按单据类型规则做预校验;通过为 validated,否则 needs_review 并返回问题列表(不入库)。",
	}, validateHandler(wf))

	mcp.AddTool(srv, &mcp.Tool{
		Name: "save_database",
		Description: "把你抽取的 fields 与转写的 content_text(正文)入库归档。平台会做权威校验:" +
			"通过则 saved;不通过且未 force 则返回 needs_review + issues,请用户确认修正后重试,或 force=true 强制入库。",
	}, saveHandler(wf))

	mcp.AddTool(srv, &mcp.Tool{
		Name: "query_document",
		Description: "结构化查询已处理文档:按单据类型/状态/关键词/上传人过滤,并可用 field_filters " +
			"按字段值精确/比较筛选(如金额>10000、甲方包含某公司、发票号=X)。适合精确、可筛选、可列全的查询。",
	}, queryHandler(wf))

	mcp.AddTool(srv, &mcp.Tool{
		Name: "ask_document",
		Description: "对已上传文件的正文内容做自然语言语义问答:返回相关原文片段与来源文档," +
			"你据此作答并注明来自哪份文件。适合「我传的合同里违约金怎么约定」这类开放的内容型问题" +
			"(精确筛选/统计请改用 query_document)。",
	}, askHandler(wf))

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
	DocType       string `json:"doc_type,omitempty" jsonschema:"单据类型;不确定时不传,后续 save 时再定"`
	Filename      string `json:"filename" jsonschema:"文件名,含扩展名"`
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

// extractHandler 返回字段清单(api.FieldBrief),不做识别。
func extractHandler(wf *clients.Workflow) mcp.ToolHandlerFor[docIDInput, api.FieldBrief] {
	return func(ctx context.Context, req *mcp.CallToolRequest, in docIDInput) (*mcp.CallToolResult, api.FieldBrief, error) {
		ctx = withUser(ctx, req)
		brief, err := wf.Extract(ctx, in.DocumentID)
		if err != nil {
			return nil, api.FieldBrief{}, err
		}
		return nil, brief, nil
	}
}

type validateInput struct {
	DocumentID string        `json:"document_id" jsonschema:"文档 ID"`
	Fields     []model.Field `json:"fields" jsonschema:"你抽取的字段"`
	DocType    string        `json:"doc_type,omitempty" jsonschema:"你判定的单据类型"`
}

func validateHandler(wf *clients.Workflow) mcp.ToolHandlerFor[validateInput, docResult] {
	return func(ctx context.Context, req *mcp.CallToolRequest, in validateInput) (*mcp.CallToolResult, docResult, error) {
		ctx = withUser(ctx, req)
		view, err := wf.Validate(ctx, in.DocumentID, api.ValidateRequest{Fields: in.Fields, DocType: in.DocType})
		if err != nil {
			return nil, docResult{}, err
		}
		return nil, toResult(view), nil
	}
}

type saveInput struct {
	DocumentID  string        `json:"document_id" jsonschema:"文档 ID"`
	Fields      []model.Field `json:"fields,omitempty" jsonschema:"你抽取的字段"`
	ContentText string        `json:"content_text,omitempty" jsonschema:"你转写的正文,用于归档检索与知识库"`
	DocType     string        `json:"doc_type,omitempty" jsonschema:"你判定的单据类型"`
	Force       bool          `json:"force,omitempty" jsonschema:"true 时允许 needs_review 直接入库"`
}

func saveHandler(wf *clients.Workflow) mcp.ToolHandlerFor[saveInput, docResult] {
	return func(ctx context.Context, req *mcp.CallToolRequest, in saveInput) (*mcp.CallToolResult, docResult, error) {
		ctx = withUser(ctx, req)
		view, err := wf.Save(ctx, in.DocumentID, api.SaveRequest{
			Fields:      in.Fields,
			ContentText: in.ContentText,
			DocType:     in.DocType,
			Force:       in.Force,
		})
		if err != nil {
			return nil, docResult{}, err
		}
		return nil, toResult(view), nil
	}
}

// queryFieldFilter 是按提取字段值做精确/比较过滤的条件(WorkBuddy 据自然语言构造)。
type queryFieldFilter struct {
	Field  string   `json:"field" jsonschema:"字段名(与该单据类型的字段一致,如 amount/party_a)"`
	Op     string   `json:"op" jsonschema:"eq(等于)|ne(不等)|contains(包含)|gt|gte|lt|lte(数值比较)|in(在候选中)"`
	Value  string   `json:"value,omitempty" jsonschema:"比较值;gt/gte/lt/lte 按数值比较(会自动去千分位逗号)"`
	Values []string `json:"values,omitempty" jsonschema:"当 op 为 in 时的候选值列表"`
}

type queryInput struct {
	DocType      string             `json:"doc_type,omitempty" jsonschema:"按单据类型过滤"`
	Status       string             `json:"status,omitempty" jsonschema:"按状态过滤"`
	Keyword      string             `json:"keyword,omitempty" jsonschema:"匹配文件名或正文的关键词"`
	UploadedBy   string             `json:"uploaded_by,omitempty" jsonschema:"按上传人过滤(操作人标识)"`
	FieldFilters []queryFieldFilter `json:"field_filters,omitempty" jsonschema:"按字段值精确/比较过滤,如金额>10000、甲方包含某公司"`
	Limit        int                `json:"limit,omitempty" jsonschema:"返回条数,默认 20,上限 100"`
}

type queryOutput struct {
	Total     int         `json:"total"`
	Documents []docResult `json:"documents"`
}

func queryHandler(wf *clients.Workflow) mcp.ToolHandlerFor[queryInput, queryOutput] {
	return func(ctx context.Context, req *mcp.CallToolRequest, in queryInput) (*mcp.CallToolResult, queryOutput, error) {
		ctx = withUser(ctx, req)
		filters := make([]store.FieldFilter, 0, len(in.FieldFilters))
		for _, f := range in.FieldFilters {
			filters = append(filters, store.FieldFilter{Field: f.Field, Op: f.Op, Value: f.Value, Values: f.Values})
		}
		// TODO: 「普通员工默认只查自己上传的文档」(方案 §8 Q5)未拍板,当前不做默认过滤
		res, err := wf.Query(ctx, store.QueryFilter{
			DocType:      in.DocType,
			Status:       in.Status,
			Keyword:      in.Keyword,
			UploadedBy:   in.UploadedBy,
			FieldFilters: filters,
			Limit:        in.Limit,
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

type askInput struct {
	Question string `json:"question" jsonschema:"用户关于已上传文件内容的自然语言问题"`
	DocType  string `json:"doc_type,omitempty" jsonschema:"可选:限定在某单据类型内检索"`
	Limit    int    `json:"limit,omitempty" jsonschema:"返回相关片段数,默认 5,上限 50"`
}

func askHandler(wf *clients.Workflow) mcp.ToolHandlerFor[askInput, api.AskResult] {
	return func(ctx context.Context, req *mcp.CallToolRequest, in askInput) (*mcp.CallToolResult, api.AskResult, error) {
		ctx = withUser(ctx, req)
		res, err := wf.Ask(ctx, api.AskRequest{Question: in.Question, Limit: in.Limit, DocType: in.DocType})
		if err != nil {
			return nil, api.AskResult{}, err
		}
		return nil, res, nil
	}
}
