// Package userauth 实现自研员工账号体系的凭证层(V1.3 取代 Keycloak):
// PBKDF2 口令哈希 + HS256 JWT(短期 access + 长期 refresh)。
// 只依赖标准库,不引入任何 OAuth/JWT 第三方组件。
//
// token 撤销依赖 users.token_version:签发时把当前版本写进 claims(ver),
// 校验时与库里比对 —— 改密/禁用账号会 +1,旧 token 随即失效。
package userauth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

/* ---------- 口令哈希(PBKDF2-SHA256) ---------- */

const (
	pbkdf2Iters   = 120_000
	pbkdf2SaltLen = 16
	pbkdf2KeyLen  = 32
)

// HashPassword 生成 "pbkdf2$sha256$iter$salt$dk" 格式的口令哈希。
func HashPassword(password string) string {
	salt := make([]byte, pbkdf2SaltLen)
	if _, err := rand.Read(salt); err != nil {
		panic(fmt.Sprintf("userauth: 随机盐生成失败: %v", err)) // crypto/rand 失败属系统级故障
	}
	dk := pbkdf2SHA256([]byte(password), salt, pbkdf2Iters, pbkdf2KeyLen)
	b64 := base64.RawStdEncoding.EncodeToString
	return fmt.Sprintf("pbkdf2$sha256$%d$%s$%s", pbkdf2Iters, b64(salt), b64(dk))
}

// VerifyPassword 恒定时间比较口令与哈希。
func VerifyPassword(hash, password string) bool {
	parts := strings.Split(hash, "$")
	if len(parts) != 5 || parts[0] != "pbkdf2" || parts[1] != "sha256" {
		return false
	}
	iters, err := strconv.Atoi(parts[2])
	if err != nil || iters < 1 {
		return false
	}
	salt, err1 := base64.RawStdEncoding.DecodeString(parts[3])
	want, err2 := base64.RawStdEncoding.DecodeString(parts[4])
	if err1 != nil || err2 != nil || len(want) == 0 {
		return false
	}
	got := pbkdf2SHA256([]byte(password), salt, iters, len(want))
	return subtle.ConstantTimeCompare(got, want) == 1
}

// pbkdf2SHA256 是 RFC 2898 的最小实现(标准库无 bcrypt/argon2,自带实现避免新依赖)。
func pbkdf2SHA256(password, salt []byte, iters, keyLen int) []byte {
	prf := func(data []byte) []byte {
		m := hmac.New(sha256.New, password)
		m.Write(data)
		return m.Sum(nil)
	}
	var out []byte
	block := make([]byte, len(salt)+4)
	copy(block, salt)
	for i := 1; len(out) < keyLen; i++ {
		binary.BigEndian.PutUint32(block[len(salt):], uint32(i))
		u := prf(block)
		t := make([]byte, len(u))
		copy(t, u)
		for j := 1; j < iters; j++ {
			u = prf(u)
			for k := range t {
				t[k] ^= u[k]
			}
		}
		out = append(out, t...)
	}
	return out[:keyLen]
}

/* ---------- JWT(HS256) ---------- */

const (
	TypAccess  = "access"
	TypRefresh = "refresh"
	TypCode    = "code" // 浏览器登录流的一次性授权码(PKCE 绑定,见 restapi authorize/token)

	DefaultAccessTTL  = time.Hour
	DefaultRefreshTTL = 30 * 24 * time.Hour
	CodeTTL           = 2 * time.Minute
)

// Claims 是本平台签发 token 的载荷。
type Claims struct {
	Sub   string `json:"sub"` // username
	Name  string `json:"name,omitempty"`
	Email string `json:"email,omitempty"`
	Typ   string `json:"typ"`           // access | refresh | code
	Ver   int    `json:"ver"`           // users.token_version,改密/禁用后旧 token 失效
	Cch   string `json:"cch,omitempty"` // code 专用:PKCE code_challenge(S256)
	Iat   int64  `json:"iat"`
	Exp   int64  `json:"exp"`
}

// Service 负责 token 的签发与校验。
type Service struct {
	secret     []byte
	AccessTTL  time.Duration
	RefreshTTL time.Duration
	now        func() time.Time // 测试可替换
}

// New 创建凭证服务;secret 为 PHX_AUTH_SECRET(生产必须为强随机长串)。
func New(secret []byte) *Service {
	return &Service{secret: secret, AccessTTL: DefaultAccessTTL, RefreshTTL: DefaultRefreshTTL, now: time.Now}
}

// Issue 为员工签发一对 token(access + refresh)。
func (s *Service) Issue(username, name, email string, tokenVersion int) (access, refresh string, expiresIn int) {
	now := s.now()
	base := Claims{Sub: username, Name: name, Email: email, Ver: tokenVersion, Iat: now.Unix()}

	a := base
	a.Typ = TypAccess
	a.Exp = now.Add(s.AccessTTL).Unix()
	r := base
	r.Typ = TypRefresh
	r.Exp = now.Add(s.RefreshTTL).Unix()

	return s.sign(a), s.sign(r), int(s.AccessTTL.Seconds())
}

// IssueCode 为浏览器登录流签发一次性授权码:短时效,绑定 PKCE code_challenge,
// 兑换 token 时校验 S256(code_verifier) == Cch(见 restapi 的 /auth/token)。
func (s *Service) IssueCode(username string, tokenVersion int, codeChallenge string) string {
	now := s.now()
	return s.sign(Claims{
		Sub: username, Typ: TypCode, Ver: tokenVersion, Cch: codeChallenge,
		Iat: now.Unix(), Exp: now.Add(CodeTTL).Unix(),
	})
}

// Verify 校验签名与有效期,并要求 typ 匹配(access token 不能当 refresh 用,反之亦然)。
func (s *Service) Verify(token, wantTyp string) (Claims, error) {
	var zero Claims
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return zero, fmt.Errorf("token 格式错误")
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return zero, fmt.Errorf("token 签名解码失败")
	}
	if !hmac.Equal(sig, s.hmac(parts[0]+"."+parts[1])) {
		return zero, fmt.Errorf("token 签名不匹配")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return zero, fmt.Errorf("token 载荷解码失败")
	}
	var c Claims
	if err := json.Unmarshal(payload, &c); err != nil {
		return zero, fmt.Errorf("token 载荷解析失败")
	}
	if c.Typ != wantTyp {
		return zero, fmt.Errorf("token 类型错误(需 %s)", wantTyp)
	}
	if s.now().Unix() >= c.Exp {
		return zero, fmt.Errorf("token 已过期")
	}
	if c.Sub == "" {
		return zero, fmt.Errorf("token 缺少 sub")
	}
	return c, nil
}

func (s *Service) sign(c Claims) string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	payload, _ := json.Marshal(c)
	body := header + "." + base64.RawURLEncoding.EncodeToString(payload)
	return body + "." + base64.RawURLEncoding.EncodeToString(s.hmac(body))
}

func (s *Service) hmac(data string) []byte {
	m := hmac.New(sha256.New, s.secret)
	m.Write([]byte(data))
	return m.Sum(nil)
}
