package extract

import (
	"context"
	"testing"

	"github.com/yuanyuexiang/phoenix/internal/schema"
)

func TestMockClassify(t *testing.T) {
	candidates := []Candidate{
		{Name: "contract", Labels: []string{"编号", "甲方", "乙方"}},
		{Name: "invoice", Labels: []string{"发票号码", "价税合计", "税额"}},
	}
	text := "编号: A-1\n甲方: 某公司\n乙方: 另一公司"

	name, conf, err := Mock{}.Classify(context.Background(), text, candidates)
	if err != nil {
		t.Fatal(err)
	}
	if name != "contract" || conf != 1.0 {
		t.Fatalf("期望 contract/1.0,得到 %s/%v", name, conf)
	}

	name, conf, _ = Mock{}.Classify(context.Background(), "无关内容", candidates)
	if name != "" || conf != 0 {
		t.Fatalf("无匹配时应返回空,得到 %s/%v", name, conf)
	}
}

func TestMockExtractOpen(t *testing.T) {
	text := "报销确认单\n工号: E1024\n事由: 差旅\n工号: 重复键应忽略\n这是一行不含键值对的正文,不应被提取。"
	fields, err := Mock{}.ExtractOpen(context.Background(), text)
	if err != nil {
		t.Fatal(err)
	}
	if len(fields) != 2 {
		t.Fatalf("期望 2 个字段(去重、忽略正文),得到 %d: %+v", len(fields), fields)
	}
	if fields[0].Name != "工号" || fields[0].Value != "E1024" {
		t.Errorf("首个字段不符: %+v", fields[0])
	}
}

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
