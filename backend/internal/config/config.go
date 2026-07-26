// Package config 从环境变量加载运行配置(12-factor)。
// 所有变量带 PHX_ 前缀,默认值见下方 Load;
// 容器环境的实际取值见 deploy/docker-compose.yml。
package config

import (
	"os"
	"strconv"
)

type Config struct {
	DoctypesDir string // 单据类型 schema 目录
	OntologyDir string // 本体定义目录(为空/不存在 = 本体层未启用)

	DatabaseDSN string

	MinioEndpoint  string
	MinioAccessKey string
	MinioSecretKey string
	MinioBucket    string
	MinioUseSSL    bool

	MinConfidence float64 // 字段置信度低于该值转人工审核(仅当客户端回传了置信度)

	// RAG 知识库 embedding(检索索引用途)。Endpoint 为空则知识库关闭:
	// save 不入向量,ask_document 返回"未启用"。任何 OpenAI 兼容 embeddings 端点均可。
	// EmbedDim 须与 store 迁移里的 vector(N) 一致(当前 1024);换维度需新迁移。
	EmbedEndpoint string // 如 https://dashscope.aliyuncs.com/compatible-mode/v1
	EmbedAPIKey   string
	EmbedModel    string
	EmbedDim      int

	// AdminPassword 是管理后台 / workflow API 的访问密钥(请求头 X-Access-Key)。
	// 置空则关闭鉴权(仅建议本机联调)。
	AdminPassword string

	// 员工级公网 REST 面(/pub/v1)的自研账号鉴权(phoenix-doc-assistant 专家用,
	// 每员工账号 + 平台签发 JWT,见 internal/userauth)。与既有 X-Access-Key(前端用)互不干扰。
	// 为空 → /pub/v1 不挂载(默认关闭);生产必须设为强随机长串,泄露/更换会使全部登录失效。
	AuthSecret string // JWT 签名密钥(PHX_AUTH_SECRET)
}

func Load() Config {
	return Config{
		DoctypesDir: env("PHX_DOCTYPES_DIR", "configs/doctypes"),
		OntologyDir: env("PHX_ONTOLOGY_DIR", "configs/ontology"),

		// 默认值与 deploy/docker-compose.yml 暴露的宿主机端口一致(make infra-up 后 make run 即可用)。
		DatabaseDSN: env("PHX_DB_DSN", "postgres://phoenix:phoenix@localhost:5433/phoenix?sslmode=disable"),

		MinioEndpoint:  env("PHX_MINIO_ENDPOINT", "localhost:9100"),
		MinioAccessKey: env("PHX_MINIO_ACCESS_KEY", "phoenix"),
		MinioSecretKey: env("PHX_MINIO_SECRET_KEY", "phoenix-secret"),
		MinioBucket:    env("PHX_MINIO_BUCKET", "documents"),
		MinioUseSSL:    envBool("PHX_MINIO_USE_SSL", false),

		MinConfidence: envFloat("PHX_MIN_CONFIDENCE", 0.8),

		EmbedEndpoint: env("PHX_EMBED_ENDPOINT", ""),
		EmbedAPIKey:   env("PHX_EMBED_API_KEY", ""),
		EmbedModel:    env("PHX_EMBED_MODEL", "text-embedding-v3"),
		EmbedDim:      envInt("PHX_EMBED_DIM", 1024),

		AdminPassword: env("PHX_ADMIN_PASSWORD", "phoenix123"), // 默认密码,生产环境务必修改

		AuthSecret: env("PHX_AUTH_SECRET", ""),
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

func envInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}
