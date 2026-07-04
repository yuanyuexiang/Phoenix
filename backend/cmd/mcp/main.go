// MCP Server 服务(说明书 §7):WorkBuddy 连接器,平台的主要使用入口。
// 五个 MCP 工具全部转调 workflow 服务的 REST API,自身无状态。
package main

import (
	"log/slog"
	"net/http"
	"os"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/yuanyuexiang/phoenix/internal/clients"
	"github.com/yuanyuexiang/phoenix/internal/config"
	"github.com/yuanyuexiang/phoenix/internal/httpx"
	"github.com/yuanyuexiang/phoenix/internal/mcpserver"
)

const version = "0.1.0"

func main() {
	cfg := config.Load()
	addr := cfg.HTTPAddr // 默认 :8080

	wf := clients.NewWorkflow(cfg.WorkflowBaseURL, cfg.AdminPassword)
	srv := mcpserver.New(wf, version)
	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return srv }, nil)

	mux := http.NewServeMux()
	mux.Handle("/mcp", handler)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	slog.Info("mcp 连接器配置", "endpoint", "/mcp", "workflow", cfg.WorkflowBaseURL)
	if err := httpx.Serve(addr, mux, "mcp 连接器"); err != nil {
		slog.Error("mcp 服务退出", "error", err)
		os.Exit(1)
	}
}
