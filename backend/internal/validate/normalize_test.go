package validate

import (
	"testing"

	"github.com/yuanyuexiang/phoenix/internal/model"
	"github.com/yuanyuexiang/phoenix/internal/schema"
)

func TestParseNumber(t *testing.T) {
	cases := map[string]float64{
		"128,000.00": 128000, "¥12,800.00": 12800, "￥1,472.57": 1472.57,
		"12800": 12800, "1，000元": 1000,
	}
	for in, want := range cases {
		got, err := ParseNumber(in)
		if err != nil || got != want {
			t.Errorf("ParseNumber(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	if _, err := ParseNumber("十万"); err == nil {
		t.Error("非数值应报错")
	}
}

func TestParseDate(t *testing.T) {
	cases := map[string]string{
		"2026年7月1日": "2026-07-01", "2026-7-1": "2026-07-01",
		"2026/07/01": "2026-07-01", "2026.7.1": "2026-07-01",
	}
	for in, want := range cases {
		got, err := ParseDate(in)
		if err != nil || got != want {
			t.Errorf("ParseDate(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	if _, err := ParseDate("上周三"); err == nil {
		t.Error("非日期应报错")
	}
}

func TestNormalize(t *testing.T) {
	dt := &schema.DocType{Name: "t", Fields: []schema.FieldSpec{
		{Name: "amount", Label: "金额", Type: "number"},
		{Name: "date", Label: "日期", Type: "date"},
		{Name: "title", Label: "标题"},
	}}
	out := Normalize([]model.Field{
		{Name: "amount", Value: "128,000.00"},
		{Name: "date", Value: "2026年7月1日"},
		{Name: "title", Value: " 保持原样 "},
		{Name: "unknown", Value: "1,000"},
	}, dt)
	if out[0].Value != "128000" {
		t.Errorf("金额归一化 = %q", out[0].Value)
	}
	if out[1].Value != "2026-07-01" {
		t.Errorf("日期归一化 = %q", out[1].Value)
	}
	if out[2].Value != " 保持原样 " || out[3].Value != "1,000" {
		t.Error("非类型化/未声明字段不应被改动")
	}
}
