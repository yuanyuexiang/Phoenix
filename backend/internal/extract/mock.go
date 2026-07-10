package extract

import (
	"context"
	"regexp"
	"strings"

	"github.com/yuanyuexiang/phoenix/internal/model"
	"github.com/yuanyuexiang/phoenix/internal/schema"
)

// Mock 是确定性的"标签: 值"实现。
// 用于开发期在没有大模型的情况下打通全流程,也可作为演示兜底。
type Mock struct{}

func (Mock) Name() string { return "mock" }

// Extract 对 schema 中的每个字段,用其 label 与 aliases 在正文中逐行匹配
// "标签[:：] 值" 形式;命中即取值,置信度固定 0.9。
func (Mock) Extract(_ context.Context, text string, dt *schema.DocType) ([]model.Field, error) {
	lines := strings.Split(text, "\n")
	fields := make([]model.Field, 0, len(dt.Fields))
	for _, spec := range dt.Fields {
		value := findLabeledValue(lines, spec)
		f := model.Field{Name: spec.Name}
		if value != "" {
			f.Value = value
			f.Confidence = 0.9
		}
		fields = append(fields, f)
	}
	return fields, nil
}

// openKV 匹配 "键: 值" 行;键限制在 30 字符内且不含冒号,避免把正文长句误判成键值对。
var openKV = regexp.MustCompile(`^\s*([^:：\s][^:：]{0,29}?)\s*[:：]\s*(.+?)\s*$`)

const openMaxFields = 50

// ExtractOpen 开放提取:抽取全文中所有 "键: 值" 行,键名原样保留,置信度 0.7。
// 用于类型识别失败的兜底,结果交人工审核定夺。
func (Mock) ExtractOpen(_ context.Context, text string) ([]model.Field, error) {
	seen := map[string]bool{}
	var fields []model.Field
	for _, line := range strings.Split(text, "\n") {
		m := openKV.FindStringSubmatch(line)
		if m == nil || seen[m[1]] {
			continue
		}
		seen[m[1]] = true
		fields = append(fields, model.Field{Name: m[1], Value: m[2], Confidence: 0.7})
		if len(fields) >= openMaxFields {
			break
		}
	}
	return fields, nil
}

// Classify 启发式分类:统计每个候选类型的字段标签在文档中以 "标签: 值" 形式
// 出现的比例,取最高者;比例即置信度。
func (Mock) Classify(_ context.Context, text string, candidates []Candidate) (string, float64, error) {
	lines := strings.Split(text, "\n")
	best, bestScore := "", 0.0
	for _, c := range candidates {
		if len(c.Labels) == 0 {
			continue
		}
		hit := 0
		for _, label := range c.Labels {
			if findLabeledValue(lines, schema.FieldSpec{Label: label}) != "" {
				hit++
			}
		}
		score := float64(hit) / float64(len(c.Labels))
		if score > bestScore {
			best, bestScore = c.Name, score
		}
	}
	return best, bestScore, nil
}

func findLabeledValue(lines []string, spec schema.FieldSpec) string {
	labels := append([]string{spec.Label}, spec.Aliases...)
	for _, label := range labels {
		re := regexp.MustCompile(`^\s*` + regexp.QuoteMeta(label) + `\s*[:：]\s*(.+?)\s*$`)
		for _, line := range lines {
			if m := re.FindStringSubmatch(line); m != nil {
				return m[1]
			}
		}
	}
	return ""
}
