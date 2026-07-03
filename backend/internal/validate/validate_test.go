package validate

import (
	"testing"

	"github.com/yuanyuexiang/phoenix/internal/model"
	"github.com/yuanyuexiang/phoenix/internal/schema"
)

func testDocType() *schema.DocType {
	return &schema.DocType{
		Name: "test",
		Fields: []schema.FieldSpec{
			{Name: "doc_no", Label: "编号", Rule: schema.Rule{Required: true}},
			{Name: "amount", Label: "金额", Rule: schema.Rule{Pattern: `[0-9,]+(\.[0-9]{1,2})?`}},
			{Name: "kind", Label: "类型", Rule: schema.Rule{Enum: []string{"合同", "发票"}}},
		},
	}
}

func TestRunAllPass(t *testing.T) {
	fields := []model.Field{
		{Name: "doc_no", Value: "A-1", Confidence: 0.95},
		{Name: "amount", Value: "1,200.50", Confidence: 0.9},
		{Name: "kind", Value: "合同", Confidence: 0.99},
	}
	status, issues := Run(fields, testDocType(), 0.8)
	if status != model.StatusValidated || len(issues) != 0 {
		t.Fatalf("期望 validated 无问题,得到 %s %v", status, issues)
	}
}

func TestRunViolations(t *testing.T) {
	fields := []model.Field{
		// doc_no 缺失 → required
		{Name: "amount", Value: "十二万", Confidence: 0.9}, // pattern 不匹配
		{Name: "kind", Value: "收据", Confidence: 0.5},    // enum + confidence 双违规
	}
	status, issues := Run(fields, testDocType(), 0.8)
	if status != model.StatusNeedsReview {
		t.Fatalf("期望 needs_review,得到 %s", status)
	}
	rules := map[string]int{}
	for _, i := range issues {
		rules[i.Rule]++
	}
	for _, want := range []string{"required", "pattern", "enum", "confidence"} {
		if rules[want] == 0 {
			t.Errorf("缺少 %s 规则的问题,实际: %v", want, issues)
		}
	}
}

func TestRunOptionalEmptyFieldSkipped(t *testing.T) {
	fields := []model.Field{
		{Name: "doc_no", Value: "A-1", Confidence: 0.95},
		{Name: "amount", Value: "", Confidence: 0}, // 非必填且为空:不触发 pattern/confidence
		{Name: "kind", Value: "发票", Confidence: 0.9},
	}
	status, issues := Run(fields, testDocType(), 0.8)
	if status != model.StatusValidated {
		t.Fatalf("非必填空字段不应触发校验问题,得到 %s %v", status, issues)
	}
}
