package extract

import (
	"context"
	"regexp"
	"strings"

	"github.com/yuanyuexiang/phoenix/internal/model"
	"github.com/yuanyuexiang/phoenix/internal/schema"
)

// Mock 是确定性的"标签: 值"提取器。
// 对 schema 中的每个字段,用其 label 与 aliases 在正文中逐行匹配
// "标签[:：] 值" 形式;命中即取值,置信度固定 0.9。
// 用于开发期在没有大模型的情况下打通全流程,也可作为演示兜底。
type Mock struct{}

func (Mock) Name() string { return "mock" }

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
