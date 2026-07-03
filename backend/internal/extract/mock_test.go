package extract

import (
	"context"
	"testing"

	"github.com/yuanyuexiang/phoenix/internal/schema"
)

func TestMockExtract(t *testing.T) {
	dt := &schema.DocType{
		Name: "test",
		Fields: []schema.FieldSpec{
			{Name: "doc_no", Label: "编号", Aliases: []string{"单据编号"}},
			{Name: "amount", Label: "金额", Aliases: []string{"价税合计"}},
			{Name: "missing", Label: "不存在的字段"},
		},
	}
	text := "标题头\n单据编号: ABC-001\n价税合计:1,234.56\n尾部"

	fields, err := Mock{}.Extract(context.Background(), text, dt)
	if err != nil {
		t.Fatal(err)
	}
	if len(fields) != 3 {
		t.Fatalf("期望 3 个字段(含未命中),得到 %d", len(fields))
	}

	got := map[string]string{}
	for _, f := range fields {
		got[f.Name] = f.Value
	}
	if got["doc_no"] != "ABC-001" {
		t.Errorf("doc_no: 期望通过别名+半角冒号命中 ABC-001,得到 %q", got["doc_no"])
	}
	if got["amount"] != "1,234.56" {
		t.Errorf("amount: 期望通过全角冒号命中 1,234.56,得到 %q", got["amount"])
	}
	if got["missing"] != "" {
		t.Errorf("missing: 未命中字段应为空值,得到 %q", got["missing"])
	}
	for _, f := range fields {
		if f.Name == "missing" && f.Confidence != 0 {
			t.Errorf("未命中字段置信度应为 0,得到 %v", f.Confidence)
		}
	}
}
