package validate

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/yuanyuexiang/phoenix/internal/model"
	"github.com/yuanyuexiang/phoenix/internal/schema"
)

// Normalize 按字段声明类型归一化字段值(number 去千分位/币符转纯数值,
// date 转 ISO 2006-01-02),在校验前调用 —— 归一化后的值既落 documents.fields
// 也是本体物化的属性来源。解析失败时保留原值(交由 pattern 校验暴露问题)。
func Normalize(fields []model.Field, dt *schema.DocType) []model.Field {
	specs := map[string]schema.FieldSpec{}
	for _, s := range dt.Fields {
		specs[s.Name] = s
	}
	out := make([]model.Field, len(fields))
	for i, f := range fields {
		out[i] = f
		spec, ok := specs[f.Name]
		if !ok || f.Value == "" {
			continue
		}
		switch spec.Type {
		case "number":
			if v, err := ParseNumber(f.Value); err == nil {
				out[i].Value = FormatNumber(v)
			}
		case "date":
			if v, err := ParseDate(f.Value); err == nil {
				out[i].Value = v
			}
		}
	}
	return out
}

// ParseNumber 解析常见金额写法:去空格/币符(¥￥)/千分位逗号,支持中文全角逗号。
func ParseNumber(s string) (float64, error) {
	c := strings.NewReplacer(" ", "", " ", "", "¥", "", "￥", "", ",", "", "，", "", "元", "").Replace(strings.TrimSpace(s))
	return strconv.ParseFloat(c, 64)
}

// FormatNumber 输出规范数值字符串(保留必要小数,金额常见两位)。
func FormatNumber(v float64) string {
	s := strconv.FormatFloat(v, 'f', -1, 64)
	// 12800 → 12800;12800.5 → 12800.5;不强补两位小数,保持无损
	return s
}

var dateRe = regexp.MustCompile(`^(\d{4})\s*[-/年.]\s*(\d{1,2})\s*[-/月.]\s*(\d{1,2})\s*日?$`)

// ParseDate 解析常见日期写法(2026年7月1日 / 2026-7-1 / 2026/07/01 / 2026.7.1)→ ISO。
func ParseDate(s string) (string, error) {
	m := dateRe.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return "", fmt.Errorf("无法识别的日期: %q", s)
	}
	y, _ := strconv.Atoi(m[1])
	mo, _ := strconv.Atoi(m[2])
	d, _ := strconv.Atoi(m[3])
	if mo < 1 || mo > 12 || d < 1 || d > 31 {
		return "", fmt.Errorf("日期越界: %q", s)
	}
	return fmt.Sprintf("%04d-%02d-%02d", y, mo, d), nil
}
