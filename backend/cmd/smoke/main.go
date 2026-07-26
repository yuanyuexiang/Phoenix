// smoke 是端到端冒烟客户端:模拟专家客户端按顺序调用 REST 接口,
// 用 samples/sample-generic.txt 走完整条流水线。服务须已启动:
//
//	make infra-up && make run-workflow     # 或 make compose-up
//	go run ./cmd/smoke [-addr http://localhost:8081]
//
// 默认走管理面 /api(X-Access-Key);带 -user/-pass 或 -token 时切到员工面 /pub/v1
// (自研账号登录换 JWT,与 phoenix-doc-assistant 专家的真实调用路径一致,见 make smoke-auth)。
// workflow 须以 PHX_AUTH_SECRET 启动,否则 /pub/v1 未挂载:
//
//	go run ./cmd/smoke -user alice -pass alice123 -require-auth
//
// -user 模式会先用 X-Access-Key 经 /api/users 确保测试账号存在(已存在则重置口令),
// 再走 /pub/v1/auth/login 登录 —— 一条命令即可自证"账号管理 + 登录 + 身份落库"整条链路。
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
	token := flag.String("token", "", "现成的 access token(优先于 -user/-pass;设置后走 /pub/v1)")
	user := flag.String("user", "", "员工用户名,设置后先确保账号存在再登录 /pub/v1")
	pass := flag.String("pass", "", "员工口令(与 -user 配套)")
	requireAuth := flag.Bool("require-auth", false, "先验证 /pub/v1 无 token 会被拒绝(workflow 须已启用 /pub/v1)")
	flag.Parse()

	ctx := context.Background()
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

	if *token == "" && *user != "" {
		if *pass == "" {
			log.Fatal("请同时提供 -user 与 -pass")
		}
		ensureUser(ctx, c, *user, *pass)
		c.token = login(ctx, c, *user, *pass)
		fmt.Println("== 登录 ==")
		fmt.Printf("  ✓ %s 已登录 /pub/v1(自研账号 + JWT)\n", *user)
	}

	// 员工面先自省身份(等价于客户端登录确认)
	if c.token != "" {
		me := c.call(ctx, http.MethodGet, "/me", nil)
		fmt.Printf("  ✓ /pub/v1/me 身份: %v\n", me["display"])
		if *user != "" && me["username"] != *user {
			log.Fatalf("me.username 应为 %q,得到 %v", *user, me["username"])
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
	if c.token != "" && *user != "" {
		if up["uploaded_by"] != *user {
			log.Fatalf("uploaded_by 应为 %q,得到 %v", *user, up["uploaded_by"])
		}
		fmt.Printf("\n✅ 身份落库:uploaded_by = %s\n", *user)
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
	if c.token != "" && *user != "" {
		q.Set("uploaded_by", *user)
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
	if c.token != "" {
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

/* ---------- 账号确保与登录(走管理面 /api/users + /pub/v1/auth/login) ---------- */

// ensureUser 用 X-Access-Key 确保测试账号存在:不存在则创建,已存在则重置口令,
// 保证随后的登录必然成功(同时覆盖了员工管理端点本身)。
func ensureUser(ctx context.Context, c *restClient, username, password string) {
	body := map[string]any{"username": username, "password": password, "display_name": "冒烟测试员工"}
	status, data, err := c.adminJSON(ctx, http.MethodPost, "/api/users", body)
	if err != nil {
		log.Fatalf("创建测试账号失败: %v", err)
	}
	switch {
	case status < 300:
		fmt.Printf("  ✓ 已创建测试账号 %s\n", username)
	case status == http.StatusConflict:
		// 已存在 → 查 id 并重置口令
		_, listRaw, err := c.adminJSON(ctx, http.MethodGet, "/api/users", nil)
		if err != nil {
			log.Fatalf("查询员工列表失败: %v", err)
		}
		var list struct {
			Users []struct {
				ID       int64  `json:"id"`
				Username string `json:"username"`
			} `json:"users"`
		}
		if err := json.Unmarshal(listRaw, &list); err != nil {
			log.Fatalf("员工列表解析失败: %v", err)
		}
		for _, u := range list.Users {
			if u.Username == username {
				st, _, err := c.adminJSON(ctx, http.MethodPost, fmt.Sprintf("/api/users/%d/password", u.ID),
					map[string]any{"password": password})
				if err != nil || st >= 300 {
					log.Fatalf("重置测试账号口令失败(HTTP %d): %v", st, err)
				}
				fmt.Printf("  ✓ 测试账号 %s 已存在,口令已重置\n", username)
				return
			}
		}
		log.Fatalf("账号 %s 显示已存在但列表中未找到", username)
	default:
		log.Fatalf("创建测试账号返回 HTTP %d: %s", status, truncate(string(data), 200))
	}
}

// login 走员工面登录,返回 access token。
func login(ctx context.Context, c *restClient, username, password string) string {
	status, data, err := c.raw(ctx, http.MethodPost, c.base+"/pub/v1/auth/login",
		map[string]any{"username": username, "password": password}, false)
	if err != nil {
		log.Fatalf("登录请求失败: %v", err)
	}
	if status != http.StatusOK {
		log.Fatalf("登录失败 HTTP %d: %s", status, truncate(string(data), 200))
	}
	var out struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(data, &out); err != nil || out.AccessToken == "" {
		log.Fatalf("登录响应解析失败: %v", err)
	}
	return out.AccessToken
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

// adminJSON 强制走管理面(X-Access-Key),无论是否已持有员工 token。
func (c *restClient) adminJSON(ctx context.Context, method, path string, body map[string]any) (int, []byte, error) {
	admin := &restClient{base: c.base, accessKey: c.accessKey}
	return admin.raw(ctx, method, c.base+path, body, true)
}

// raw 发送一次请求;withAuth=false 用于负向验证/登录(不带任何凭证)。
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

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
