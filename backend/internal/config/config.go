// Package config 从环境变量加载运行配置(12-factor)。
// 所有变量带 PHX_ 前缀,默认值见下方 Load;
// 容器环境的实际取值见 deploy/docker-compose.yml。
package config

import (
	"os"
	"strconv"
)

type Config struct {
	HTTPAddr    string // MCP/HTTP 服务监听地址
	DoctypesDir string // 单据类型 schema 目录

	DatabaseDSN string

	MinioEndpoint  string
	MinioAccessKey string
	MinioSecretKey string
	MinioBucket    string
	MinioUseSSL    bool

	OCRBaseURL      string // OCR 服务地址
	ParserBaseURL   string // 文档解析服务地址
	AIBaseURL       string // AI 字段提取服务地址
	WorkflowBaseURL string // 工作流引擎地址(mcp/admin 使用)

	// LLM 为空则使用 Mock 提取器。
	LLMEndpoint string
	LLMAPIKey   string
	LLMModel    string

	MinConfidence float64 // 低于该置信度转人工审核

	// AdminPassword 是管理后台 / workflow API 的访问密钥(请求头 X-Access-Key)。
	// 置空则关闭鉴权(仅建议本机联调);mcp 服务用同一配置调用 workflow。
	AdminPassword string
}

func Load() Config {
	return Config{
		HTTPAddr:    env("PHX_HTTP_ADDR", ":8080"),
		DoctypesDir: env("PHX_DOCTYPES_DIR", "configs/doctypes"),

		// 默认值与 deploy/docker-compose.yml 暴露的宿主机端口一致(make infra-up 后 make run 即可用)。
		DatabaseDSN: env("PHX_DB_DSN", "postgres://phoenix:phoenix@localhost:5433/phoenix?sslmode=disable"),

		MinioEndpoint:  env("PHX_MINIO_ENDPOINT", "localhost:9100"),
		MinioAccessKey: env("PHX_MINIO_ACCESS_KEY", "phoenix"),
		MinioSecretKey: env("PHX_MINIO_SECRET_KEY", "phoenix-secret"),
		MinioBucket:    env("PHX_MINIO_BUCKET", "documents"),
		MinioUseSSL:    envBool("PHX_MINIO_USE_SSL", false),

		OCRBaseURL:      env("PHX_OCR_URL", "http://localhost:8001"),
		ParserBaseURL:   env("PHX_PARSER_URL", "http://localhost:8082"),
		AIBaseURL:       env("PHX_AI_URL", "http://localhost:8083"),
		WorkflowBaseURL: env("PHX_WORKFLOW_URL", "http://localhost:8081"),

		LLMEndpoint: env("PHX_LLM_ENDPOINT", ""),
		LLMAPIKey:   env("PHX_LLM_API_KEY", ""),
		LLMModel:    env("PHX_LLM_MODEL", "deepseek-chat"),

		MinConfidence: envFloat("PHX_MIN_CONFIDENCE", 0.8),

		AdminPassword: env("PHX_ADMIN_PASSWORD", "phoenix123"), // 默认密码,生产环境务必修改
	}
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envBool(key string, def bool) bool {
	if v := os.Getenv(key); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return def
}

func envFloat(key string, def float64) float64 {
	if v := os.Getenv(key); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return def
}
