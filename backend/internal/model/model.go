// Package model 定义平台的核心领域模型:文档、提取字段、校验问题与处理状态。
package model

import "time"

// Status 是文档在处理流水线中的状态。
// 流转:uploaded → extracted → validated|needs_review → saved;任一阶段出错 → failed。
type Status string

const (
	StatusUploaded    Status = "uploaded"     // 已上传归档,待提取
	StatusExtracted   Status = "extracted"    // 已完成文字识别/解析与字段提取
	StatusValidated   Status = "validated"    // 规则校验通过,可入库
	StatusNeedsReview Status = "needs_review" // 校验未通过或置信度不足,待人工审核
	StatusSaved       Status = "saved"        // 结构化数据已确认入库
	StatusFailed      Status = "failed"       // 处理失败,详见 Error
)

// 特殊单据类型(不在 doctypes 配置内):
//   - DocTypeAuto:上传时未指定类型,提取前先自动分类;
//   - DocTypeUnknown:自动分类失败,已走开放提取,待人工确认类型。
const (
	DocTypeAuto    = "auto"
	DocTypeUnknown = "unknown"
)

// Field 是从文档中提取出的一个字段。
// Confidence 可选:WorkBuddy 回传字段时通常不带自评置信度(缺省为 0,校验时跳过该维度)。
type Field struct {
	Name       string  `json:"name"`
	Value      string  `json:"value"`
	Confidence float64 `json:"confidence,omitempty"`
}

// ValidationIssue 是规则校验发现的一个问题。
type ValidationIssue struct {
	Field   string `json:"field"`
	Rule    string `json:"rule"`
	Message string `json:"message"`
}

// Document 是一份被处理的文档及其全部处理结果。
type Document struct {
	ID         string            `json:"id"`
	DocType    string            `json:"doc_type"`
	Filename   string            `json:"filename"`
	ObjectKey  string            `json:"object_key"` // MinIO 中的归档位置
	Text       string            `json:"-"`          // 识别/解析后的正文,查询结果默认不回传
	Status     Status            `json:"status"`
	Error      string            `json:"error,omitempty"`
	Fields     []Field           `json:"fields"`
	Issues     []ValidationIssue `json:"issues"`
	UploadedBy string            `json:"uploaded_by,omitempty"` // 上传人(OAuth 身份展示口径,或 'admin')
	ReviewedBy string            `json:"reviewed_by,omitempty"` // 入库确认人
	CreatedAt  time.Time         `json:"created_at"`
	UpdatedAt  time.Time         `json:"updated_at"`
}
