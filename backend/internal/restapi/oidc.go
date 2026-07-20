package restapi

import (
	"context"
	"fmt"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"

	"github.com/yuanyuexiang/phoenix/internal/identity"
)

// Verifier 校验 /pub/v1 请求里的 Keycloak Bearer token(JWT):验签名 + iss + exp + aud,
// 通过后把身份 claims 解成 identity.User。与 internal/mcpauth 各自独立(不引 MCP SDK),
// 但底层同样走 go-oidc 的 OIDC discovery + JWKS,行为一致。
type Verifier struct {
	inner *oidc.IDTokenVerifier
}

// NewVerifier 连接授权服务器(拉 OIDC discovery 与 JWKS)并返回 token 验证器。
// 服务启动时调用一次;AS 未就绪时有限重试(对齐 mcpauth.NewVerifier 的 compose 启动约定)。
// discoveryURL 为空则用 issuer;两者不同(容器内网地址取元数据、token 的 iss 仍按 issuer 校验)
// 时用 InsecureIssuerURLContext 放开 iss 与 discovery host 不一致的限制。
func NewVerifier(ctx context.Context, issuer, discoveryURL, audience string) (*Verifier, error) {
	disc := discoveryURL
	if disc == "" {
		disc = issuer
	}
	pctx := ctx
	if disc != issuer {
		pctx = oidc.InsecureIssuerURLContext(ctx, issuer)
	}

	var provider *oidc.Provider
	var err error
	for attempt := 0; attempt < 15; attempt++ {
		provider, err = oidc.NewProvider(pctx, disc)
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
		return nil, fmt.Errorf("restapi: 连接授权服务器失败(discovery=%s): %w", disc, err)
	}

	// ClientID=audience → 要求 token 的 aud 含该值;同时验签名/iss/exp。
	return &Verifier{inner: provider.Verifier(&oidc.Config{ClientID: audience})}, nil
}

// Verify 校验原始 token 字符串,成功返回操作人身份(至少含 Sub)。
func (vf *Verifier) Verify(ctx context.Context, raw string) (identity.User, error) {
	tok, err := vf.inner.Verify(ctx, raw)
	if err != nil {
		return identity.User{}, err
	}
	var claims struct {
		PreferredUsername string `json:"preferred_username"`
		Name              string `json:"name"`
		Email             string `json:"email"`
	}
	_ = tok.Claims(&claims) // 身份 claims 缺失不致命,Sub 一定有
	return identity.User{
		Sub:      tok.Subject,
		Username: claims.PreferredUsername,
		Email:    claims.Email,
		Name:     claims.Name,
	}, nil
}
