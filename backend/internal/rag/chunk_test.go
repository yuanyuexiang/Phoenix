package rag

import (
	"strings"
	"testing"
)

func TestSplit(t *testing.T) {
	t.Run("空串返回 nil", func(t *testing.T) {
		if Split("   \n  ") != nil {
			t.Fatal("空串应返回 nil")
		}
	})

	t.Run("短文本按段落合并为少量块", func(t *testing.T) {
		text := "服务合作确认单\n\n编号: PHX-1\n金额: 100\n\n以上确认无误。"
		chunks := Split(text)
		if len(chunks) == 0 {
			t.Fatal("应产出至少一个块")
		}
		joined := strings.Join(chunks, "")
		for _, kw := range []string{"PHX-1", "确认无误"} {
			if !strings.Contains(joined, kw) {
				t.Errorf("切片丢失内容 %q", kw)
			}
		}
	})

	t.Run("超长段落滑窗切分且带重叠", func(t *testing.T) {
		long := strings.Repeat("甲", 2500) // 单段远超 maxRunes
		chunks := Split(long)
		if len(chunks) < 2 {
			t.Fatalf("超长段应切成多块,得到 %d", len(chunks))
		}
		for _, c := range chunks {
			if n := len([]rune(c)); n > maxRunes {
				t.Errorf("块长 %d 超过上限 %d", n, maxRunes)
			}
		}
	})
}
