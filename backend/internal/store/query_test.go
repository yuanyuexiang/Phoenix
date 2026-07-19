package store

import (
	"strings"
	"testing"
)

func TestFieldFilterCond(t *testing.T) {
	t.Run("eq", func(t *testing.T) {
		var args []any
		cond, err := fieldFilterCond(FieldFilter{Field: "doc_no", Op: "eq", Value: "PHX-1"}, &args)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(cond, "jsonb_array_elements(fields)") || !strings.Contains(cond, "fe->>'name' = $1") || !strings.Contains(cond, "fe->>'value' = $2") {
			t.Fatalf("eq 条件异常: %s", cond)
		}
		if len(args) != 2 || args[0] != "doc_no" || args[1] != "PHX-1" {
			t.Fatalf("args = %v", args)
		}
	})

	t.Run("contains 用 ILIKE 且包裹 %", func(t *testing.T) {
		var args []any
		cond, _ := fieldFilterCond(FieldFilter{Field: "party_a", Op: "contains", Value: "某公司"}, &args)
		if !strings.Contains(cond, "ILIKE $2") {
			t.Fatalf("contains 应用 ILIKE: %s", cond)
		}
		if args[1] != "%某公司%" {
			t.Fatalf("contains 值应包裹 %%: %v", args[1])
		}
	})

	t.Run("in 用 ANY", func(t *testing.T) {
		var args []any
		cond, err := fieldFilterCond(FieldFilter{Field: "status", Op: "in", Values: []string{"a", "b"}}, &args)
		if err != nil || !strings.Contains(cond, "= ANY($2)") {
			t.Fatalf("in 异常: %s, %v", cond, err)
		}
		vals, ok := args[1].([]string)
		if !ok || len(vals) != 2 {
			t.Fatalf("in values = %v", args[1])
		}
	})

	t.Run("gt 数值:去逗号 + 正则守卫 + cast", func(t *testing.T) {
		var args []any
		cond, err := fieldFilterCond(FieldFilter{Field: "amount", Op: "gt", Value: "10,000"}, &args)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(cond, "replace(fe->>'value', ',', '')") || !strings.Contains(cond, "~ '^-?[0-9]") || !strings.Contains(cond, "::numeric > $2::numeric") {
			t.Fatalf("gt 条件异常: %s", cond)
		}
		if args[1] != "10000" { // 过滤值也去逗号
			t.Fatalf("gt 过滤值应去逗号: %v", args[1])
		}
	})

	t.Run("错误:空 field / 未知 op / in 无 values", func(t *testing.T) {
		var args []any
		if _, err := fieldFilterCond(FieldFilter{Op: "eq", Value: "x"}, &args); err == nil {
			t.Error("空 field 应报错")
		}
		if _, err := fieldFilterCond(FieldFilter{Field: "f", Op: "like", Value: "x"}, &args); err == nil {
			t.Error("未知 op 应报错")
		}
		if _, err := fieldFilterCond(FieldFilter{Field: "f", Op: "in"}, &args); err == nil {
			t.Error("in 无 values 应报错")
		}
	})
}
