// MCP Server 服务(说明书 §7):WorkBuddy 连接器,平台的主要使用入口。
// 五个 MCP 工具全部转调 workflow 服务的 REST API,自身无状态。
//
// 对外鉴权(docs/MCP-OAuth鉴权方案.md):PHX_OAUTH_MODE ≠ off 时,/mcp 要求
// OAuth 2.1 Bearer token(验签走授权服务器 JWKS),并发布 RFC 9728 资源元数据
// 端点供客户端自动发现授权服务器;/healthz 始终开放。
package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/yuanyuexiang/phoenix/internal/clients"
	"github.com/yuanyuexiang/phoenix/internal/config"
	"github.com/yuanyuexiang/phoenix/internal/httpx"
	"github.com/yuanyuexiang/phoenix/internal/mcpauth"
	"github.com/yuanyuexiang/phoenix/internal/mcpserver"
)

const version = "0.1.0"

func main() {
	cfg := config.Load()
	addr := cfg.HTTPAddr // 默认 :8080

	wf := clients.NewWorkflow(cfg.WorkflowBaseURL, cfg.AdminPassword)
	srv := mcpserver.New(wf, version)
	var handler http.Handler = mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return srv }, nil)

	mux := http.NewServeMux()

	oc := mcpauth.Config{
		Mode:         cfg.OAuthMode,
		Issuer:       cfg.OAuthIssuer,
		DiscoveryURL: cfg.OAuthDiscoveryURL,
		Audience:     cfg.OAuthAudience,
		Resource:     cfg.OAuthResource,
		Scopes:       strings.Fields(cfg.OAuthScopes),
	}
	if err := oc.Validate(); err != nil {
		slog.Error("OAuth 配置无效", "error", err)
		os.Exit(1)
	}
	if oc.Mode != mcpauth.ModeOff {
		verifier, err := mcpauth.NewVerifier(context.Background(), oc)
		if err != nil {
			slog.Error("连接授权服务器失败", "error", err)
			os.Exit(1)
		}
		handler = mcpauth.Middleware(oc, verifier)(handler)
		// RFC 9728 元数据:同时挂根路径与路径插入两种形态
		// (/.well-known/oauth-protected-resource 与 …/mcp)
		meta := mcpauth.MetadataHandler(oc)
		mux.Handle("/.well-known/oauth-protected-resource", meta)
		mux.Handle("/.well-known/oauth-protected-resource/", meta)
		slog.Info("MCP OAuth 鉴权已启用",
			"mode", oc.Mode, "issuer", oc.Issuer, "audience", oc.Audience, "resource", oc.Resource)
	} else {
		slog.Warn("MCP 端点未启用 OAuth 鉴权(PHX_OAUTH_MODE=off),对外发布前必须开启")
	}

	mux.Handle("/mcp", handler)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	slog.Info("mcp 连接器配置", "endpoint", "/mcp", "workflow", cfg.WorkflowBaseURL, "oauth", oc.Mode)
	if err := httpx.Serve(addr, mux, "mcp 连接器"); err != nil {
		slog.Error("mcp 服务退出", "error", err)
		os.Exit(1)
	}
}
