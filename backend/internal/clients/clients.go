// Package clients 提供 Monorepo 内各服务的 HTTP 客户端,
// 供 workflow(调 parser/ai)与 mcp/admin(调 workflow)使用。
package clients

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/yuanyuexiang/phoenix/internal/api"
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

// Parser 是文档解析服务的客户端。
type Parser struct {
	base string
	http *http.Client
}

func NewParser(baseURL string) *Parser {
	return &Parser{base: strings.TrimRight(baseURL, "/"), http: &http.Client{Timeout: 2 * time.Minute}}
}

// Parse 上传文件字节,返回解析出的纯文本。
func (c *Parser) Parse(ctx context.Context, filename string, data []byte) (string, error) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	part, err := w.CreateFormFile("file", filename)
	if err != nil {
		return "", err
	}
	if _, err := part.Write(data); err != nil {
		return "", err
	}
	if err := w.Close(); err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/parse", &buf)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("parser 服务不可达: %w", err)
	}
	defer resp.Body.Close()
	out, err := decode[api.ParseResponse](resp)
	if err != nil {
		return "", fmt.Errorf("parser: %w", err)
	}
	return out.Text, nil
}

// AI 是 AI 字段提取服务的客户端。
type AI struct {
	base string
	http *http.Client
}

func NewAI(baseURL string) *AI {
	return &AI{base: strings.TrimRight(baseURL, "/"), http: &http.Client{Timeout: 3 * time.Minute}}
}

func (c *AI) Extract(ctx context.Context, req api.ExtractRequest) (api.ExtractResponse, error) {
	out, err := postJSON[api.ExtractRequest, api.ExtractResponse](ctx, c.http, c.base+"/extract", req)
	if err != nil {
		return api.ExtractResponse{}, fmt.Errorf("ai: %w", err)
	}
	return out, nil
}

func (c *AI) Classify(ctx context.Context, req api.ClassifyRequest) (api.ClassifyResponse, error) {
	out, err := postJSON[api.ClassifyRequest, api.ClassifyResponse](ctx, c.http, c.base+"/classify", req)
	if err != nil {
		return api.ClassifyResponse{}, fmt.Errorf("ai: %w", err)
	}
	return out, nil
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

func NewWorkflow(baseURL, accessKey string) *Workflow {
	return &Workflow{
		base: strings.TrimRight(baseURL, "/"),
		http: &http.Client{
			Timeout:   5 * time.Minute,
			Transport: accessKeyTransport{key: accessKey, base: http.DefaultTransport},
		},
	}
}

func (c *Workflow) Upload(ctx context.Context, req api.UploadRequest) (api.DocumentView, error) {
	return postJSON[api.UploadRequest, api.DocumentView](ctx, c.http, c.base+"/api/documents", req)
}

func (c *Workflow) Extract(ctx context.Context, id string) (api.DocumentView, error) {
	return postJSON[struct{}, api.DocumentView](ctx, c.http, c.base+"/api/documents/"+url.PathEscape(id)+"/extract", struct{}{})
}

func (c *Workflow) Validate(ctx context.Context, id string) (api.DocumentView, error) {
	return postJSON[struct{}, api.DocumentView](ctx, c.http, c.base+"/api/documents/"+url.PathEscape(id)+"/validate", struct{}{})
}

func (c *Workflow) Save(ctx context.Context, id string, req api.SaveRequest) (api.DocumentView, error) {
	return postJSON[api.SaveRequest, api.DocumentView](ctx, c.http, c.base+"/api/documents/"+url.PathEscape(id)+"/save", req)
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
	if f.Limit > 0 {
		q.Set("limit", strconv.Itoa(f.Limit))
	}
	return getJSON[api.QueryResult](ctx, c.http, c.base+"/api/documents?"+q.Encode())
}
