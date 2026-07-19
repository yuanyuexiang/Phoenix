// Package clients 提供 Monorepo 内各服务的 HTTP 客户端(mcp/admin 调 workflow)。
package clients

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/yuanyuexiang/phoenix/internal/api"
	"github.com/yuanyuexiang/phoenix/internal/identity"
	"github.com/yuanyuexiang/phoenix/internal/store"
)

func postJSON[Req any, Resp any](ctx context.Context, hc *http.Client, u string, req Req) (Resp, error) {
	var zero Resp
	body, err := json.Marshal(req)
	if err != nil {
		return zero, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(body))
	if err != nil {
		return zero, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := hc.Do(httpReq)
	if err != nil {
		return zero, err
	}
	defer resp.Body.Close()
	return decode[Resp](resp)
}

func getJSON[Resp any](ctx context.Context, hc *http.Client, u string) (Resp, error) {
	var zero Resp
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return zero, err
	}
	resp, err := hc.Do(req)
	if err != nil {
		return zero, err
	}
	defer resp.Body.Close()
	return decode[Resp](resp)
}

func decode[Resp any](resp *http.Response) (Resp, error) {
	var zero Resp
	data, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if err != nil {
		return zero, err
	}
	if resp.StatusCode >= 400 {
		var er api.ErrorResponse
		if json.Unmarshal(data, &er) == nil && er.Error != "" {
			return zero, fmt.Errorf("%s", er.Error)
		}
		return zero, fmt.Errorf("HTTP %d: %s", resp.StatusCode, truncate(string(data), 200))
	}
	var out Resp
	if err := json.Unmarshal(data, &out); err != nil {
		return zero, fmt.Errorf("响应解析失败: %w", err)
	}
	return out, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// Workflow 是工作流引擎的客户端,MCP Server 等内部服务使用。
// accessKey 非空时每个请求自动带上 X-Access-Key(workflow 开启鉴权时必需)。
type Workflow struct {
	base string
	http *http.Client
}

type accessKeyTransport struct {
	key  string
	base http.RoundTripper
}

func (t accessKeyTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if t.key != "" {
		req.Header.Set("X-Access-Key", t.key)
	}
	return t.base.RoundTrip(req)
}

// identityTransport 标记请求来源为 mcp,并把 ctx 中的 OAuth 身份(工具 handler 经
// identity.WithUser 注入)透传给 workflow(X-Phx-User-* 头,见 workflowapi.operatorOf)。
type identityTransport struct {
	base http.RoundTripper
}

func (t identityTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.Header.Set(identity.HeaderSource, "mcp")
	if u, ok := identity.FromContext(req.Context()); ok {
		identity.SetHeaders(req.Header, u)
	}
	return t.base.RoundTrip(req)
}

func NewWorkflow(baseURL, accessKey string) *Workflow {
	return &Workflow{
		base: strings.TrimRight(baseURL, "/"),
		http: &http.Client{
			Timeout:   5 * time.Minute,
			Transport: identityTransport{base: accessKeyTransport{key: accessKey, base: http.DefaultTransport}},
		},
	}
}

func (c *Workflow) Upload(ctx context.Context, req api.UploadRequest) (api.DocumentView, error) {
	return postJSON[api.UploadRequest, api.DocumentView](ctx, c.http, c.base+"/api/documents", req)
}

// Extract 取该文档类型要抽的字段清单(后端不再识别,只下发抽取指令)。
func (c *Workflow) Extract(ctx context.Context, id string) (api.FieldBrief, error) {
	return postJSON[struct{}, api.FieldBrief](ctx, c.http, c.base+"/api/documents/"+url.PathEscape(id)+"/extract", struct{}{})
}

// Validate 对回传字段做 schema 预校验。
func (c *Workflow) Validate(ctx context.Context, id string, req api.ValidateRequest) (api.DocumentView, error) {
	return postJSON[api.ValidateRequest, api.DocumentView](ctx, c.http, c.base+"/api/documents/"+url.PathEscape(id)+"/validate", req)
}

func (c *Workflow) Save(ctx context.Context, id string, req api.SaveRequest) (api.DocumentView, error) {
	return postJSON[api.SaveRequest, api.DocumentView](ctx, c.http, c.base+"/api/documents/"+url.PathEscape(id)+"/save", req)
}

// Ask 知识库语义问答。
func (c *Workflow) Ask(ctx context.Context, req api.AskRequest) (api.AskResult, error) {
	return postJSON[api.AskRequest, api.AskResult](ctx, c.http, c.base+"/api/ask", req)
}

func (c *Workflow) Query(ctx context.Context, f store.QueryFilter) (api.QueryResult, error) {
	q := url.Values{}
	if f.DocType != "" {
		q.Set("doc_type", f.DocType)
	}
	if f.Status != "" {
		q.Set("status", f.Status)
	}
	if f.Keyword != "" {
		q.Set("keyword", f.Keyword)
	}
	if f.UploadedBy != "" {
		q.Set("uploaded_by", f.UploadedBy)
	}
	if len(f.FieldFilters) > 0 {
		if b, err := json.Marshal(f.FieldFilters); err == nil {
			q.Set("field_filters", string(b))
		}
	}
	if f.Limit > 0 {
		q.Set("limit", strconv.Itoa(f.Limit))
	}
	return getJSON[api.QueryResult](ctx, c.http, c.base+"/api/documents?"+q.Encode())
}
