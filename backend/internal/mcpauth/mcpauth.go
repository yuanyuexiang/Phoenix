// Package mcpauth 为 MCP 端点实现 OAuth 2.1 资源服务器(RS)侧组件
// (docs/MCP-OAuth鉴权方案.md):
//
//   - NewVerifier:JWT 验签(OIDC discovery + JWKS,验 iss/exp/aud/签名);
//   - Middleware:按 PHX_OAUTH_MODE 组合 SDK 的 Bearer 校验中间件,
//     optional 档实现「无 token 放行、有 token 强校验」的灰度语义;
//   - MetadataHandler:RFC 9728 受保护资源元数据端点,客户端由此发现授权服务器。
//
// 协议细节(401/WWW-Authenticate、scope 检查、TokenInfo 注入、防会话劫持)
// 全部由官方 go-sdk 的 auth 包与 streamable transport 承担,这里只做装配。
package mcpauth

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/oauthex"
)

// 三档鉴权模式(PHX_OAUTH_MODE)。
const (
	ModeOff      = "off"      // 完全关闭(默认,行为与未接入 OAuth 前一致)
	ModeOptional = "optional" // 无 Authorization 头放行(匿名);有则强校验并记身份 —— 灰度观察用
	ModeRequired = "required" // 强制:所有 /mcp 请求必须携带有效 Bearer token
)

// Config 是 MCP OAuth 鉴权配置,取值来源见 internal/config 的 PHX_OAUTH_*。
type Config struct {
	Mode         string   // off | optional | required
	Issuer       string   // 期望的 iss claim(授权服务器标识)
	DiscoveryURL string   // 实际拉取 OIDC discovery/JWKS 的地址;空 = Issuer。
	Audience     string   // 期望的 aud claim(本资源在 AS 侧的标识)
	Resource     string   // RFC 9728 资源标识,即 MCP 端点对外 URL
	Scopes       []string // 必需 scope,空 = 不检查
}

// Validate 校验配置组合;mode≠off 时缺关键项直接报错(启动 fail-fast)。
func (c Config) Validate() error {
	switch c.Mode {
	case ModeOff, ModeOptional, ModeRequired:
	default:
		return fmt.Errorf("mcpauth: PHX_OAUTH_MODE 取值必须是 off|optional|required,得到 %q", c.Mode)
	}
	if c.Mode == ModeOff {
		return nil
	}
	if c.Issuer == "" {
		return fmt.Errorf("mcpauth: 开启 OAuth(mode=%s)必须设置 PHX_OAUTH_ISSUER", c.Mode)
	}
	if c.Audience == "" {
		return fmt.Errorf("mcpauth: 开启 OAuth(mode=%s)必须设置 PHX_OAUTH_AUDIENCE", c.Mode)
	}
	if _, err := url.Parse(c.Resource); c.Resource == "" || err != nil {
		return fmt.Errorf("mcpauth: PHX_OAUTH_RESOURCE 必须是 MCP 端点的完整 URL,得到 %q", c.Resource)
	}
	return nil
}

// MetadataURL 返回资源元数据端点的完整 URL(RFC 9728 路径插入形态):
// https://host/mcp → https://host/.well-known/oauth-protected-resource/mcp。
// 401 响应的 WWW-Authenticate 头会携带它,客户端由此开始发现流程。
func (c Config) MetadataURL() string {
	u, err := url.Parse(c.Resource)
	if err != nil {
		return ""
	}
	u.Path = "/.well-known/oauth-protected-resource" + u.Path
	u.RawQuery = ""
	return u.String()
}

func (c Config) discoveryURL() string {
	if c.DiscoveryURL != "" {
		return c.DiscoveryURL
	}
	return c.Issuer
}

// NewVerifier 连接授权服务器(拉取 OIDC discovery 元数据与 JWKS)并返回 token 验证器。
// 服务启动时调用一次;AS 未就绪时有限重试(照 store.Open 的 compose 启动约定)。
//
// 返回的 verifier 满足 SDK 约定:
//   - 校验失败返回 auth.ErrInvalidToken 包装错(SDK 据此回 401 而非 500);
//   - 成功必须填 Expiration(SDK 检查过期)与 UserID=sub(SDK 防会话劫持)。
func NewVerifier(ctx context.Context, cfg Config) (auth.TokenVerifier, error) {
	pctx := ctx
	if cfg.discoveryURL() != cfg.Issuer {
		// 容器内经内网地址取元数据,token 的 iss 仍按 Issuer 校验(如 Keycloak 联调场景)
		pctx = oidc.InsecureIssuerURLContext(ctx, cfg.Issuer)
	}
	var provider *oidc.Provider
	var err error
	for attempt := 0; attempt < 15; attempt++ {
		provider, err = oidc.NewProvider(pctx, cfg.discoveryURL())
		if err == nil {
			break
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
	if err != nil {
		return nil, fmt.Errorf("mcpauth: 连接授权服务器失败(discovery=%s): %w", cfg.discoveryURL(), err)
	}

	v := provider.Verifier(&oidc.Config{ClientID: cfg.Audience}) // 验签名/iss/exp,并要求 aud 含 Audience
	return func(ctx context.Context, token string, _ *http.Request) (*auth.TokenInfo, error) {
		tok, err := v.Verify(ctx, token)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", auth.ErrInvalidToken, err)
		}
		var claims struct {
			PreferredUsername string `json:"preferred_username"`
			Name              string `json:"name"`
			Email             string `json:"email"`
			Scope             string `json:"scope"`
		}
		_ = tok.Claims(&claims) // 身份 claims 缺失不致命,Sub 一定有
		return &auth.TokenInfo{
			Scopes:     strings.Fields(claims.Scope),
			Expiration: tok.Expiry,
			UserID:     tok.Subject,
			Extra: map[string]any{
				"username": claims.PreferredUsername,
				"email":    claims.Email,
				"name":     claims.Name,
			},
		}, nil
	}, nil
}

// Middleware 返回按 Mode 组合的鉴权中间件,包裹 /mcp 的 Streamable HTTP handler。
// Mode 为 off 时不应调用本函数(装配处直接跳过)。
func Middleware(cfg Config, verifier auth.TokenVerifier) func(http.Handler) http.Handler {
	require := auth.RequireBearerToken(verifier, &auth.RequireBearerTokenOptions{
		ResourceMetadataURL: cfg.MetadataURL(),
		Scopes:              cfg.Scopes,
	})
	if cfg.Mode != ModeOptional {
		return require
	}
	// optional:无 Authorization 头 → 匿名放行;带了就必须有效(坏 token 不能装匿名混过)。
	// 注意 SDK 会拒绝「带 token 建的会话后续裸奔」(session/UserID 绑定),无需在此处理。
	return func(next http.Handler) http.Handler {
		guarded := require(next)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") == "" {
				next.ServeHTTP(w, r)
				return
			}
			guarded.ServeHTTP(w, r)
		})
	}
}

// MetadataHandler 返回 RFC 9728 受保护资源元数据端点(SDK 实现,自带 CORS)。
func MetadataHandler(cfg Config) http.Handler {
	return auth.ProtectedResourceMetadataHandler(&oauthex.ProtectedResourceMetadata{
		Resource:               cfg.Resource,
		AuthorizationServers:   []string{cfg.Issuer},
		ScopesSupported:        cfg.Scopes,
		BearerMethodsSupported: []string{"header"},
		ResourceName:           "Phoenix 企业智能文档处理平台 MCP 端点",
	})
}
