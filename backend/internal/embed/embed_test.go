package embed

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestEmbed(t *testing.T) {
	var gotPath, gotAuth string
	var gotReq embedRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &gotReq)
		// 故意乱序返回,验证按 index 归位
		_, _ = w.Write([]byte(`{"data":[{"index":1,"embedding":[0.3,0.4]},{"index":0,"embedding":[0.1,0.2]}]}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "k", "text-embedding-v3", 1024)
	vecs, err := c.Embed(context.Background(), []string{"甲", "乙"})
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != "/embeddings" || gotAuth != "Bearer k" {
		t.Fatalf("path=%q auth=%q", gotPath, gotAuth)
	}
	if gotReq.Model != "text-embedding-v3" || gotReq.Dimensions != 1024 || len(gotReq.Input) != 2 {
		t.Fatalf("请求异常: %+v", gotReq)
	}
	if len(vecs) != 2 || vecs[0][0] != 0.1 || vecs[1][0] != 0.3 {
		t.Fatalf("向量未按 index 归位: %v", vecs)
	}
}

func TestEmbedError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"error":{"message":"quota exceeded"}}`))
	}))
	defer srv.Close()
	_, err := New(srv.URL, "k", "m", 1024).Embed(context.Background(), []string{"x"})
	if err == nil || !strings.Contains(err.Error(), "quota exceeded") {
		t.Fatalf("应透传端点错误: %v", err)
	}
}
