// 工作流引擎服务(说明书 §7):编排文档处理流水线,持有 PostgreSQL 与 MinIO。
// 本入口只做依赖装配;REST API 路由与鉴权见 internal/workflowapi。
package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"

	"github.com/yuanyuexiang/phoenix/internal/config"
	"github.com/yuanyuexiang/phoenix/internal/embed"
	"github.com/yuanyuexiang/phoenix/internal/httpx"
	"github.com/yuanyuexiang/phoenix/internal/pipeline"
	"github.com/yuanyuexiang/phoenix/internal/restapi"
	"github.com/yuanyuexiang/phoenix/internal/schema"
	"github.com/yuanyuexiang/phoenix/internal/store"
	"github.com/yuanyuexiang/phoenix/internal/workflowapi"
)

func main() {
	if err := run(); err != nil {
		slog.Error("workflow 启动失败", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg := config.Load()
	addr := os.Getenv("PHX_WORKFLOW_ADDR")
	if addr == "" {
		addr = ":8081"
	}
	ctx := context.Background()

	registry, err := schema.Load(cfg.DoctypesDir)
	if err != nil {
		return err
	}
	slog.Info("单据类型已加载", "types", registry.Names())

	db, err := store.Open(ctx, cfg.DatabaseDSN)
	if err != nil {
		return err
	}
	defer db.Close()

	objects, err := store.OpenObjects(ctx, cfg.MinioEndpoint, cfg.MinioAccessKey, cfg.MinioSecretKey, cfg.MinioBucket, cfg.MinioUseSSL)
	if err != nil {
		return err
	}

	var embedder pipeline.Embedder // nil = 知识库关闭
	if cfg.EmbedEndpoint != "" {
		embedder = embed.New(cfg.EmbedEndpoint, cfg.EmbedAPIKey, cfg.EmbedModel, cfg.EmbedDim)
		slog.Info("知识库 embedding 就绪", "model", cfg.EmbedModel, "dim", cfg.EmbedDim)
	} else {
		slog.Warn("未配置 PHX_EMBED_ENDPOINT,知识库未启用(ask_document 将报未启用)")
	}

	pipe := &pipeline.Pipeline{
		DB:            db,
		Objects:       objects,
		Registry:      registry,
		Embedder:      embedder,
		MinConfidence: cfg.MinConfidence,
	}

	// 既有的内部 REST 面(X-Access-Key;前端 + mcp 在用),行为不变。
	handler := workflowapi.NewHandler(workflowapi.Options{
		Pipeline:      pipe,
		Registry:      registry,
		DB:            db,
		AdminPassword: cfg.AdminPassword,
		// 识别已移至 WorkBuddy;后端不再依赖 parser/ai 服务,健康聚合只剩自身 + 存储。
	})

	// 员工级公网 REST 面 /pub/v1(Keycloak Bearer;新 phoenix-doc-assistant 专家用)。
	// Issuer 未配置则不挂载 —— 老部署完全不受影响。挂载时用一个顶层 mux 按前缀分流,
	// 既有 /api、/healthz 等仍全部交给 workflowapi,一个字节都不改。
	if cfg.APIOIDCIssuer != "" {
		verifier, err := restapi.NewVerifier(ctx, cfg.APIOIDCIssuer, cfg.APIOIDCDiscoveryURL, cfg.APIOIDCAudience)
		if err != nil {
			return err
		}
		restHandler := restapi.NewHandler(restapi.Options{Pipeline: pipe, DB: db, Verifier: verifier})
		root := http.NewServeMux()
		root.Handle("/pub/v1/", restHandler)
		root.Handle("/", handler)
		handler = root
		slog.Info("员工级 REST 面 /pub/v1 已启用(Keycloak OAuth 资源服务器)", "issuer", cfg.APIOIDCIssuer, "audience", cfg.APIOIDCAudience)
	} else {
		slog.Info("未配置 PHX_API_OIDC_ISSUER,/pub/v1 员工级 REST 面未启用")
	}

	return httpx.Serve(addr, handler, "workflow 工作流引擎")
}
