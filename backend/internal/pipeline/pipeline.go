// Package pipeline 是工作流引擎(说明书 §7)的编排核心。
//
// 内容识别/字段提取由 WorkBuddy(多模态大模型)在客户端完成;后端只做:
//
//	上传归档(MinIO) → [WorkBuddy 识别] → 回传字段+正文 → 规则校验 → 入库
//
// 每个阶段状态持久化在 documents.status,调用方(MCP/管理后台)可分步驱动。
package pipeline

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/yuanyuexiang/phoenix/internal/api"
	"github.com/yuanyuexiang/phoenix/internal/model"
	"github.com/yuanyuexiang/phoenix/internal/rag"
	"github.com/yuanyuexiang/phoenix/internal/schema"
	"github.com/yuanyuexiang/phoenix/internal/store"
	"github.com/yuanyuexiang/phoenix/internal/validate"
)

// Embedder 把文本向量化(RAG 知识库)。nil = 知识库未启用(不入向量、ask 返回未启用)。
type Embedder interface {
	Embed(ctx context.Context, texts []string) ([][]float32, error)
}

type Pipeline struct {
	DB            *store.DB
	Objects       *store.Objects
	Registry      *schema.Registry
	Embedder      Embedder // 可为 nil(知识库关闭)
	MinConfidence float64  // 字段置信度低于此值转人工(仅当客户端回传了置信度)
}

// Upload 实现 upload_document:归档原始文件到 MinIO 并登记任务。
// docType 为空或 "auto" 时按待定类型处理(由 WorkBuddy 在提取阶段确定)。
// uploadedBy 是操作人展示口径(可为空),由 API 层从请求头解析(workflowapi.operatorOf)。
func (p *Pipeline) Upload(ctx context.Context, docType, filename string, data []byte, uploadedBy string) (*model.Document, error) {
	if docType == "" {
		docType = model.DocTypeAuto
	}
	if docType != model.DocTypeAuto {
		if _, ok := p.Registry.Get(docType); !ok {
			return nil, fmt.Errorf("未知的单据类型 %q,可用类型: %v(或留空/auto 待定)", docType, p.Registry.Names())
		}
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("文件内容为空")
	}

	id := uuid.NewString()
	key := fmt.Sprintf("%s/%s/%s/%s", docType, time.Now().UTC().Format("2006/01"), id, filename)
	if err := p.Objects.Put(ctx, key, data, ""); err != nil {
		return nil, fmt.Errorf("归档文件失败: %w", err)
	}

	doc := &model.Document{
		ID:         id,
		DocType:    docType,
		Filename:   filename,
		ObjectKey:  key,
		Status:     model.StatusUploaded,
		UploadedBy: uploadedBy,
	}
	if err := p.DB.CreateDocument(ctx, doc); err != nil {
		return nil, fmt.Errorf("登记文档失败: %w", err)
	}
	return doc, nil
}

// FieldBrief 实现 extract_fields 的新语义:后端不识别,只下发"该抽哪些字段"。
// doc 的类型已配置 → 返回该类型的字段清单;类型 auto/unknown → 返回全部类型目录供 WorkBuddy 选型。
func (p *Pipeline) FieldBrief(ctx context.Context, id string) (api.FieldBrief, error) {
	doc, err := p.DB.GetDocument(ctx, id)
	if err != nil {
		return api.FieldBrief{}, fmt.Errorf("文档 %s 不存在", id)
	}
	if dt, ok := p.Registry.Get(doc.DocType); ok {
		return briefOf(dt), nil
	}
	// 类型未定:给目录供 WorkBuddy 选型
	brief := api.FieldBrief{DocType: doc.DocType}
	for _, name := range p.Registry.Names() {
		dt, _ := p.Registry.Get(name)
		brief.Catalog = append(brief.Catalog, api.DocTypeDigest{Name: dt.Name, Title: dt.Title, Description: dt.Description})
	}
	return brief, nil
}

func briefOf(dt *schema.DocType) api.FieldBrief {
	brief := api.FieldBrief{DocType: dt.Name, Title: dt.Title}
	for _, f := range dt.Fields {
		brief.Fields = append(brief.Fields, api.BriefField{
			Name:        f.Name,
			Label:       f.Label,
			Description: f.Description,
			Aliases:     f.Aliases,
			Required:    f.Rule.Required,
			Pattern:     f.Rule.Pattern,
			Enum:        f.Rule.Enum,
		})
	}
	return brief
}

// Validate 实现 validate_document:对 WorkBuddy 回传的字段做 schema 预校验(不置 saved)。
// docType 非空时以其为准(WorkBuddy 定的类型)。
func (p *Pipeline) Validate(ctx context.Context, id string, fields []model.Field, docType string) (*model.Document, error) {
	doc, err := p.DB.GetDocument(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("文档 %s 不存在", id)
	}
	if docType != "" {
		doc.DocType = docType
	}
	if len(fields) > 0 {
		doc.Fields = fields
	}

	dt, ok := p.Registry.Get(doc.DocType)
	if !ok {
		// 类型未识别/未配置:无 schema 可校验,转人工定类型
		doc.Status = model.StatusNeedsReview
		doc.Issues = []model.ValidationIssue{{
			Field:   "doc_type",
			Rule:    "classify",
			Message: fmt.Sprintf("单据类型 %q 未识别或未配置,请人工确认类型与字段", doc.DocType),
		}}
		if err := p.DB.UpdateDocument(ctx, doc); err != nil {
			return nil, err
		}
		return doc, nil
	}

	status, issues := validate.Run(doc.Fields, dt, p.MinConfidence)
	doc.Status = status
	doc.Issues = issues
	if err := p.DB.UpdateDocument(ctx, doc); err != nil {
		return nil, err
	}
	return doc, nil
}

// Save 实现 save_database:落字段+正文,服务端权威校验后入库。
// WorkBuddy 走「upload → save(带 fields+contentText)」直接入库,无需先 extract/validate;
// 校验就地跑一次:不通过且未 force → needs_review(非报错,回带 issues 交客户端转述)。
// reviewedBy 记为「入库确认人」。
func (p *Pipeline) Save(ctx context.Context, id string, fields []model.Field, contentText, docType string, force bool, reviewedBy string) (*model.Document, error) {
	doc, err := p.DB.GetDocument(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("文档 %s 不存在", id)
	}
	if doc.Status == model.StatusSaved {
		return doc, nil // 幂等
	}
	if docType != "" {
		doc.DocType = docType
	}
	if len(fields) > 0 {
		doc.Fields = fields
	}
	if contentText != "" {
		doc.Text = contentText // 正文落库(检索与知识库源)
	}

	dt, ok := p.Registry.Get(doc.DocType)
	if !ok {
		return nil, fmt.Errorf("单据类型 %q 未配置,无法入库,请先确认类型", doc.DocType)
	}

	status, issues := validate.Run(doc.Fields, dt, p.MinConfidence)
	doc.Issues = issues
	if status == model.StatusNeedsReview && !force {
		doc.Status = model.StatusNeedsReview
		if err := p.DB.UpdateDocument(ctx, doc); err != nil {
			return nil, err
		}
		return doc, nil // 未入库,回带 issues 供 WorkBuddy 请用户确认或修正
	}

	doc.Status = model.StatusSaved
	doc.Error = ""
	doc.ReviewedBy = reviewedBy
	if err := p.DB.UpdateDocument(ctx, doc); err != nil {
		return nil, err
	}
	p.ingest(ctx, doc) // 知识库入库(best-effort,失败不阻断)
	return doc, nil
}

// ingest 把正文切片 + 向量化后存入知识库。best-effort:未启用/正文空/embedding 故障时
// 仅告警,不影响主流程(结构化数据与归档已落库)。
func (p *Pipeline) ingest(ctx context.Context, doc *model.Document) {
	if p.Embedder == nil || strings.TrimSpace(doc.Text) == "" {
		return
	}
	parts := rag.Split(doc.Text)
	if len(parts) == 0 {
		return
	}
	vecs, err := p.Embedder.Embed(ctx, parts)
	if err != nil {
		slog.Warn("知识库入库失败(可后续重建)", "document_id", doc.ID, "error", err)
		return
	}
	chunks := make([]store.Chunk, 0, len(parts))
	for i, c := range parts {
		chunks = append(chunks, store.Chunk{Index: i, Content: c, Embedding: vecs[i]})
	}
	if err := p.DB.ReplaceChunks(ctx, doc.ID, chunks); err != nil {
		slog.Warn("知识库写入失败", "document_id", doc.ID, "error", err)
	}
}

// Query 实现 query_document。
func (p *Pipeline) Query(ctx context.Context, f store.QueryFilter) ([]*model.Document, error) {
	return p.DB.QueryDocuments(ctx, f)
}

// Delete 删除一份文档:结构化行 + 知识库切片(级联)+ MinIO 归档原件。
// 仅供管理后台使用(不暴露给 WorkBuddy)。
func (p *Pipeline) Delete(ctx context.Context, id string) error {
	doc, err := p.DB.GetDocument(ctx, id)
	if err != nil {
		return fmt.Errorf("文档 %s 不存在", id)
	}
	// 先删数据库行(级联清 document_chunks),再 best-effort 删归档对象——
	// 对象删失败只是残留孤儿文件,不会造成"行指向不存在文件"的坏引用。
	if err := p.DB.DeleteDocument(ctx, id); err != nil {
		return fmt.Errorf("删除文档记录失败: %w", err)
	}
	if doc.ObjectKey != "" {
		if err := p.Objects.Remove(ctx, doc.ObjectKey); err != nil {
			slog.Warn("删除归档文件失败(孤儿对象可后续清理)", "document_id", id, "object_key", doc.ObjectKey, "error", err)
		}
	}
	return nil
}

// Ask 实现 ask_document:把问题向量化后做语义检索,返回相关正文片段与来源文档。
func (p *Pipeline) Ask(ctx context.Context, question string, limit int, docType string) ([]store.ChunkHit, error) {
	if p.Embedder == nil {
		return nil, fmt.Errorf("知识库未启用:请为 workflow 配置 PHX_EMBED_ENDPOINT / PHX_EMBED_API_KEY")
	}
	if strings.TrimSpace(question) == "" {
		return nil, fmt.Errorf("question 不能为空")
	}
	vecs, err := p.Embedder.Embed(ctx, []string{question})
	if err != nil {
		return nil, fmt.Errorf("问题向量化失败: %w", err)
	}
	return p.DB.SearchChunks(ctx, vecs[0], limit, docType, "")
}
