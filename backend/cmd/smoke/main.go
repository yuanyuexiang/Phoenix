// smoke 是端到端冒烟客户端:模拟 WorkBuddy 按顺序调用五个 MCP 工具,
// 用 samples/sample-generic.txt 走完整条流水线。服务须已启动:
//
//	make infra-up && make run     # 或 make compose-up
//	go run ./cmd/smoke [-addr http://localhost:8080/mcp]
//
// OAuth 冒烟(mcp 以 PHX_OAUTH_MODE=required/optional 启动,见 make smoke-oauth):
//
//	go run ./cmd/smoke -oauth-issuer http://localhost:8180/realms/phoenix \
//	    -oauth-user alice -oauth-pass alice123 -require-auth
//
// 取 token 走标准 OIDC discovery + password grant(Keycloak 测试客户端开了
// Direct Access Grant),也可用 -token 直接传现成 token。
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func main() {
	addr := flag.String("addr", "http://localhost:8080/mcp", "Phoenix MCP 端点")
	sample := flag.String("sample", "samples/sample-generic.txt", "样例文档路径")
	rag := flag.Bool("rag", false, "追加 ask_document 语义问答用例(workflow 须已配置 PHX_EMBED_*)")
	token := flag.String("token", "", "现成的 access token(优先于 -oauth-* 取 token)")
	oauthIssuer := flag.String("oauth-issuer", "", "授权服务器 issuer,设置后用 password grant 取 token")
	oauthClient := flag.String("oauth-client", "phoenix-smoke", "OAuth 客户端 ID(须开 Direct Access Grant)")
	oauthUser := flag.String("oauth-user", "", "测试用户名")
	oauthPass := flag.String("oauth-pass", "", "测试用户密码")
	requireAuth := flag.Bool("require-auth", false, "先验证无 token 会被拒绝(mcp 须为 required 模式)")
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

	if *requireAuth {
		if err := tryConnect(ctx, *addr, ""); err == nil {
			log.Fatal("负向验证失败:不带 token 竟能连接,请确认 mcp 以 PHX_OAUTH_MODE=required 启动")
		} else {
			fmt.Printf("  ✓ 无 token 连接被拒绝(%v)\n", truncateErr(err))
		}
	}

	client := mcp.NewClient(&mcp.Implementation{Name: "workbuddy-sim", Version: "0.0.1"}, nil)
	session, err := client.Connect(ctx, transportFor(*addr, *token), nil)
	if err != nil {
		log.Fatalf("连接 MCP 失败: %v", err)
	}
	defer session.Close()

	tools, err := session.ListTools(ctx, nil)
	if err != nil {
		log.Fatalf("tools/list 失败: %v", err)
	}
	fmt.Println("== 可用工具 ==")
	for _, t := range tools.Tools {
		fmt.Printf("  - %s\n", t.Name)
	}

	// 「连接器即专家包」验证:服务器应内嵌专家提示词(prompts + instructions)
	prompts, err := session.ListPrompts(ctx, nil)
	if err != nil {
		log.Fatalf("prompts/list 失败: %v", err)
	}
	fmt.Println("== 内嵌专家(prompts)==")
	for _, p := range prompts.Prompts {
		fmt.Printf("  - %s(%s)\n", p.Name, p.Title)
	}
	got, err := session.GetPrompt(ctx, &mcp.GetPromptParams{Name: "document-expert"})
	if err != nil {
		log.Fatalf("prompts/get document-expert 失败: %v", err)
	}
	if len(got.Messages) == 0 {
		log.Fatal("document-expert 提示词为空")
	}
	fmt.Println("  ✓ document-expert 提示词可获取")

	content, err := os.ReadFile(*sample)
	if err != nil {
		log.Fatal(err)
	}

	// 1. 上传归档(WorkBuddy 把原件传给平台留存)
	up := call(ctx, session, "upload_document", map[string]any{
		"doc_type":     "generic",
		"filename":     "sample-generic.txt",
		"content_text": string(content),
	})
	id := up["id"].(string)

	// 身份落库断言:带 token 时 uploaded_by 应为登录用户
	if *token != "" && *oauthUser != "" {
		if up["uploaded_by"] != *oauthUser {
			log.Fatalf("uploaded_by 应为 %q,得到 %v", *oauthUser, up["uploaded_by"])
		}
		fmt.Printf("\n✅ 身份落库:uploaded_by = %s\n", *oauthUser)
	}

	// 2. 取字段清单(后端下发抽取指令,不识别)
	brief := call(ctx, session, "extract_fields", map[string]any{"document_id": id})
	if bf, _ := brief["fields"].([]any); len(bf) == 0 {
		log.Fatalf("extract_fields 应返回字段清单,得到 %v", brief)
	}
	fmt.Println("  ✓ extract_fields 返回字段清单")

	// 3. 模拟 WorkBuddy 已识别:抽好的字段 + 转写的正文
	fields := []map[string]any{
		{"name": "doc_no", "value": "PHX-2026-0001"},
		{"name": "title", "value": "企业文档处理平台采购项目"},
		{"name": "amount", "value": "128,000.00"},
		{"name": "issue_date", "value": "2026年7月1日"},
		{"name": "party_a", "value": "某某科技有限公司"},
		{"name": "party_b", "value": "凤凰软件服务有限公司"},
	}

	// 4. 预校验(应通过)
	vres := call(ctx, session, "validate_document", map[string]any{
		"document_id": id, "fields": fields, "doc_type": "generic",
	})
	if vres["status"] != "validated" {
		log.Fatalf("validate_document 应为 validated,得到 %v(issues=%v)", vres["status"], vres["issues"])
	}
	fmt.Println("  ✓ validate_document = validated")

	// 5. 入库(带正文,应 saved)
	sres := call(ctx, session, "save_database", map[string]any{
		"document_id": id, "fields": fields, "doc_type": "generic",
		"content_text": string(content),
	})
	if sres["status"] != "saved" {
		log.Fatalf("save_database 应为 saved,得到 %v", sres["status"])
	}
	fmt.Println("  ✓ save_database = saved")

	// 6. 按关键词查询(命中证明正文已落库)
	q := map[string]any{"keyword": "采购项目", "limit": 5}
	if *token != "" && *oauthUser != "" {
		q["uploaded_by"] = *oauthUser
	}
	qres := call(ctx, session, "query_document", q)
	if total, _ := qres["total"].(float64); total < 1 {
		log.Fatalf("query_document 应命中已入库文档,得到 total=%v", qres["total"])
	}
	fmt.Println("  ✓ query_document 命中(正文已落库)")

	// 7. 字段级过滤:金额 > 10000(数值比较,自动去千分位逗号)+ 甲方包含「科技」
	fres := call(ctx, session, "query_document", map[string]any{
		"doc_type": "generic",
		"field_filters": []map[string]any{
			{"field": "amount", "op": "gt", "value": "10000"},
			{"field": "party_a", "op": "contains", "value": "科技"},
		},
	})
	if total, _ := fres["total"].(float64); total < 1 {
		log.Fatalf("字段过滤(金额>10000 且甲方含科技)应命中,得到 total=%v", fres["total"])
	}
	fmt.Println("  ✓ query_document 字段级过滤命中(金额>10000 且甲方含「科技」)")

	// 反向:金额 > 999999 应查不到
	nres := call(ctx, session, "query_document", map[string]any{
		"doc_type":      "generic",
		"field_filters": []map[string]any{{"field": "amount", "op": "gt", "value": "999999"}},
	})
	if total, _ := nres["total"].(float64); total != 0 {
		log.Fatalf("金额>999999 应无命中,得到 total=%v", nres["total"])
	}
	fmt.Println("  ✓ query_document 字段级过滤反向验证(金额>999999 无命中)")

	// 8. 知识库语义问答(-rag,workflow 须配置 PHX_EMBED_*;save 时已把正文切片+向量入库)
	if *rag {
		ares := call(ctx, session, "ask_document", map[string]any{
			"question": "这个采购项目的金额是多少?", "limit": 3,
		})
		total, _ := ares["total"].(float64)
		if total < 1 {
			log.Fatalf("ask_document 应命中知识库片段,得到 total=%v", ares["total"])
		}
		chunks, _ := ares["chunks"].([]any)
		first, _ := chunks[0].(map[string]any)
		if !strings.Contains(fmt.Sprint(first["content"]), "采购项目") {
			log.Fatalf("ask_document 命中片段应含正文,得到 %v", first["content"])
		}
		fmt.Printf("  ✓ ask_document 语义问答命中(来源 %v,score=%.3f)\n", first["filename"], first["score"])
	}

	fmt.Println("\n✅ 全流程跑通:WorkBuddy 驱动写入/校验 + 结构化查询(含字段级过滤)" + ragTip(*rag))
}

func ragTip(rag bool) string {
	if rag {
		return " + 知识库语义问答"
	}
	return ""
}

/* ---------- OAuth ---------- */

// bearerTransport 给每个出站请求带上 Authorization: Bearer <token>。
type bearerTransport struct {
	token string
}

func (t bearerTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r.Header.Set("Authorization", "Bearer "+t.token)
	return http.DefaultTransport.RoundTrip(r)
}

func transportFor(addr, token string) *mcp.StreamableClientTransport {
	tr := &mcp.StreamableClientTransport{Endpoint: addr}
	if token != "" {
		tr.HTTPClient = &http.Client{Transport: bearerTransport{token: token}}
	}
	return tr
}

// tryConnect 尝试建立 MCP 会话(用于负向验证),成功则立即关闭。
func tryConnect(ctx context.Context, addr, token string) error {
	client := mcp.NewClient(&mcp.Implementation{Name: "workbuddy-sim-noauth", Version: "0.0.1"}, nil)
	session, err := client.Connect(ctx, transportFor(addr, token), nil)
	if err != nil {
		return err
	}
	_ = session.Close()
	return nil
}

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

func truncateErr(err error) string {
	s := err.Error()
	if len(s) > 120 {
		return s[:120] + "..."
	}
	return s
}

/* ---------- 工具调用 ---------- */

func call(ctx context.Context, s *mcp.ClientSession, tool string, args map[string]any) map[string]any {
	res, err := s.CallTool(ctx, &mcp.CallToolParams{Name: tool, Arguments: args})
	if err != nil {
		log.Fatalf("%s 调用失败: %v", tool, err)
	}
	if res.IsError {
		log.Fatalf("%s 返回错误: %s", tool, textOf(res))
	}
	fmt.Printf("\n== %s ==\n%s\n", tool, textOf(res))

	var out map[string]any
	if res.StructuredContent != nil {
		raw, _ := json.Marshal(res.StructuredContent)
		_ = json.Unmarshal(raw, &out)
	}
	return out
}

func textOf(res *mcp.CallToolResult) string {
	for _, c := range res.Content {
		if t, ok := c.(*mcp.TextContent); ok {
			return t.Text
		}
	}
	return "(无文本内容)"
}
