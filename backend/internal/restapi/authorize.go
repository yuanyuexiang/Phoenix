package restapi

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"html/template"
	"net/http"
	"net/url"
	"time"

	"github.com/yuanyuexiang/phoenix/internal/store"
	"github.com/yuanyuexiang/phoenix/internal/userauth"
)

// 浏览器登录流(复刻 OAuth 授权码 + PKCE 的交互体验,但登录页由本平台自己出,
// 无任何第三方身份组件):
//
//	客户端(auth.py)起本机回调服务 → 弹浏览器打开 GET /pub/v1/auth/authorize
//	→ 员工在平台登录页输入账号口令(POST /pub/v1/auth/authorize)
//	→ 验证通过签发一次性授权码,302 回 http://127.0.0.1:{port}/callback?code=...&state=...
//	→ 客户端 POST /pub/v1/auth/token(code + code_verifier)换 access/refresh。
//
// 安全要点:redirect_uri 只允许本机回环(防止授权码被外部站点截走);
// code 为短时效(2 分钟)签名 JWT,绑定 PKCE challenge,且服务端记录一次性使用。

/* ---------- GET /pub/v1/auth/authorize:登录页 ---------- */

type authorizeParams struct {
	RedirectURI   string
	State         string
	CodeChallenge string
	Error         string
}

func (s *server) authorizePage(w http.ResponseWriter, r *http.Request) {
	p := authorizeParams{
		RedirectURI:   r.URL.Query().Get("redirect_uri"),
		State:         r.URL.Query().Get("state"),
		CodeChallenge: r.URL.Query().Get("code_challenge"),
	}
	if err := validateAuthorizeParams(p, r.URL.Query().Get("code_challenge_method")); err != "" {
		http.Error(w, err, http.StatusBadRequest)
		return
	}
	renderLogin(w, p)
}

/* ---------- POST /pub/v1/auth/authorize:表单提交 ---------- */

func (s *server) authorizeSubmit(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "表单解析失败", http.StatusBadRequest)
		return
	}
	p := authorizeParams{
		RedirectURI:   r.PostFormValue("redirect_uri"),
		State:         r.PostFormValue("state"),
		CodeChallenge: r.PostFormValue("code_challenge"),
	}
	if err := validateAuthorizeParams(p, "S256"); err != "" {
		http.Error(w, err, http.StatusBadRequest)
		return
	}

	username, password := r.PostFormValue("username"), r.PostFormValue("password")
	u, err := s.opts.DB.GetUserByUsername(r.Context(), username)
	if err != nil {
		p.Error = "查询账号失败,请稍后重试"
		renderLogin(w, p)
		return
	}
	if u == nil || u.Disabled || !userauth.VerifyPassword(u.PasswordHash, password) {
		p.Error = "用户名或密码错误(或账号已禁用)"
		renderLogin(w, p)
		return
	}

	code := s.opts.Auth.IssueCode(u.Username, u.TokenVersion, p.CodeChallenge)
	e := store.AuditEntry{Actor: u.Username, ActorSource: "password", Action: "login",
		Detail: map[string]any{"username": u.Username, "flow": "browser"}}
	_ = s.opts.DB.InsertAudit(r.Context(), e)

	q := url.Values{"code": {code}, "state": {p.State}}
	http.Redirect(w, r, p.RedirectURI+"?"+q.Encode(), http.StatusFound)
}

/* ---------- POST /pub/v1/auth/token:授权码换 token ---------- */

func (s *server) token(w http.ResponseWriter, r *http.Request) {
	var req struct {
		GrantType    string `json:"grant_type"`
		Code         string `json:"code"`
		CodeVerifier string `json:"code_verifier"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.GrantType != "authorization_code" {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", `需要 {"grant_type":"authorization_code","code","code_verifier"}`)
		return
	}
	c, err := s.opts.Auth.Verify(req.Code, userauth.TypCode)
	if err != nil {
		s.unauthorized(w, "授权码无效: "+err.Error())
		return
	}
	if s256(req.CodeVerifier) != c.Cch {
		s.unauthorized(w, "PKCE 校验失败")
		return
	}
	if !s.markCodeUsed(req.Code, c.Exp) {
		s.unauthorized(w, "授权码已使用")
		return
	}
	u, err := s.currentUser(r, c)
	if err != nil {
		s.unauthorized(w, err.Error())
		return
	}
	s.writeTokens(w, u)
}

/* ---------- 辅助 ---------- */

// validateAuthorizeParams 校验授权参数;返回空串表示通过。
func validateAuthorizeParams(p authorizeParams, method string) string {
	if !isLoopback(p.RedirectURI) {
		return "redirect_uri 必须是本机回环地址(http://127.0.0.1:port/... 或 http://localhost:port/...)"
	}
	if p.CodeChallenge == "" || (method != "" && method != "S256") {
		return "需要 code_challenge(method=S256)"
	}
	return ""
}

// isLoopback 只放行本机回环回调,防止授权码被重定向到外部站点。
func isLoopback(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "http" {
		return false
	}
	h := u.Hostname()
	return h == "127.0.0.1" || h == "localhost" || h == "::1"
}

func s256(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// markCodeUsed 登记授权码使用;重复使用返回 false(一次性)。顺带清理过期记录。
func (s *server) markCodeUsed(code string, exp int64) bool {
	s.codeMu.Lock()
	defer s.codeMu.Unlock()
	now := time.Now().Unix()
	for k, e := range s.usedCodes {
		if e < now {
			delete(s.usedCodes, k)
		}
	}
	if _, dup := s.usedCodes[code]; dup {
		return false
	}
	s.usedCodes[code] = exp
	return true
}

/* ---------- 登录页(与管理后台同一套企业蓝视觉语言) ---------- */

func renderLogin(w http.ResponseWriter, p authorizeParams) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = loginTpl.Execute(w, p)
}

var loginTpl = template.Must(template.New("login").Parse(`<!doctype html>
<html lang="zh-CN"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Phoenix · 员工登录</title>
<style>
  :root{--bg:#F6F8FB;--card:#FFF;--line:#D9E1EC;--ink:#2B3A52;--dim:#8B98AC;--acc:#2563EB;--red:#DC2626;}
  @media (prefers-color-scheme:dark){:root{--bg:#0E141F;--card:#151C29;--line:#2C3850;--ink:#CBD6E5;--dim:#6D7C94;--acc:#5B8DEF;--red:#F06A6A;}}
  *{box-sizing:border-box;margin:0}
  body{min-height:100vh;display:grid;place-items:center;background:var(--bg);
       font-family:-apple-system,"PingFang SC","Microsoft YaHei",system-ui,sans-serif;color:var(--ink)}
  .card{width:min(360px,92vw);background:var(--card);border:1px solid var(--line);border-radius:14px;padding:34px 30px}
  .mark{position:relative;width:46px;height:46px;margin:0 auto 14px;display:grid;place-items:center;color:var(--acc);font-size:20px;font-weight:600}
  .mark::before{content:"";position:absolute;inset:0;border:1px solid var(--acc);opacity:.5;border-radius:50%}
  .mark::after{content:"";position:absolute;inset:5px;border:1px solid var(--acc);opacity:.25;border-radius:50%}
  h1{font-size:17px;text-align:center;margin-bottom:4px}
  .sub{font-size:12px;color:var(--dim);text-align:center;margin-bottom:22px}
  label{display:block;font-size:12px;color:var(--dim);margin:12px 0 5px}
  input[type=text],input[type=password]{width:100%;padding:9px 12px;font-size:14px;color:var(--ink);
    background:transparent;border:1px solid var(--line);border-radius:8px;outline:none}
  input:focus{border-color:var(--acc)}
  button{width:100%;margin-top:20px;padding:10px;font-size:14px;border:0;border-radius:8px;
    background:var(--acc);color:#fff;cursor:pointer}
  .err{margin-top:14px;font-size:12.5px;color:var(--red);text-align:center}
  .foot{margin-top:18px;font-size:11px;color:var(--dim);text-align:center}
</style></head><body>
<form class="card" method="post" action="">
  <div class="mark">凤</div>
  <h1>Phoenix 员工登录</h1>
  <div class="sub">登录成功后请返回 WorkBuddy 继续</div>
  <label>用户名</label>
  <input type="text" name="username" autocomplete="username" autofocus required>
  <label>口令</label>
  <input type="password" name="password" autocomplete="current-password" required>
  <input type="hidden" name="redirect_uri" value="{{.RedirectURI}}">
  <input type="hidden" name="state" value="{{.State}}">
  <input type="hidden" name="code_challenge" value="{{.CodeChallenge}}">
  <button type="submit">登 录</button>
  {{if .Error}}<div class="err">{{.Error}}</div>{{end}}
  <div class="foot">账号由管理员在 Phoenix 管理后台创建 · 忘记口令请联系管理员重置</div>
</form>
</body></html>`))
