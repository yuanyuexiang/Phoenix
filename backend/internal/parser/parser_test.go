package parser

import (
	"strings"
	"testing"
)

func TestExtractTextRouting(t *testing.T) {
	if _, err := ExtractText("scan.tiff", []byte("x")); err == nil || !strings.Contains(err.Error(), "TIFF") {
		t.Errorf("tiff 应返回明确的不支持错误: %v", err)
	}
	if _, err := ExtractText("photo.png", []byte("x")); err == nil || !strings.Contains(err.Error(), "视觉转写") {
		t.Errorf("图片应提示交由 AI 视觉转写: %v", err)
	}
	if text, err := ExtractText("a.txt", []byte("hello")); err != nil || text != "hello" {
		t.Errorf("txt 解析异常: %q, %v", text, err)
	}
}
