package userauth

import (
	"strings"
	"testing"
	"time"
)

func TestPasswordHashRoundTrip(t *testing.T) {
	h := HashPassword("s3cret-口令")
	if !strings.HasPrefix(h, "pbkdf2$sha256$") {
		t.Fatalf("哈希格式异常: %s", h)
	}
	if !VerifyPassword(h, "s3cret-口令") {
		t.Fatal("正确口令应通过校验")
	}
	if VerifyPassword(h, "wrong") {
		t.Fatal("错误口令不应通过校验")
	}
	if VerifyPassword("garbage", "s3cret-口令") {
		t.Fatal("非法哈希不应通过校验")
	}
	if h2 := HashPassword("s3cret-口令"); h2 == h {
		t.Fatal("相同口令应因随机盐得到不同哈希")
	}
}

func TestJWTIssueVerify(t *testing.T) {
	s := New([]byte("test-secret"))
	access, refresh, expiresIn := s.Issue("alice", "Alice", "a@x.com", 3)
	if expiresIn != int(DefaultAccessTTL.Seconds()) {
		t.Fatalf("expiresIn = %d", expiresIn)
	}

	c, err := s.Verify(access, TypAccess)
	if err != nil {
		t.Fatalf("access 校验失败: %v", err)
	}
	if c.Sub != "alice" || c.Ver != 3 || c.Email != "a@x.com" {
		t.Fatalf("claims 不符: %+v", c)
	}

	if _, err := s.Verify(access, TypRefresh); err == nil {
		t.Fatal("access token 不应通过 refresh 类型校验")
	}
	if _, err := s.Verify(refresh, TypRefresh); err != nil {
		t.Fatalf("refresh 校验失败: %v", err)
	}

	// 篡改载荷应被签名校验拦下
	parts := strings.Split(access, ".")
	tampered := parts[0] + "." + parts[1] + "x." + parts[2]
	if _, err := s.Verify(tampered, TypAccess); err == nil {
		t.Fatal("被篡改的 token 不应通过")
	}

	// 换密钥应失败
	if _, err := New([]byte("other")).Verify(access, TypAccess); err == nil {
		t.Fatal("错误密钥不应通过")
	}
}

func TestJWTExpiry(t *testing.T) {
	s := New([]byte("k"))
	s.now = func() time.Time { return time.Unix(1000, 0) }
	access, _, _ := s.Issue("bob", "", "", 1)
	s.now = func() time.Time { return time.Unix(1000 + int64(DefaultAccessTTL.Seconds()) + 1, 0) }
	if _, err := s.Verify(access, TypAccess); err == nil {
		t.Fatal("过期 token 不应通过")
	}
}
