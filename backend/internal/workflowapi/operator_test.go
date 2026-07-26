package workflowapi

import (
	"net/http/httptest"
	"testing"
)

func TestOperatorOf(t *testing.T) {
	// 管理面共享密码,操作人统一记 'admin'(员工级身份走 /pub/v1,见 internal/restapi)
	r := httptest.NewRequest("POST", "/api/documents", nil)
	actor, source, u := operatorOf(r)
	if actor != "admin" || source != "admin" || !u.IsZero() {
		t.Fatalf("operatorOf = (%q, %q, %v), want (admin, admin, 空身份)", actor, source, u)
	}
}
