// 工作流引擎服务(说明书 §7):编排文档处理流水线,持有 PostgreSQL 与 MinIO。
// 本入口只做依赖装配;REST API 路由与鉴权见 internal/workflowapi。
package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/yuanyuexiang/phoenix/internal/clients"
	"github.com/yuanyuexiang/phoenix/internal/config"
	"github.com/yuanyuexiang/phoenix/internal/httpx"
	"github.com/yuanyuexiang/phoenix/internal/pipeline"
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

	handler := workflowapi.NewHandler(workflowapi.Options{
		Pipeline: &pipeline.Pipeline{
			DB:              db,
			Objects:         objects,
			Parser:          clients.NewParser(cfg.ParserBaseURL),
			AI:              clients.NewAI(cfg.AIBaseURL),
			Registry:        registry,
			MinConfidence:   cfg.MinConfidence,
			ClassifyMinConf: cfg.ClassifyMinConf,
		},
		Registry:      registry,
		DB:            db,
		AdminPassword: cfg.AdminPassword,
		HealthTargets: []workflowapi.HealthTarget{ // 服务状态页(管理后台)聚合探测
			{Name: "parser 文档解析", URL: cfg.ParserBaseURL + "/healthz"},
			{Name: "ai 字段提取", URL: cfg.AIBaseURL + "/healthz"},
		},
	})

	return httpx.Serve(addr, handler, "workflow 工作流引擎")
}
