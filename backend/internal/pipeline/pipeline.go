// Package pipeline 是工作流引擎(说明书 §7)的编排核心:
//
//	上传 → 文字识别/解析 → AI字段提取 → 规则校验 → [人工审核] → 入库 → 归档
//
// 每个阶段状态持久化在 documents.status,调用方(MCP/管理后台)可分步驱动、
// 断点续跑。文档解析与 AI 能力是独立服务,这里只做路由与编排:
// 图片 → ai 服务视觉转写;办公文档 → parser 服务;正文 → ai 服务提取。
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
	"github.com/yuanyuexiang/phoenix/internal/parser"
	"github.com/yuanyuexiang/phoenix/internal/schema"
	"github.com/yuanyuexiang/phoenix/internal/store"
	"github.com/yuanyuexiang/phoenix/internal/validate"
)

type Pipeline struct {
	DB              *store.DB
	Objects         *store.Objects
	Parser          *clients.Parser // 文档解析服务(PDF/Word/Excel)
	AI              *clients.AI     // AI 字段提取/分类/图片转写服务
	Registry        *schema.Registry
	MinConfidence   float64 // 字段置信度低于此值转人工
	ClassifyMinConf float64 // 自动分类置信度低于此值走开放提取兜底
}

// Upload 实现 upload_document:归档原始文件到 MinIO 并登记任务。
// docType 为空或 "auto" 时按待自动分类处理(提取阶段识别类型)。
// uploadedBy 是操作人展示口径(可为空),由 API 层从请求头解析(workflowapi.operatorOf)。
func (p *Pipeline) Upload(ctx context.Context, docType, filename string, data []byte, uploadedBy string) (*model.Document, error) {
	if docType == "" {
		docType = model.DocTypeAuto
	}
	if docType != model.DocTypeAuto {
		if _, ok := p.Registry.Get(docType); !ok {
			return nil, fmt.Errorf("未知的单据类型 %q,可用类型: %v(或留空/auto 自动识别)", docType, p.Registry.Names())
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

// ExtractFields 实现 extract_fields:取正文(图片走 ai 服务视觉转写,文档走解析服务),
// 类型未知时先自动分类,再调用 AI 服务提取字段:
//   - 分类命中已配置类型 → 按该类型 schema 定向提取;
//   - 分类失败 → 标记 unknown,开放提取兜底(校验阶段必转人工审核)。
func (p *Pipeline) ExtractFields(ctx context.Context, id string) (*model.Document, error) {
	doc, err := p.DB.GetDocument(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("文档 %s 不存在", id)
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
		if strings.TrimSpace(text) == "" {
			return p.fail(ctx, doc, fmt.Errorf("未能从文档中识别出任何文字,请确认上传的是单据而非普通照片"))
		}
		doc.Text = text
	}

	// 类型未知(或上次识别失败,人工重新提取时再试一次)→ 自动分类
	if doc.DocType == model.DocTypeAuto || doc.DocType == model.DocTypeUnknown {
		res, err := p.AI.Classify(ctx, api.ClassifyRequest{Text: doc.Text, Candidates: p.classifyCandidates()})
		if err != nil {
			return p.fail(ctx, doc, fmt.Errorf("自动分类失败: %w", err))
		}
		if _, ok := p.Registry.Get(res.DocType); ok && res.Confidence >= p.ClassifyMinConf {
			doc.DocType = res.DocType
		} else {
			doc.DocType = model.DocTypeUnknown
		}
	}

	var req api.ExtractRequest
	if doc.DocType == model.DocTypeUnknown {
		// 开放提取兜底:不套 schema,抽取实际存在的键值对
		req = api.ExtractRequest{Text: doc.Text, DocType: model.DocTypeUnknown}
	} else {
		dt, ok := p.Registry.Get(doc.DocType)
		if !ok {
			return nil, fmt.Errorf("文档的单据类型 %q 已不在配置中", doc.DocType)
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
		req = api.ExtractRequest{Text: doc.Text, DocType: dt.Name, Fields: specs}
	}

	resp, err := p.AI.Extract(ctx, req)
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

// classifyCandidates 把已配置的单据类型转成分类候选(标签取各字段中文名)。
func (p *Pipeline) classifyCandidates() []api.DocTypeCandidate {
	names := p.Registry.Names()
	candidates := make([]api.DocTypeCandidate, 0, len(names))
	for _, name := range names {
		dt, _ := p.Registry.Get(name)
		labels := make([]string, 0, len(dt.Fields))
		for _, f := range dt.Fields {
			labels = append(labels, f.Label)
		}
		candidates = append(candidates, api.DocTypeCandidate{
			Name:        dt.Name,
			Title:       dt.Title,
			Description: dt.Description,
			Labels:      labels,
		})
	}
	return candidates
}

// toText 按文件类型路由:图片 → ai 服务视觉转写,其余 → 文档解析服务。
func (p *Pipeline) toText(ctx context.Context, filename string, data []byte) (string, error) {
	ext := strings.ToLower(filepath.Ext(filename))
	if parser.ImageExts[ext] {
		return p.AI.Transcribe(ctx, filename, data)
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

	// 类型未识别:无 schema 可校验,直接转人工审核定类型
	if doc.DocType == model.DocTypeUnknown || doc.DocType == model.DocTypeAuto {
		doc.Status = model.StatusNeedsReview
		doc.Issues = []model.ValidationIssue{{
			Field:   "doc_type",
			Rule:    "classify",
			Message: "未能自动识别单据类型,已做开放提取,请人工确认类型与字段",
		}}
		if err := p.DB.UpdateDocument(ctx, doc); err != nil {
			return nil, err
		}
		return doc, nil
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
// reviewedBy 记为「入库确认人」(幂等分支不覆盖已有值)。
func (p *Pipeline) Save(ctx context.Context, id string, fields []model.Field, force bool, reviewedBy string) (*model.Document, error) {
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
	doc.ReviewedBy = reviewedBy
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
