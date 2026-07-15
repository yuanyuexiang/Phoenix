package extract

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/yuanyuexiang/phoenix/internal/parser"
)

func TestVLMTranscribe(t *testing.T) {
	imgData := []byte("fake-png-bytes")
	var gotPath, gotAuth string
	var gotBody []byte

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		gotBody, _ = io.ReadAll(r.Body)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"发票号: 123\n"}}]}`))
	}))
	defer srv.Close()

	v := NewVLM(srv.URL, "test-key", "qwen-vl-plus")
	text, err := v.Transcribe(context.Background(), "sample.PNG", imgData)
	if err != nil {
		t.Fatalf("Transcribe: %v", err)
	}
	if text != "发票号: 123" {
		t.Errorf("text = %q(应 TrimSpace)", text)
	}
	if gotPath != "/chat/completions" {
		t.Errorf("path = %q", gotPath)
	}
	if gotAuth != "Bearer test-key" {
		t.Errorf("auth = %q", gotAuth)
	}
	// vision 请求不能带 response_format(DashScope vision 会拒),检查原始 JSON
	if strings.Contains(string(gotBody), "response_format") {
		t.Error("vision 请求不应携带 response_format")
	}

	var req visionChatRequest
	if err := json.Unmarshal(gotBody, &req); err != nil {
		t.Fatal(err)
	}
	if req.Model != "qwen-vl-plus" || req.Temperature != 0 {
		t.Errorf("model=%q temperature=%v", req.Model, req.Temperature)
	}
	if len(req.Messages) != 1 || len(req.Messages[0].Content) != 2 {
		t.Fatalf("content 应为 [image_url, text] 两段,得到 %+v", req.Messages)
	}
	img, txt := req.Messages[0].Content[0], req.Messages[0].Content[1]
	if img.Type != "image_url" || img.ImageURL == nil {
		t.Fatalf("第一段应为 image_url: %+v", img)
	}
	const prefix = "data:image/png;base64,"
	if !strings.HasPrefix(img.ImageURL.URL, prefix) {
		t.Fatalf("data URL 前缀错误(扩展名应大小写不敏感): %.60s", img.ImageURL.URL)
	}
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(img.ImageURL.URL, prefix))
	if err != nil || string(decoded) != string(imgData) {
		t.Errorf("base64 payload 还原失败: %v", err)
	}
	if txt.Type != "text" || !strings.Contains(txt.Text, "转写") {
		t.Errorf("第二段应为转写 prompt: %+v", txt)
	}
}

func TestVLMTranscribeErrors(t *testing.T) {
	t.Run("模型端错误透传", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":{"message":"image too small"}}`))
		}))
		defer srv.Close()
		_, err := NewVLM(srv.URL, "k", "m").Transcribe(context.Background(), "a.png", []byte("x"))
		if err == nil || !strings.Contains(err.Error(), "image too small") {
			t.Fatalf("应透传模型端错误: %v", err)
		}
	})

	t.Run("不支持的扩展名", func(t *testing.T) {
		_, err := NewVLM("http://unused", "k", "m").Transcribe(context.Background(), "a.gif", []byte("x"))
		if err == nil || !strings.Contains(err.Error(), "不支持") {
			t.Fatalf("应拒绝未知格式: %v", err)
		}
	})

	t.Run("超过 10MB 守卫", func(t *testing.T) {
		big := make([]byte, 8<<20) // base64 后约 10.7MB
		_, err := NewVLM("http://unused", "k", "m").Transcribe(context.Background(), "a.png", big)
		if err == nil || !strings.Contains(err.Error(), "10MB") {
			t.Fatalf("应触发大小守卫: %v", err)
		}
	})
}

func TestStripTranscriptFence(t *testing.T) {
	cases := []struct{ in, want string }{
		{"发票号: 123", "发票号: 123"},
		{"```markdown\n| 编号: 1 |\n```", "| 编号: 1 |"},
		{"```\n正文\n```", "正文"},
		{"含 ``` 在中间的正文", "含 ``` 在中间的正文"},
	}
	for _, tc := range cases {
		if got := stripTranscriptFence(tc.in); got != tc.want {
			t.Errorf("stripTranscriptFence(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// imageMIME 与 parser.ImageExts 是路由与编码两张表,必须同步。
func TestImageExtsHaveMIME(t *testing.T) {
	for ext := range parser.ImageExts {
		if _, ok := imageMIME[ext]; !ok {
			t.Errorf("parser.ImageExts 含 %q 但 imageMIME 缺少对应 MIME", ext)
		}
	}
	for ext := range imageMIME {
		if !parser.ImageExts[ext] {
			t.Errorf("imageMIME 含 %q 但 parser.ImageExts 不路由它", ext)
		}
	}
}
