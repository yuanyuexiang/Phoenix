// smoke 是端到端冒烟客户端:模拟专家客户端按顺序调用 REST 接口,
// 用 samples/sample-generic.txt 走完整条流水线。服务须已启动:
//
//	make infra-up && make run-workflow     # 或 make compose-up
//	go run ./cmd/smoke [-addr http://localhost:8081]
//
// 默认走管理面 /api(X-Access-Key);带 -token 或 -oauth-* 时切到员工级 /pub/v1
// (Bearer 鉴权,与 phoenix-doc-assistant 专家的真实调用路径一致,见 make smoke-oauth)。
// workflow 须以 PHX_API_OIDC_ISSUER 启动,否则 /pub/v1 未挂载:
//
//	go run ./cmd/smoke -oauth-issuer http://localhost:8180/realms/phoenix \
//	    -oauth-user alice -oauth-pass alice123 -require-auth
//
// 取 token 走标准 OIDC discovery + password grant(Keycloak 测试客户端开了
// Direct Access Grant),也可用 -token 直接传现成 token。
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
)

func main() {
	addr := flag.String("addr", "http://localhost:8081", "workflow 服务地址")
	sample := flag.String("sample", "samples/sample-generic.txt", "样例文档路径")
	rag := flag.Bool("rag", false, "追加知识库语义问答用例(workflow 须已配置 PHX_EMBED_*)")
	accessKey := flag.String("access-key", "phoenix123", "管理面 /api 的 X-Access-Key(与 PHX_ADMIN_PASSWORD 一致)")
	token := flag.String("token", "", "现成的 access token(优先于 -oauth-* 取 token;设置后走 /pub/v1)")
	oauthIssuer := flag.String("oauth-issuer", "", "授权服务器 issuer,设置后用 password grant 取 token 并走 /pub/v1")
	oauthClient := flag.String("oauth-client", "phoenix-smoke", "OAuth 客户端 ID(须开 Direct Access Grant)")
	oauthUser := flag.String("oauth-user", "", "测试用户名")
	oauthPass := flag.String("oauth-pass", "", "测试用户密码")
	requireAuth := flag.Bool("require-auth", false, "先验证 /pub/v1 无 token 会被拒绝(workflow 须已启用 /pub/v1)")
	flag.Parse()

	ctx := context.Background()

	if *token == "" && *oauthIssuer != "" {
		t, err := fetchToken(ctx, *oauthIssuer, *oauthClient, *oauthUser, *oauthPass)
		if err != nil {
			log.Fatalf("取 token 失败: %v", err)
		}
		*token = t
		fmt.Println("== OAuth ==")
		fmt.Printf("  ✓ 已从 %s 取得 %s 的 access token\n", *oauthIssuer, *oauthUser)
	}

	c := &restClient{base: strings.TrimRight(*addr, "/"), token: *token, accessKey: *accessKey}

	if *requireAuth {
		status, _, err := c.raw(ctx, http.MethodGet, c.base+"/pub/v1/me", nil, false)
		if err != nil {
			log.Fatalf("负向验证请求失败: %v", err)
		}
		if status != http.StatusUnauthorized {
			log.Fatalf("负向验证失败:无 token 访问 /pub/v1/me 应 401,得到 HTTP %d", status)
		}
		fmt.Println("  ✓ 无 token 访问 /pub/v1 被拒绝(HTTP 401)")
	}

	// 员工面先自省身份(等价于客户端登录确认)
	if *token != "" {
		me := c.call(ctx, http.MethodGet, "/me", nil)
		fmt.Printf("  ✓ /pub/v1/me 身份: %v\n", me["display"])
		if *oauthUser != "" && me["username"] != *oauthUser {
			log.Fatalf("me.username 应为 %q,得到 %v", *oauthUser, me["username"])
		}
	}

	content, err := os.ReadFile(*sample)
	if err != nil {
		log.Fatal(err)
	}

	// 1. 上传归档(专家把原件传给平台留存)
	up := c.call(ctx, http.MethodPost, "/documents", map[string]any{
		"doc_type":     "generic",
		"filename":     "sample-generic.txt",
		"content_text": string(content),
	})
	id := up["id"].(string)

	// 身份落库断言:员工面 uploaded_by 应为登录用户
	if *token != "" && *oauthUser != "" {
		if up["uploaded_by"] != *oauthUser {
			log.Fatalf("uploaded_by 应为 %q,得到 %v", *oauthUser, up["uploaded_by"])
		}
		fmt.Printf("\n✅ 身份落库:uploaded_by = %s\n", *oauthUser)
	}

	// 2. 取字段清单(后端下发抽取指令,不识别)
	brief := c.call(ctx, http.MethodPost, "/documents/"+id+"/extract", map[string]any{})
	if bf, _ := brief["fields"].([]any); len(bf) == 0 {
		log.Fatalf("extract 应返回字段清单,得到 %v", brief)
	}
	fmt.Println("  ✓ extract 返回字段清单")

	// 3. 模拟专家已识别:抽好的字段 + 转写的正文
	fields := []map[string]any{
		{"name": "doc_no", "value": "PHX-2026-0001"},
		{"name": "title", "value": "企业文档处理平台采购项目"},
		{"name": "amount", "value": "128,000.00"},
		{"name": "issue_date", "value": "2026年7月1日"},
		{"name": "party_a", "value": "某某科技有限公司"},
		{"name": "party_b", "value": "凤凰软件服务有限公司"},
	}

	// 4. 预校验(应通过)
	vres := c.call(ctx, http.MethodPost, "/documents/"+id+"/validate", map[string]any{
		"fields": fields, "doc_type": "generic",
	})
	if vres["status"] != "validated" {
		log.Fatalf("validate 应为 validated,得到 %v(issues=%v)", vres["status"], vres["issues"])
	}
	fmt.Println("  ✓ validate = validated")

	// 5. 入库(带正文,应 saved)
	sres := c.call(ctx, http.MethodPost, "/documents/"+id+"/save", map[string]any{
		"fields": fields, "doc_type": "generic", "content_text": string(content),
	})
	if sres["status"] != "saved" {
		log.Fatalf("save 应为 saved,得到 %v", sres["status"])
	}
	fmt.Println("  ✓ save = saved")

	// 6. 按关键词查询(命中证明正文已落库)
	q := url.Values{"keyword": {"采购项目"}, "limit": {"5"}}
	if *token != "" && *oauthUser != "" {
		q.Set("uploaded_by", *oauthUser)
	}
	qres := c.call(ctx, http.MethodGet, "/documents?"+q.Encode(), nil)
	if total, _ := qres["total"].(float64); total < 1 {
		log.Fatalf("query 应命中已入库文档,得到 total=%v", qres["total"])
	}
	fmt.Println("  ✓ query 命中(正文已落库)")

	// 7. 字段级过滤:金额 > 10000(数值比较,自动去千分位逗号)+ 甲方包含「科技」
	fres := c.call(ctx, http.MethodGet, "/documents?"+queryWithFilters("generic", []map[string]any{
		{"field": "amount", "op": "gt", "value": "10000"},
		{"field": "party_a", "op": "contains", "value": "科技"},
	}), nil)
	if total, _ := fres["total"].(float64); total < 1 {
		log.Fatalf("字段过滤(金额>10000 且甲方含科技)应命中,得到 total=%v", fres["total"])
	}
	fmt.Println("  ✓ query 字段级过滤命中(金额>10000 且甲方含「科技」)")

	// 反向:金额 > 999999 应查不到
	nres := c.call(ctx, http.MethodGet, "/documents?"+queryWithFilters("generic", []map[string]any{
		{"field": "amount", "op": "gt", "value": "999999"},
	}), nil)
	if total, _ := nres["total"].(float64); total != 0 {
		log.Fatalf("金额>999999 应无命中,得到 total=%v", nres["total"])
	}
	fmt.Println("  ✓ query 字段级过滤反向验证(金额>999999 无命中)")

	// 8. 知识库语义问答(-rag,workflow 须配置 PHX_EMBED_*;save 时已把正文切片+向量入库)
	if *rag {
		ares := c.call(ctx, http.MethodPost, "/ask", map[string]any{
			"question": "这个采购项目的金额是多少?", "limit": 3,
		})
		total, _ := ares["total"].(float64)
		if total < 1 {
			log.Fatalf("ask 应命中知识库片段,得到 total=%v", ares["total"])
		}
		chunks, _ := ares["chunks"].([]any)
		first, _ := chunks[0].(map[string]any)
		if !strings.Contains(fmt.Sprint(first["content"]), "采购项目") {
			log.Fatalf("ask 命中片段应含正文,得到 %v", first["content"])
		}
		fmt.Printf("  ✓ ask 语义问答命中(来源 %v,score=%.3f)\n", first["filename"], first["score"])
	}

	face := "管理面 /api"
	if *token != "" {
		face = "员工面 /pub/v1"
	}
	fmt.Printf("\n✅ 全流程跑通(%s):专家驱动写入/校验 + 结构化查询(含字段级过滤)%s\n", face, ragTip(*rag))
}

func ragTip(rag bool) string {
	if rag {
		return " + 知识库语义问答"
	}
	return ""
}

// queryWithFilters 组装带 field_filters(JSON)的查询串。
func queryWithFilters(docType string, filters []map[string]any) string {
	raw, _ := json.Marshal(filters)
	q := url.Values{"doc_type": {docType}, "field_filters": {string(raw)}}
	return q.Encode()
}

/* ---------- REST 客户端 ---------- */

// restClient 按有无 token 选择 API 面:有 token 走员工面 /pub/v1(Bearer),
// 否则走管理面 /api(X-Access-Key)。两面路由与请求/响应形状一致。
type restClient struct {
	base      string
	token     string
	accessKey string
}

func (c *restClient) prefix() string {
	if c.token != "" {
		return "/pub/v1"
	}
	return "/api"
}

// call 请求 prefix+path,4xx/5xx 或解析失败直接终止冒烟。
func (c *restClient) call(ctx context.Context, method, path string, body map[string]any) map[string]any {
	u := c.base + c.prefix() + path
	status, data, err := c.raw(ctx, method, u, body, true)
	if err != nil {
		log.Fatalf("%s %s 请求失败: %v", method, path, err)
	}
	if status >= 400 {
		log.Fatalf("%s %s 返回 HTTP %d: %s", method, path, status, truncate(string(data), 300))
	}
	fmt.Printf("\n== %s %s ==\n%s\n", method, path, truncate(string(data), 600))

	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		log.Fatalf("%s %s 响应解析失败: %v", method, path, err)
	}
	return out
}

// raw 发送一次请求;withAuth=false 用于负向验证(不带任何凭证)。
func (c *restClient) raw(ctx context.Context, method, u string, body map[string]any, withAuth bool) (int, []byte, error) {
	var rd io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return 0, nil, err
		}
		rd = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, rd)
	if err != nil {
		return 0, nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if withAuth {
		if c.token != "" {
			req.Header.Set("Authorization", "Bearer "+c.token)
		} else if c.accessKey != "" {
			req.Header.Set("X-Access-Key", c.accessKey)
		}
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if err != nil {
		return 0, nil, err
	}
	return resp.StatusCode, data, nil
}

/* ---------- OAuth ---------- */

// fetchToken 走标准 OIDC:discovery 拿 token_endpoint,再用 password grant 换 token
// (不写死 Keycloak 路径,任何支持 Direct Access Grant 的 AS 均可)。
func fetchToken(ctx context.Context, issuer, clientID, username, password string) (string, error) {
	if username == "" || password == "" {
		return "", fmt.Errorf("请同时提供 -oauth-user 与 -oauth-pass")
	}
	discoURL := strings.TrimRight(issuer, "/") + "/.well-known/openid-configuration"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, discoURL, nil)
	if err != nil {
		return "", err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("授权服务器不可达(%s): %w", discoURL, err)
	}
	defer resp.Body.Close()
	var disco struct {
		TokenEndpoint string `json:"token_endpoint"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&disco); err != nil || disco.TokenEndpoint == "" {
		return "", fmt.Errorf("解析 OIDC discovery 失败(HTTP %d)", resp.StatusCode)
	}

	form := url.Values{
		"grant_type": {"password"},
		"client_id":  {clientID},
		"username":   {username},
		"password":   {password},
		"scope":      {"openid profile email"},
	}
	tokReq, err := http.NewRequestWithContext(ctx, http.MethodPost, disco.TokenEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	tokReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	tokResp, err := http.DefaultClient.Do(tokReq)
	if err != nil {
		return "", err
	}
	defer tokResp.Body.Close()
	var out struct {
		AccessToken      string `json:"access_token"`
		Error            string `json:"error"`
		ErrorDescription string `json:"error_description"`
	}
	if err := json.NewDecoder(tokResp.Body).Decode(&out); err != nil {
		return "", err
	}
	if out.AccessToken == "" {
		return "", fmt.Errorf("token 端点返回 HTTP %d: %s %s", tokResp.StatusCode, out.Error, out.ErrorDescription)
	}
	return out.AccessToken, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
