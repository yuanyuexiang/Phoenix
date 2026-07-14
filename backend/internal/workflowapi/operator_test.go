package workflowapi

import (
	"net/http/httptest"
	"testing"

	"github.com/yuanyuexiang/phoenix/internal/identity"
)

func TestOperatorOf(t *testing.T) {
	cases := []struct {
		name       string
		headers    map[string]string
		wantActor  string
		wantSource string
	}{
		{
			name:       "mcp 透传 OAuth 身份",
			headers:    map[string]string{identity.HeaderSource: "mcp", identity.HeaderSub: "sub-1", identity.HeaderName: "alice"},
			wantActor:  "alice",
			wantSource: "oauth",
		},
		{
			name:       "mcp 透传中文姓名(percent-encoded)",
			headers:    map[string]string{identity.HeaderSource: "mcp", identity.HeaderSub: "sub-2", identity.HeaderName: "%E5%BC%A0%E4%B8%89"},
			wantActor:  "张三",
			wantSource: "oauth",
		},
		{
			name:       "mcp 无 token(OAuth off/optional)",
			headers:    map[string]string{identity.HeaderSource: "mcp"},
			wantActor:  "",
			wantSource: "anonymous",
		},
		{
			name:       "管理后台/脚本直连",
			headers:    nil,
			wantActor:  "admin",
			wantSource: "admin",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest("POST", "/api/documents", nil)
			for k, v := range tc.headers {
				r.Header.Set(k, v)
			}
			actor, source, _ := operatorOf(r)
			if actor != tc.wantActor || source != tc.wantSource {
				t.Fatalf("operatorOf = (%q, %q), want (%q, %q)", actor, source, tc.wantActor, tc.wantSource)
			}
		})
	}
}
