// Package extract 实现 AI 字段提取。
//
// Extractor 是唯一入口接口;当前提供两个实现:
//   - Mock:基于"标签: 值"行匹配的确定性提取器,开发/演示用,无外部依赖。
//   - LLM:OpenAI 兼容端点(DeepSeek/Qwen/客户自备模型),配置了 endpoint 即启用。
//
// 提取逻辑始终在平台内执行(见产品说明书 §13),模型只是可替换的资源。
package extract

import (
	"context"

	"github.com/yuanyuexiang/phoenix/internal/model"
	"github.com/yuanyuexiang/phoenix/internal/schema"
)

// Extractor 从文档正文中按单据类型 schema 提取字段。
// 未找到的字段应返回 Value 为空、Confidence 为 0 的条目,交由校验环节判定。
type Extractor interface {
	Extract(ctx context.Context, text string, dt *schema.DocType) ([]model.Field, error)
	Name() string
}
