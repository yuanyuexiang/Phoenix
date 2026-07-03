// Package validate 实现规则校验:按单据类型 schema 检查提取结果,
// 并依据置信度阈值决定是否转人工审核(产品说明书 §6)。
package validate

import (
	"fmt"
	"regexp"
	"slices"

	"github.com/yuanyuexiang/phoenix/internal/model"
	"github.com/yuanyuexiang/phoenix/internal/schema"
)

// Run 校验提取字段。无任何问题 → validated;否则 → needs_review,问题明细见返回值。
func Run(fields []model.Field, dt *schema.DocType, minConfidence float64) (model.Status, []model.ValidationIssue) {
	byName := map[string]model.Field{}
	for _, f := range fields {
		byName[f.Name] = f
	}

	var issues []model.ValidationIssue
	for _, spec := range dt.Fields {
		f, ok := byName[spec.Name]
		if !ok || f.Value == "" {
			if spec.Rule.Required {
				issues = append(issues, issue(spec, "required", "必填字段未提取到值"))
			}
			continue
		}
		if spec.Rule.Pattern != "" {
			re := regexp.MustCompile("^(?:" + spec.Rule.Pattern + ")$") // Load 时已校验可编译
			if !re.MatchString(f.Value) {
				issues = append(issues, issue(spec, "pattern", fmt.Sprintf("值 %q 不符合格式 %s", f.Value, spec.Rule.Pattern)))
			}
		}
		if len(spec.Rule.Enum) > 0 && !slices.Contains(spec.Rule.Enum, f.Value) {
			issues = append(issues, issue(spec, "enum", fmt.Sprintf("值 %q 不在允许范围 %v 内", f.Value, spec.Rule.Enum)))
		}
		if f.Confidence < minConfidence {
			issues = append(issues, issue(spec, "confidence", fmt.Sprintf("置信度 %.2f 低于阈值 %.2f", f.Confidence, minConfidence)))
		}
	}

	if len(issues) > 0 {
		return model.StatusNeedsReview, issues
	}
	return model.StatusValidated, nil
}

func issue(spec schema.FieldSpec, rule, msg string) model.ValidationIssue {
	return model.ValidationIssue{Field: spec.Name, Rule: rule, Message: fmt.Sprintf("%s(%s): %s", spec.Label, spec.Name, msg)}
}
