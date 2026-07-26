package identity

import (
	"context"
	"testing"
)

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
