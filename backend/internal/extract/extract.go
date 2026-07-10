// Package extract 实现 AI 字段提取与单据分类。
//
// Extractor 是唯一入口接口;当前提供两个实现:
//   - Mock:基于"标签: 值"行匹配的确定性实现,开发/演示用,无外部依赖。
//   - LLM:OpenAI 兼容端点(DeepSeek/Qwen/客户自备模型),配置了 endpoint 即启用。
//
// 三种能力对应流水线的三种场景:
//   - Classify:类型未知时,在已配置的单据类型中自动识别;
//   - Extract:类型已知,按 schema 定向提取字段;
//   - ExtractOpen:类型识别失败的兜底,提取文档中实际存在的全部键值对,交人工审核。
//
// 提取逻辑始终在平台内执行(见产品说明书 §13),模型只是可替换的资源。
package extract

import (
	"context"

	"github.com/yuanyuexiang/phoenix/internal/model"
	"github.com/yuanyuexiang/phoenix/internal/schema"
)

// Candidate 是分类的候选单据类型。Labels 是该类型各字段的中文标签,
// 供启发式匹配/提示词判断文档与类型的契合程度。
type Candidate struct {
	Name        string
	Title       string
	Description string
	Labels      []string
}

// Extractor 提供字段提取与单据分类能力。
type Extractor interface {
	// Extract 按单据类型 schema 定向提取。未找到的字段返回空值、置信度 0。
	Extract(ctx context.Context, text string, dt *schema.DocType) ([]model.Field, error)
	// ExtractOpen 开放提取:不套 schema,抽取文档中实际存在的键值对。
	ExtractOpen(ctx context.Context, text string) ([]model.Field, error)
	// Classify 在候选类型中识别文档类型,返回类型名与 0~1 置信度;无法判断时返回 ("", 0)。
	Classify(ctx context.Context, text string, candidates []Candidate) (string, float64, error)
	Name() string
}
