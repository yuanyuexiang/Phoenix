// Package pipeline 是工作流引擎(说明书 §7)的编排核心:
//
//	上传 → OCR/解析 → AI字段提取 → 规则校验 → [人工审核] → 入库 → 归档
//
// 每个阶段状态持久化在 documents.status,调用方(MCP/管理后台)可分步驱动、
// 断点续跑。OCR、文档解析、AI 提取分别是独立服务,这里只做路由与编排:
// 图片 → OCR 服务;办公文档 → parser 服务;正文 → ai 服务。
package pipeline

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/yuanyuexiang/phoenix/internal/api"
	"github.com/yuanyuexiang/phoenix/internal/clients"
	"github.com/yuanyuexiang/phoenix/internal/model"
	"github.com/yuanyuexiang/phoenix/internal/ocr"
	"github.com/yuanyuexiang/phoenix/internal/parser"
	"github.com/yuanyuexiang/phoenix/internal/schema"
	"github.com/yuanyuexiang/phoenix/internal/store"
	"github.com/yuanyuexiang/phoenix/internal/validate"
)

type Pipeline struct {
	DB            *store.DB
	Objects       *store.Objects
	OCR           *ocr.Client     // OCR 服务(图片)
	Parser        *clients.Parser // 文档解析服务(PDF/Word/Excel)
	AI            *clients.AI     // AI 字段提取服务
	Registry      *schema.Registry
	MinConfidence float64
}

// Upload 实现 upload_document:归档原始文件到 MinIO 并登记任务。
func (p *Pipeline) Upload(ctx context.Context, docType, filename string, data []byte) (*model.Document, error) {
	if _, ok := p.Registry.Get(docType); !ok {
		return nil, fmt.Errorf("未知的单据类型 %q,可用类型: %v", docType, p.Registry.Names())
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
		ID:        id,
		DocType:   docType,
		Filename:  filename,
		ObjectKey: key,
		Status:    model.StatusUploaded,
	}
	if err := p.DB.CreateDocument(ctx, doc); err != nil {
		return nil, fmt.Errorf("登记文档失败: %w", err)
	}
	return doc, nil
}

// ExtractFields 实现 extract_fields:取正文(图片走 OCR 服务,文档走解析服务),
// 再调用 AI 服务提取字段。
func (p *Pipeline) ExtractFields(ctx context.Context, id string) (*model.Document, error) {
	doc, err := p.DB.GetDocument(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("文档 %s 不存在", id)
	}
	dt, ok := p.Registry.Get(doc.DocType)
	if !ok {
		return nil, fmt.Errorf("文档的单据类型 %q 已不在配置中", doc.DocType)
	}

	if doc.Text == "" {
		data, err := p.Objects.Get(ctx, doc.ObjectKey)
		if err != nil {
			return p.fail(ctx, doc, fmt.Errorf("读取归档文件失败: %w", err))
		}
		text, err := p.toText(ctx, doc.Filename, data)
		if err != nil {
			return p.fail(ctx, doc, err)
		}
		doc.Text = text
	}

	specs := make([]api.FieldSpecView, 0, len(dt.Fields))
	for _, f := range dt.Fields {
		specs = append(specs, api.FieldSpecView{
			Name:        f.Name,
			Label:       f.Label,
			Description: f.Description,
			Aliases:     f.Aliases,
		})
	}
	resp, err := p.AI.Extract(ctx, api.ExtractRequest{Text: doc.Text, DocType: dt.Name, Fields: specs})
	if err != nil {
		return p.fail(ctx, doc, fmt.Errorf("字段提取失败: %w", err))
	}

	doc.Fields = resp.Fields
	doc.Status = model.StatusExtracted
	doc.Error = ""
	doc.Issues = nil
	if err := p.DB.UpdateDocument(ctx, doc); err != nil {
		return nil, err
	}
	return doc, nil
}

// toText 按文件类型路由:图片 → OCR 服务,其余 → 文档解析服务。
func (p *Pipeline) toText(ctx context.Context, filename string, data []byte) (string, error) {
	ext := strings.ToLower(filepath.Ext(filename))
	if parser.ImageExts[ext] {
		return p.OCR.Recognize(ctx, filename, data)
	}
	return p.Parser.Parse(ctx, filename, data)
}

// Validate 实现 validate_document:规则校验 + 置信度阈值,决定是否转人工。
func (p *Pipeline) Validate(ctx context.Context, id string) (*model.Document, error) {
	doc, err := p.DB.GetDocument(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("文档 %s 不存在", id)
	}
	if len(doc.Fields) == 0 && doc.Status == model.StatusUploaded {
		return nil, fmt.Errorf("文档尚未提取字段,请先调用 extract_fields")
	}
	dt, ok := p.Registry.Get(doc.DocType)
	if !ok {
		return nil, fmt.Errorf("文档的单据类型 %q 已不在配置中", doc.DocType)
	}

	status, issues := validate.Run(doc.Fields, dt, p.MinConfidence)
	doc.Status = status
	doc.Issues = issues
	if err := p.DB.UpdateDocument(ctx, doc); err != nil {
		return nil, err
	}
	return doc, nil
}

// Save 实现 save_database:确认入库。
// fields 非空时以其为准(人工审核修正后的值),并要求先通过校验或显式覆盖。
func (p *Pipeline) Save(ctx context.Context, id string, fields []model.Field, force bool) (*model.Document, error) {
	doc, err := p.DB.GetDocument(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("文档 %s 不存在", id)
	}
	if len(fields) > 0 {
		doc.Fields = fields // 人工审核修正
		doc.Issues = nil
	}
	switch doc.Status {
	case model.StatusValidated:
	case model.StatusNeedsReview:
		if len(fields) == 0 && !force {
			return nil, fmt.Errorf("文档待人工审核(%d 个问题),请传入修正后的 fields,或 force=true 强制入库", len(doc.Issues))
		}
	case model.StatusSaved:
		return doc, nil // 幂等
	default:
		return nil, fmt.Errorf("当前状态 %q 不能入库,请先完成提取与校验", doc.Status)
	}

	doc.Status = model.StatusSaved
	doc.Error = ""
	if err := p.DB.UpdateDocument(ctx, doc); err != nil {
		return nil, err
	}
	return doc, nil
}

// Query 实现 query_document。
func (p *Pipeline) Query(ctx context.Context, f store.QueryFilter) ([]*model.Document, error) {
	return p.DB.QueryDocuments(ctx, f)
}

// fail 把失败状态落库后返回原始错误,保证任务可追溯。
func (p *Pipeline) fail(ctx context.Context, doc *model.Document, cause error) (*model.Document, error) {
	doc.Status = model.StatusFailed
	doc.Error = cause.Error()
	if err := p.DB.UpdateDocument(ctx, doc); err != nil {
		return nil, fmt.Errorf("%w(且状态落库失败: %v)", cause, err)
	}
	return nil, cause
}
