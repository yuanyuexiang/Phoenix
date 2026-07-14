// Package identity 定义经 OAuth 鉴权得到的操作人身份,以及它在服务间的传递方式:
// mcp 服务从 access token 取出身份 → 存入 ctx → 出站请求头(X-Phx-User-*)→
// workflow 服务解出后落库(documents.uploaded_by / reviewed_by、audit_log)。
//
// 身份头只是传输载体,不是鉴权凭证:workflow 仅在 X-Access-Key 校验通过后才信任它
// (见 workflowapi.operatorOf)。
package identity

import (
	"context"
	"net/http"
	"net/url"
)

// User 是从 access token claims 提取的操作人身份。
type User struct {
	Sub      string // token 的 sub claim,IdP 内唯一
	Username string // preferred_username
	Email    string
	Name     string // 姓名(display name)
}

// Display 返回落库/展示用的口径:username → email → sub。
// 落库口径(工号/邮箱/sub)客户未拍板(方案 §8 Q4),先用最可读的;
// audit_log.detail 存全量 claims,后续可回溯换口径。
func (u User) Display() string {
	switch {
	case u.Username != "":
		return u.Username
	case u.Email != "":
		return u.Email
	default:
		return u.Sub
	}
}

// IsZero 报告是否为空身份。
func (u User) IsZero() bool { return u == User{} }

type ctxKey struct{}

// WithUser 把身份存入 ctx(mcp 工具 handler 使用,随出站请求自动透传)。
func WithUser(ctx context.Context, u User) context.Context {
	return context.WithValue(ctx, ctxKey{}, u)
}

// FromContext 取出 ctx 中的身份。
func FromContext(ctx context.Context) (User, bool) {
	u, ok := ctx.Value(ctxKey{}).(User)
	return u, ok && !u.IsZero()
}

// 服务间身份透传的请求头契约。值经 url.QueryEscape 编码,
// 防止中文姓名等非 ASCII 字符破坏 HTTP 头。
const (
	HeaderSub    = "X-Phx-User-Sub"
	HeaderName   = "X-Phx-User-Name"
	HeaderEmail  = "X-Phx-User-Email"
	HeaderSource = "X-Phx-Source" // "mcp" = 来自连接器;缺失 = 管理后台/脚本直连
)

// SetHeaders 把身份写入出站请求头(空字段不写)。
func SetHeaders(h http.Header, u User) {
	set := func(key, val string) {
		if val != "" {
			h.Set(key, url.QueryEscape(val))
		}
	}
	set(HeaderSub, u.Sub)
	// Name 头承载展示名:优先 username(与 Display 口径一致),其次姓名
	set(HeaderName, u.Username)
	if u.Username == "" {
		set(HeaderName, u.Name)
	}
	set(HeaderEmail, u.Email)
}

// FromHeaders 从入站请求头解出身份;没有任何身份头时 ok 为 false。
func FromHeaders(h http.Header) (User, bool) {
	get := func(key string) string {
		v, err := url.QueryUnescape(h.Get(key))
		if err != nil {
			return h.Get(key) // 解码失败按原值处理,不丢身份
		}
		return v
	}
	u := User{
		Sub:      get(HeaderSub),
		Username: get(HeaderName),
		Email:    get(HeaderEmail),
	}
	return u, !u.IsZero()
}
