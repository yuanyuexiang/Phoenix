package restapi

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/yuanyuexiang/phoenix/internal/identity"
	"github.com/yuanyuexiang/phoenix/internal/store"
	"github.com/yuanyuexiang/phoenix/internal/userauth"
)

/* ---------- 登录 / 续期(无需 Bearer) ---------- */

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type refreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	User         struct {
		Username string `json:"username"`
		Name     string `json:"name"`
		Email    string `json:"email"`
	} `json:"user"`
}

// login 用账号口令换一对 token。失败统一回 401,不区分"用户不存在/口令错"以免枚举账号。
func (s *server) login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Username == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "需要 username 与 password")
		return
	}
	u, err := s.opts.DB.GetUserByUsername(r.Context(), req.Username)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "查询账号失败: "+err.Error())
		return
	}
	if u == nil || u.Disabled || !userauth.VerifyPassword(u.PasswordHash, req.Password) {
		writeError(w, http.StatusUnauthorized, "AUTH_FAILED", "用户名或密码错误(或账号已禁用)")
		return
	}
	s.writeTokens(w, u)
	e := store.AuditEntry{Actor: u.Username, ActorSource: "password", Action: "login",
		Detail: map[string]any{"username": u.Username, "email": u.Email}}
	_ = s.opts.DB.InsertAudit(r.Context(), e)
}

// refresh 用 refresh token 换新的一对 token(轮换)。
func (s *server) refresh(w http.ResponseWriter, r *http.Request) {
	var req refreshRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.RefreshToken == "" {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "需要 refresh_token")
		return
	}
	c, err := s.opts.Auth.Verify(req.RefreshToken, userauth.TypRefresh)
	if err != nil {
		s.unauthorized(w, "refresh token 无效: "+err.Error())
		return
	}
	u, err := s.currentUser(r, c)
	if err != nil {
		s.unauthorized(w, err.Error())
		return
	}
	s.writeTokens(w, u)
}

func (s *server) writeTokens(w http.ResponseWriter, u *store.User) {
	access, refresh, expiresIn := s.opts.Auth.Issue(u.Username, u.DisplayName, u.Email, u.TokenVersion)
	resp := tokenResponse{AccessToken: access, RefreshToken: refresh, TokenType: "Bearer", ExpiresIn: expiresIn}
	resp.User.Username, resp.User.Name, resp.User.Email = u.Username, u.DisplayName, u.Email
	writeJSON(w, resp)
}

/* ---------- Bearer 鉴权中间件 ---------- */

// authMiddleware 要求每个请求携带本平台签发的 access token;校验签名/有效期后
// 再对库比对账号状态与 token_version(改密/禁用即时生效),身份存入 ctx。
func (s *server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw := bearerToken(r.Header.Get("Authorization"))
		if raw == "" {
			s.unauthorized(w, "缺少 Bearer token(请先登录)")
			return
		}
		c, err := s.opts.Auth.Verify(raw, userauth.TypAccess)
		if err != nil {
			s.unauthorized(w, "token 校验失败: "+err.Error())
			return
		}
		u, err := s.currentUser(r, c)
		if err != nil {
			s.unauthorized(w, err.Error())
			return
		}
		id := identity.User{Sub: u.Username, Username: u.Username, Email: u.Email, Name: u.DisplayName}
		next.ServeHTTP(w, r.WithContext(identity.WithUser(r.Context(), id)))
	})
}

// currentUser 按 claims 载入账号并做状态/版本核验。
func (s *server) currentUser(r *http.Request, c userauth.Claims) (*store.User, error) {
	u, err := s.opts.DB.GetUserByUsername(r.Context(), c.Sub)
	if err != nil {
		return nil, err
	}
	switch {
	case u == nil:
		return nil, errAuth("账号不存在")
	case u.Disabled:
		return nil, errAuth("账号已禁用")
	case u.TokenVersion != c.Ver:
		return nil, errAuth("凭证已失效(口令已修改),请重新登录")
	}
	return u, nil
}

type errAuth string

func (e errAuth) Error() string { return string(e) }

func bearerToken(h string) string {
	const p = "Bearer "
	if len(h) >= len(p) && strings.EqualFold(h[:len(p)], p) {
		return strings.TrimSpace(h[len(p):])
	}
	return ""
}

func (s *server) unauthorized(w http.ResponseWriter, msg string) {
	w.Header().Set("WWW-Authenticate", `Bearer error="invalid_token"`)
	writeError(w, http.StatusUnauthorized, "AUTH_FAILED", msg)
}
