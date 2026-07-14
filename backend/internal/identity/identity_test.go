package identity

import (
	"context"
	"net/http"
	"testing"
)

func TestHeadersRoundTrip(t *testing.T) {
	cases := []struct {
		name string
		in   User
		want User // 经头传输后期望解出的身份(Name 不上头,Username 承载展示名)
		ok   bool
	}{
		{
			name: "英文用户名",
			in:   User{Sub: "sub-1", Username: "alice", Email: "alice@example.com"},
			want: User{Sub: "sub-1", Username: "alice", Email: "alice@example.com"},
			ok:   true,
		},
		{
			name: "中文姓名(无 username 时降级为展示名)",
			in:   User{Sub: "sub-2", Name: "张三", Email: "zhang@example.com"},
			want: User{Sub: "sub-2", Username: "张三", Email: "zhang@example.com"},
			ok:   true,
		},
		{
			name: "仅 sub",
			in:   User{Sub: "sub-3"},
			want: User{Sub: "sub-3"},
			ok:   true,
		},
		{
			name: "空身份不写头",
			in:   User{},
			want: User{},
			ok:   false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := http.Header{}
			SetHeaders(h, tc.in)
			got, ok := FromHeaders(h)
			if ok != tc.ok {
				t.Fatalf("ok = %v, want %v", ok, tc.ok)
			}
			if got != tc.want {
				t.Fatalf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestHeaderValuesAreASCII(t *testing.T) {
	h := http.Header{}
	SetHeaders(h, User{Sub: "s", Name: "李四"})
	for _, key := range []string{HeaderSub, HeaderName, HeaderEmail} {
		for _, c := range h.Get(key) {
			if c > 127 {
				t.Fatalf("%s 头包含非 ASCII 字符: %q", key, h.Get(key))
			}
		}
	}
}

func TestDisplay(t *testing.T) {
	cases := []struct {
		u    User
		want string
	}{
		{User{Username: "alice", Email: "a@x.com", Sub: "s"}, "alice"},
		{User{Email: "a@x.com", Sub: "s"}, "a@x.com"},
		{User{Sub: "s"}, "s"},
		{User{}, ""},
	}
	for _, tc := range cases {
		if got := tc.u.Display(); got != tc.want {
			t.Errorf("Display(%+v) = %q, want %q", tc.u, got, tc.want)
		}
	}
}

func TestContextRoundTrip(t *testing.T) {
	ctx := context.Background()
	if _, ok := FromContext(ctx); ok {
		t.Fatal("空 ctx 不应有身份")
	}
	u := User{Sub: "sub-1", Username: "alice"}
	if got, ok := FromContext(WithUser(ctx, u)); !ok || got != u {
		t.Fatalf("got %+v ok=%v, want %+v", got, ok, u)
	}
	if _, ok := FromContext(WithUser(ctx, User{})); ok {
		t.Fatal("零值身份应视为无身份")
	}
}
