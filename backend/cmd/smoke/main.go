// smoke 是端到端冒烟客户端:模拟 WorkBuddy 按顺序调用五个 MCP 工具,
// 用 samples/sample-generic.txt 走完整条流水线。服务须已启动:
//
//	make infra-up && make run     # 或 make compose-up
//	go run ./cmd/smoke [-addr http://localhost:8080/mcp]
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func main() {
	addr := flag.String("addr", "http://localhost:8080/mcp", "Phoenix MCP 端点")
	sample := flag.String("sample", "samples/sample-generic.txt", "样例文档路径")
	flag.Parse()

	ctx := context.Background()
	client := mcp.NewClient(&mcp.Implementation{Name: "workbuddy-sim", Version: "0.0.1"}, nil)
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: *addr}, nil)
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

	content, err := os.ReadFile(*sample)
	if err != nil {
		log.Fatal(err)
	}

	up := call(ctx, session, "upload_document", map[string]any{
		"doc_type":     "generic",
		"filename":     "sample-generic.txt",
		"content_text": string(content),
	})
	id := up["id"].(string)

	call(ctx, session, "extract_fields", map[string]any{"document_id": id})
	call(ctx, session, "validate_document", map[string]any{"document_id": id})
	call(ctx, session, "save_database", map[string]any{"document_id": id})
	call(ctx, session, "query_document", map[string]any{"keyword": "采购项目", "limit": 5})

	fmt.Println("\n✅ 五个工具全部调用成功,流水线端到端跑通")
}

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
