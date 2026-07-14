package mcpserver

import (
	"testing"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestUserFromRequest(t *testing.T) {
	t.Run("nil 请求", func(t *testing.T) {
		if _, ok := userFromRequest(nil); ok {
			t.Fatal("nil 请求不应有身份")
		}
	})
	t.Run("无 Extra(OAuth 关闭)", func(t *testing.T) {
		if _, ok := userFromRequest(&mcp.CallToolRequest{}); ok {
			t.Fatal("无 Extra 不应有身份")
		}
	})
	t.Run("无 TokenInfo(optional 匿名)", func(t *testing.T) {
		req := &mcp.CallToolRequest{Extra: &mcp.RequestExtra{}}
		if _, ok := userFromRequest(req); ok {
			t.Fatal("无 TokenInfo 不应有身份")
		}
	})
	t.Run("带 TokenInfo", func(t *testing.T) {
		req := &mcp.CallToolRequest{Extra: &mcp.RequestExtra{
			TokenInfo: &auth.TokenInfo{
				UserID: "sub-1",
				Extra:  map[string]any{"username": "alice", "email": "a@x.com", "name": "Alice"},
			},
		}}
		u, ok := userFromRequest(req)
		if !ok {
			t.Fatal("应取到身份")
		}
		if u.Sub != "sub-1" || u.Username != "alice" || u.Email != "a@x.com" || u.Name != "Alice" {
			t.Fatalf("身份不完整: %+v", u)
		}
	})
}
