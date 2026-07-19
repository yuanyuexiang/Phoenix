// Package rag 提供知识库入库/检索的辅助:正文切片(chunking)。
package rag

import "strings"

const (
	targetRunes  = 600  // 每块目标字符数(按 rune 计,适配中文)
	overlapRunes = 100  // 相邻块重叠字符数,保留上下文
	maxRunes     = 1000 // 单块硬上限
)

// Split 把正文按段落感知切片:先按空行分段,过长的段再按滑动窗口切,块间保留 overlap。
// 返回的每个切片非空、已 TrimSpace。
func Split(text string) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	paras := splitParagraphs(text)

	var chunks []string
	var buf []rune
	flush := func() {
		if len(buf) > 0 {
			if s := strings.TrimSpace(string(buf)); s != "" {
				chunks = append(chunks, s)
			}
			buf = buf[:0]
		}
	}
	for _, p := range paras {
		pr := []rune(p)
		// 超长段落:滑动窗口切,带 overlap
		if len(pr) > maxRunes {
			flush()
			for start := 0; start < len(pr); start += targetRunes - overlapRunes {
				end := start + targetRunes
				if end > len(pr) {
					end = len(pr)
				}
				if s := strings.TrimSpace(string(pr[start:end])); s != "" {
					chunks = append(chunks, s)
				}
				if end == len(pr) {
					break
				}
			}
			continue
		}
		// 累积到目标大小就 flush(段落间用换行拼接)
		if len(buf)+len(pr) > targetRunes && len(buf) > 0 {
			flush()
		}
		if len(buf) > 0 {
			buf = append(buf, '\n')
		}
		buf = append(buf, pr...)
	}
	flush()
	return chunks
}

// splitParagraphs 按空行(含多余空白)切段;无空行时退化为整段。
func splitParagraphs(text string) []string {
	lines := strings.Split(text, "\n")
	var paras []string
	var cur []string
	push := func() {
		if len(cur) > 0 {
			if s := strings.TrimSpace(strings.Join(cur, "\n")); s != "" {
				paras = append(paras, s)
			}
			cur = cur[:0]
		}
	}
	for _, ln := range lines {
		if strings.TrimSpace(ln) == "" {
			push()
			continue
		}
		cur = append(cur, ln)
	}
	push()
	if len(paras) == 0 {
		return []string{text}
	}
	return paras
}
