// Package identity 定义经 OAuth 鉴权得到的操作人身份:
// /pub/v1(internal/restapi)校验 Bearer token 后把 claims 解成 User 存入 ctx,
// handler 取出后落库(documents.uploaded_by / reviewed_by、audit_log),
// 使每个操作可追溯到具体员工。
package identity

import "context"

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

// WithUser 把身份存入 ctx(鉴权中间件校验 token 后调用)。
func WithUser(ctx context.Context, u User) context.Context {
	return context.WithValue(ctx, ctxKey{}, u)
}

// FromContext 取出 ctx 中的身份。
func FromContext(ctx context.Context) (User, bool) {
	u, ok := ctx.Value(ctxKey{}).(User)
	return u, ok && !u.IsZero()
}
