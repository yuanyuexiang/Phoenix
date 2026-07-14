package mcpauth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	jose "github.com/go-jose/go-jose/v4"
	"github.com/modelcontextprotocol/go-sdk/auth"
)

// fakeAS 是测试用授权服务器:提供 OIDC discovery 与 JWKS,并能签发任意 claims 的 JWT。
type fakeAS struct {
	srv    *httptest.Server
	signer jose.Signer
}

func newFakeAS(t *testing.T) *fakeAS {
	t.Helper()
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := jose.NewSigner(
		jose.SigningKey{Algorithm: jose.RS256, Key: priv},
		(&jose.SignerOptions{}).WithHeader("kid", "test-kid"),
	)
	if err != nil {
		t.Fatal(err)
	}

	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer":                                srv.URL,
			"jwks_uri":                              srv.URL + "/jwks",
			"authorization_endpoint":                srv.URL + "/authorize",
			"token_endpoint":                        srv.URL + "/token",
			"id_token_signing_alg_values_supported": []string{"RS256"},
		})
	})
	mux.HandleFunc("/jwks", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{
			{Key: &priv.PublicKey, KeyID: "test-kid", Algorithm: "RS256", Use: "sig"},
		}})
	})

	return &fakeAS{srv: srv, signer: signer}
}

// token 签发一个默认合法的 access token;mutate 可改写任意 claim。
func (a *fakeAS) token(t *testing.T, mutate func(claims map[string]any)) string {
	t.Helper()
	claims := map[string]any{
		"iss":                a.srv.URL,
		"sub":                "sub-alice",
		"aud":                "phoenix-mcp",
		"exp":                time.Now().Add(time.Hour).Unix(),
		"iat":                time.Now().Unix(),
		"preferred_username": "alice",
		"email":              "alice@example.com",
		"name":               "Alice Zhang",
		"scope":              "openid profile",
	}
	if mutate != nil {
		mutate(claims)
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		t.Fatal(err)
	}
	sig, err := a.signer.Sign(payload)
	if err != nil {
		t.Fatal(err)
	}
	s, err := sig.CompactSerialize()
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func (a *fakeAS) config(mode string) Config {
	return Config{
		Mode:     mode,
		Issuer:   a.srv.URL,
		Audience: "phoenix-mcp",
		Resource: "http://localhost:8080/mcp",
	}
}

func (a *fakeAS) verifier(t *testing.T, cfg Config) auth.TokenVerifier {
	t.Helper()
	v, err := NewVerifier(context.Background(), cfg)
	if err != nil {
		t.Fatalf("NewVerifier: %v", err)
	}
	return v
}

func TestVerifier(t *testing.T) {
	as := newFakeAS(t)
	verify := as.verifier(t, as.config(ModeRequired))

	t.Run("合法 token", func(t *testing.T) {
		info, err := verify(context.Background(), as.token(t, nil), nil)
		if err != nil {
			t.Fatalf("应通过校验: %v", err)
		}
		if info.UserID != "sub-alice" {
			t.Errorf("UserID = %q, want sub-alice", info.UserID)
		}
		if info.Expiration.IsZero() || info.Expiration.Before(time.Now()) {
			t.Errorf("Expiration 必须为未来时间(SDK 强制检查),得到 %v", info.Expiration)
		}
		if got := info.Extra["username"]; got != "alice" {
			t.Errorf("Extra[username] = %v, want alice", got)
		}
		if got := info.Extra["email"]; got != "alice@example.com" {
			t.Errorf("Extra[email] = %v", got)
		}
		if len(info.Scopes) != 2 || info.Scopes[0] != "openid" {
			t.Errorf("Scopes = %v, want [openid profile]", info.Scopes)
		}
	})

	bad := map[string]func(map[string]any){
		"过期":     func(c map[string]any) { c["exp"] = time.Now().Add(-time.Hour).Unix() },
		"aud 不符": func(c map[string]any) { c["aud"] = "other-service" },
		"iss 不符": func(c map[string]any) { c["iss"] = "https://evil.example.com" },
	}
	for name, mutate := range bad {
		t.Run(name, func(t *testing.T) {
			_, err := verify(context.Background(), as.token(t, mutate), nil)
			if err == nil {
				t.Fatal("应拒绝")
			}
			if !errors.Is(err, auth.ErrInvalidToken) {
				t.Errorf("错误必须包装 auth.ErrInvalidToken(否则 SDK 回 500 而非 401): %v", err)
			}
		})
	}

	t.Run("他人私钥签名", func(t *testing.T) {
		other := newFakeAS(t) // 不同私钥
		forged := other.token(t, func(c map[string]any) { c["iss"] = as.srv.URL })
		if _, err := verify(context.Background(), forged, nil); err == nil {
			t.Fatal("JWKS 之外的密钥签名应被拒绝")
		}
	})
}

// echoTokenInfo 返回下游能否从 ctx 拿到 TokenInfo。
func echoTokenInfo() (http.Handler, *[]string) {
	var seen []string
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if info := auth.TokenInfoFromContext(r.Context()); info != nil {
			seen = append(seen, info.UserID)
		} else {
			seen = append(seen, "")
		}
		w.WriteHeader(http.StatusOK)
	})
	return h, &seen
}

func do(t *testing.T, h http.Handler, token string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest("POST", "/mcp", strings.NewReader("{}"))
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestMiddlewareRequired(t *testing.T) {
	as := newFakeAS(t)
	cfg := as.config(ModeRequired)
	next, seen := echoTokenInfo()
	h := Middleware(cfg, as.verifier(t, cfg))(next)

	if w := do(t, h, ""); w.Code != http.StatusUnauthorized {
		t.Fatalf("无 token 应 401,得到 %d", w.Code)
	} else if hdr := w.Header().Get("WWW-Authenticate"); !strings.Contains(hdr, "resource_metadata") {
		t.Errorf("WWW-Authenticate 应携带 resource_metadata 供客户端发现: %q", hdr)
	}
	if w := do(t, h, "not-a-jwt"); w.Code != http.StatusUnauthorized {
		t.Fatalf("坏 token 应 401,得到 %d", w.Code)
	}
	if w := do(t, h, as.token(t, nil)); w.Code != http.StatusOK {
		t.Fatalf("合法 token 应 200,得到 %d: %s", w.Code, w.Body)
	}
	if len(*seen) != 1 || (*seen)[0] != "sub-alice" {
		t.Fatalf("下游应拿到 TokenInfo(UserID=sub-alice),得到 %v", *seen)
	}
}

func TestMiddlewareOptional(t *testing.T) {
	as := newFakeAS(t)
	cfg := as.config(ModeOptional)
	next, seen := echoTokenInfo()
	h := Middleware(cfg, as.verifier(t, cfg))(next)

	if w := do(t, h, ""); w.Code != http.StatusOK {
		t.Fatalf("optional 下无 token 应放行,得到 %d", w.Code)
	}
	if w := do(t, h, "bad-token"); w.Code != http.StatusUnauthorized {
		t.Fatalf("optional 下坏 token 不能装匿名混过,应 401,得到 %d", w.Code)
	}
	if w := do(t, h, as.token(t, nil)); w.Code != http.StatusOK {
		t.Fatalf("optional 下合法 token 应 200,得到 %d", w.Code)
	}
	if want := []string{"", "sub-alice"}; fmt.Sprint(*seen) != fmt.Sprint(want) {
		t.Fatalf("下游看到的身份序列 = %v, want %v", *seen, want)
	}
}

func TestMetadataHandler(t *testing.T) {
	as := newFakeAS(t)
	cfg := as.config(ModeRequired)
	r := httptest.NewRequest("GET", "/.well-known/oauth-protected-resource/mcp", nil)
	w := httptest.NewRecorder()
	MetadataHandler(cfg).ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("HTTP %d", w.Code)
	}
	var meta struct {
		Resource             string   `json:"resource"`
		AuthorizationServers []string `json:"authorization_servers"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &meta); err != nil {
		t.Fatal(err)
	}
	if meta.Resource != cfg.Resource {
		t.Errorf("resource = %q, want %q", meta.Resource, cfg.Resource)
	}
	if len(meta.AuthorizationServers) != 1 || meta.AuthorizationServers[0] != cfg.Issuer {
		t.Errorf("authorization_servers = %v, want [%s]", meta.AuthorizationServers, cfg.Issuer)
	}
}

func TestConfigMetadataURL(t *testing.T) {
	cases := []struct{ resource, want string }{
		{"http://localhost:8080/mcp", "http://localhost:8080/.well-known/oauth-protected-resource/mcp"},
		{"https://phoenix.matrix-net.tech/mcp", "https://phoenix.matrix-net.tech/.well-known/oauth-protected-resource/mcp"},
	}
	for _, tc := range cases {
		if got := (Config{Resource: tc.resource}).MetadataURL(); got != tc.want {
			t.Errorf("MetadataURL(%s) = %q, want %q", tc.resource, got, tc.want)
		}
	}
}

func TestConfigValidate(t *testing.T) {
	if err := (Config{Mode: ModeOff}).Validate(); err != nil {
		t.Errorf("off 不要求其他配置: %v", err)
	}
	if err := (Config{Mode: "on"}).Validate(); err == nil {
		t.Error("非法 mode 应报错")
	}
	if err := (Config{Mode: ModeRequired}).Validate(); err == nil {
		t.Error("required 缺 issuer 应报错")
	}
	ok := Config{Mode: ModeRequired, Issuer: "http://as", Audience: "aud", Resource: "http://x/mcp"}
	if err := ok.Validate(); err != nil {
		t.Errorf("完整配置应通过: %v", err)
	}
}
