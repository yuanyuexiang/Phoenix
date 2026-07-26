package ontology

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/yuanyuexiang/phoenix/internal/api"
	"github.com/yuanyuexiang/phoenix/internal/model"
	"github.com/yuanyuexiang/phoenix/internal/store"
	"github.com/yuanyuexiang/phoenix/internal/validate"
)

// Materializer 把已入库文档物化为对象与链接(设计方案 §5)。
type Materializer struct {
	Reg *Registry
	DB  *store.DB
}

var multiSep = regexp.MustCompile(`[;,、，；\s]+`)

// Materialize 物化一份 saved 文档:先撤销其旧链接(重放语义),再按命中的
// 本体定义生成/更新对象、链接与证据。返回摘要(对象引用+警告)。
// 错误不向上抛断业务 —— 汇入 Warnings,由调用方记录告警。
func (m *Materializer) Materialize(ctx context.Context, doc *model.Document) *api.OntologySummary {
	sum := &api.OntologySummary{}
	types := m.Reg.BySource(doc.DocType)
	if len(types) == 0 {
		return sum
	}
	fieldVal := map[string]string{}
	for _, f := range doc.Fields {
		fieldVal[f.Name] = strings.TrimSpace(f.Value)
	}

	if err := m.DB.DeleteDocumentLinks(ctx, doc.ID); err != nil {
		sum.Warnings = append(sum.Warnings, "撤销旧链接失败: "+err.Error())
		return sum
	}

	seen := map[string]bool{} // 摘要内按对象 ID 去重
	addRef := func(ot *ObjectType, id, display string, isNew bool) {
		if seen[id] {
			return
		}
		seen[id] = true
		sum.Objects = append(sum.Objects, api.OntologyObjectRef{
			Type: ot.Name, Title: ot.Title, ID: id, Display: display, IsNew: isNew,
		})
	}

	for _, ot := range types {
		for _, src := range ot.Sources {
			if src.DocType != doc.DocType {
				continue
			}
			m.materializeSource(ctx, ot, src, doc, fieldVal, sum, addRef)
		}
	}
	return sum
}

func (m *Materializer) materializeSource(ctx context.Context, ot *ObjectType, src Source,
	doc *model.Document, fieldVal map[string]string, sum *api.OntologySummary,
	addRef func(*ObjectType, string, string, bool)) {

	props := map[string]any{}
	for docField, propName := range src.Map {
		v := fieldVal[docField]
		if v == "" {
			continue
		}
		props[propName] = coerce(ot.propByName[propName].Type, v)
	}
	display := fmt.Sprint(props[ot.DisplayProperty])
	if props[ot.DisplayProperty] == nil {
		display = ""
	}
	id, isNew, ok := m.upsert(ctx, ot, display, props, doc.ID, sum)
	if !ok {
		return
	}
	addRef(ot, id, display, isNew)

	for docField, lm := range src.LinkMap {
		raw := fieldVal[docField]
		if raw == "" {
			continue
		}
		vals := []string{raw}
		if lm.Multi {
			vals = multiSep.Split(raw, -1)
		}
		target, _ := m.Reg.Get(lm.To)
		linkDef := ot.linkByName[lm.Link]
		for _, v := range vals {
			v = strings.TrimSpace(v)
			if v == "" {
				continue
			}
			tprops := map[string]any{lm.Property: coerce(target.propByName[lm.Property].Type, v)}
			tid, tNew, ok := m.upsert(ctx, target, v, tprops, doc.ID, sum)
			if !ok {
				continue
			}
			addRef(target, tid, v, tNew)
			if err := m.DB.InsertLink(ctx, lm.Link, id, tid, doc.ID); err != nil {
				sum.Warnings = append(sum.Warnings, fmt.Sprintf("建立关系 %s→%s 失败: %v", lm.Link, v, err))
				continue
			}
			if linkDef.WarnDuplicate {
				if n, err := m.DB.OtherLinkSources(ctx, lm.Link, tid, doc.ID); err == nil && n > 0 {
					sum.Warnings = append(sum.Warnings, fmt.Sprintf(
						"%s「%s」已被另外 %d 份单据以「%s」关系引用,疑似重复(如重复报销),请人工核实",
						target.Title, v, n, linkDef.Label))
				}
			}
		}
	}
}

// upsert 解析归一键并写入对象与证据;键冲突(并发)重试一次。
func (m *Materializer) upsert(ctx context.Context, ot *ObjectType, display string,
	props map[string]any, docID string, sum *api.OntologySummary) (string, bool, bool) {

	hashes := ot.KeyHashes(props)
	if len(hashes) == 0 {
		sum.Warnings = append(sum.Warnings, fmt.Sprintf("%s 缺少可用归一键,未物化", ot.Title))
		return "", false, false
	}
	id, isNew, err := m.DB.UpsertObjectByKeys(ctx, ot.Name, display, props, hashes)
	if errors.Is(err, store.ErrKeyConflict) { // 并发创建撞键 → 此时键已存在,重试即命中
		id, isNew, err = m.DB.UpsertObjectByKeys(ctx, ot.Name, display, props, hashes)
	}
	if err != nil {
		sum.Warnings = append(sum.Warnings, fmt.Sprintf("%s「%s」物化失败: %v", ot.Title, display, err))
		return "", false, false
	}
	if err := m.DB.InsertObjectEvidence(ctx, id, docID); err != nil {
		sum.Warnings = append(sum.Warnings, fmt.Sprintf("%s「%s」记录证据失败: %v", ot.Title, display, err))
	}
	return id, isNew, true
}

// coerce 按属性类型转换值:number → JSON 数值,date → ISO 字符串。
// 值在 P0 已随文档字段归一化,这里兜底再处理一次(链接目标值不经过 doctype 归一)。
func coerce(typ, v string) any {
	switch typ {
	case "number":
		if n, err := validate.ParseNumber(v); err == nil {
			return n
		}
	case "date":
		if d, err := validate.ParseDate(v); err == nil {
			return d
		}
	}
	return v
}

// Rebuild 全量重建对象层:清空后按全部 saved 文档重放(本体 YAML 大改后使用)。
func (m *Materializer) Rebuild(ctx context.Context, listSaved func(context.Context) ([]*model.Document, error)) (int, []string, error) {
	if err := m.DB.TruncateOntology(ctx); err != nil {
		return 0, nil, err
	}
	docs, err := listSaved(ctx)
	if err != nil {
		return 0, nil, err
	}
	var warnings []string
	for _, d := range docs {
		s := m.Materialize(ctx, d)
		warnings = append(warnings, s.Warnings...)
	}
	return len(docs), warnings, nil
}
