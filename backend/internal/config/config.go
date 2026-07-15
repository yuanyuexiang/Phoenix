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

	ParserBaseURL   string // 文档解析服务地址
	AIBaseURL       string // AI 字段提取/图片转写服务地址
	WorkflowBaseURL string // 工作流引擎地址(mcp/admin 使用)

	// LLM 为空则使用 Mock 提取器。
	LLMEndpoint string
	LLMAPIKey   string
	LLMModel    string

	// Vision 为空则图片转写未启用(上传图片会在提取阶段明确报错)。
	// 任何 OpenAI 兼容视觉端点均可,基准为阿里 DashScope 兼容模式。
	VisionEndpoint string // 如 https://dashscope.aliyuncs.com/compatible-mode/v1
	VisionAPIKey   string
	VisionModel    string

	MinConfidence   float64 // 字段置信度低于该值转人工审核
	ClassifyMinConf float64 // 自动分类置信度低于该值走开放提取兜底

	// AdminPassword 是管理后台 / workflow API 的访问密钥(请求头 X-Access-Key)。
	// 置空则关闭鉴权(仅建议本机联调);mcp 服务用同一配置调用 workflow。
	AdminPassword string

	// MCP 端点 OAuth 2.1 鉴权(mcp 服务专用,docs/MCP-OAuth鉴权方案.md)。
	// Mode 为 off(默认)时完全不启用,以下其余项不生效。
	OAuthMode         string // off | optional(有 token 记身份、无 token 放行,灰度用)| required
	OAuthIssuer       string // 期望的 iss claim,如 https://kc.example.com/realms/phoenix
	OAuthDiscoveryURL string // 实际拉取 OIDC discovery/JWKS 的地址;空 = Issuer(容器内网地址与 iss 不同时才需设置)
	OAuthAudience     string // 期望的 aud claim(本资源在授权服务器侧的标识)
	OAuthResource     string // RFC 9728 资源标识 = MCP 端点对外 URL
	OAuthScopes       string // 空格分隔的必需 scope,空 = 不检查
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

		ParserBaseURL:   env("PHX_PARSER_URL", "http://localhost:8082"),
		AIBaseURL:       env("PHX_AI_URL", "http://localhost:8083"),
		WorkflowBaseURL: env("PHX_WORKFLOW_URL", "http://localhost:8081"),

		LLMEndpoint: env("PHX_LLM_ENDPOINT", ""),
		LLMAPIKey:   env("PHX_LLM_API_KEY", ""),
		LLMModel:    env("PHX_LLM_MODEL", "deepseek-chat"),

		// 默认 qwen-vl-plus:实测 qwen-vl-ocr 不遵循转写指令(强制表格+代码围栏)
		VisionEndpoint: env("PHX_VISION_ENDPOINT", ""),
		VisionAPIKey:   env("PHX_VISION_API_KEY", ""),
		VisionModel:    env("PHX_VISION_MODEL", "qwen-vl-plus"),

		MinConfidence:   envFloat("PHX_MIN_CONFIDENCE", 0.8),
		ClassifyMinConf: envFloat("PHX_CLASSIFY_MIN_CONF", 0.5),

		AdminPassword: env("PHX_ADMIN_PASSWORD", "phoenix123"), // 默认密码,生产环境务必修改

		OAuthMode:         env("PHX_OAUTH_MODE", "off"),
		OAuthIssuer:       env("PHX_OAUTH_ISSUER", ""),
		OAuthDiscoveryURL: env("PHX_OAUTH_DISCOVERY_URL", ""),
		OAuthAudience:     env("PHX_OAUTH_AUDIENCE", "phoenix-mcp"),
		OAuthResource:     env("PHX_OAUTH_RESOURCE", "http://localhost:8080/mcp"),
		OAuthScopes:       env("PHX_OAUTH_SCOPES", ""),
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
