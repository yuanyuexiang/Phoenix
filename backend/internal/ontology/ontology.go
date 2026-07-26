// Package ontology 实现本体层(V1.4):对象/链接/证据的语义层压在文档流水线之上。
//
// configs/ontology/*.yaml 声明对象类型(带类型属性 + 归一键 + 关系 + 来源单据映射),
// 文档 save 之后由 Materializer 物化为对象与链接(docs/Ontology本体层设计方案.md)。
// 加载时对引用完整性 fail-fast(link 目标类型、映射字段必须存在),与 schema.Registry 同策略。
package ontology

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/yuanyuexiang/phoenix/internal/schema"
)

// Property 是对象的一个带类型属性。
type Property struct {
	Name     string `yaml:"name" json:"name"`
	Label    string `yaml:"label" json:"label"`
	Type     string `yaml:"type,omitempty" json:"type,omitempty"` // ""/string | number | date
	Required bool   `yaml:"required,omitempty" json:"required,omitempty"`
}

// LinkDef 声明本类型作为起点的一种关系。
type LinkDef struct {
	Name          string `yaml:"name" json:"name"`
	Label         string `yaml:"label" json:"label"`
	To            string `yaml:"to" json:"to"`
	WarnDuplicate bool   `yaml:"warn_duplicate,omitempty" json:"warn_duplicate,omitempty"` // 目标被其他文档同类链接引用时告警(重复报销检测)
}

// LinkMapEntry 声明来源文档字段如何生成关联对象。
type LinkMapEntry struct {
	Link     string `yaml:"link"`            // 使用的 LinkDef 名
	To       string `yaml:"to"`              // 目标对象类型
	Property string `yaml:"property"`        // 字段值写入目标的哪个属性(也是归一依据)
	Multi    bool   `yaml:"multi,omitempty"` // 值为分隔列表(; , 、),逐个建链
}

// Source 声明从哪种单据类型物化本对象。
type Source struct {
	DocType string                  `yaml:"doc_type"`
	Map     map[string]string       `yaml:"map"`      // 文档字段名 → 对象属性名
	LinkMap map[string]LinkMapEntry `yaml:"link_map"` // 文档字段名 → 关联对象
}

// ObjectType 是一种对象类型的完整定义。
type ObjectType struct {
	Name            string     `yaml:"name" json:"name"`
	Title           string     `yaml:"title" json:"title"`
	DisplayProperty string     `yaml:"display_property" json:"display_property"`
	Properties      []Property `yaml:"properties" json:"properties"`
	ResolutionKeys  [][]string `yaml:"resolution_keys" json:"resolution_keys"`
	Links           []LinkDef  `yaml:"links,omitempty" json:"links,omitempty"`
	Sources         []Source   `yaml:"sources,omitempty" json:"-"`

	propByName map[string]Property
	linkByName map[string]LinkDef
}

// Registry 持有全部已加载的对象类型。
type Registry struct {
	byName   map[string]*ObjectType
	bySource map[string][]*ObjectType // doc_type → 会被它物化的对象类型
}

// Load 读取目录下全部本体定义并做引用完整性校验(fail-fast)。
// 目录不存在或为空返回 (nil, nil) —— 本体层视为未启用,文档流水线不受影响。
func Load(dir string, docTypes *schema.Registry) (*Registry, error) {
	files, err := filepath.Glob(filepath.Join(dir, "*.yaml"))
	if err != nil || len(files) == 0 {
		return nil, nil
	}
	r := &Registry{byName: map[string]*ObjectType{}, bySource: map[string][]*ObjectType{}}
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			return nil, err
		}
		var ot ObjectType
		if err := yaml.Unmarshal(data, &ot); err != nil {
			return nil, fmt.Errorf("ontology: 解析 %s 失败: %w", f, err)
		}
		if err := ot.check(); err != nil {
			return nil, fmt.Errorf("ontology: %s: %w", f, err)
		}
		if _, dup := r.byName[ot.Name]; dup {
			return nil, fmt.Errorf("ontology: 对象类型 %q 重复定义(%s)", ot.Name, f)
		}
		r.byName[ot.Name] = &ot
	}
	if err := r.crossCheck(docTypes); err != nil {
		return nil, err
	}
	for _, ot := range r.byName {
		for _, s := range ot.Sources {
			r.bySource[s.DocType] = append(r.bySource[s.DocType], ot)
		}
	}
	return r, nil
}

func (ot *ObjectType) check() error {
	if ot.Name == "" || ot.Title == "" {
		return fmt.Errorf("缺少 name/title")
	}
	if len(ot.Properties) == 0 {
		return fmt.Errorf("对象类型 %q 没有属性", ot.Name)
	}
	ot.propByName = map[string]Property{}
	for _, p := range ot.Properties {
		if p.Name == "" {
			return fmt.Errorf("对象类型 %q 存在缺少 name 的属性", ot.Name)
		}
		switch p.Type {
		case "", "string", "number", "date":
		default:
			return fmt.Errorf("属性 %q 的 type 非法(%q)", p.Name, p.Type)
		}
		ot.propByName[p.Name] = p
	}
	if _, ok := ot.propByName[ot.DisplayProperty]; !ok {
		return fmt.Errorf("display_property %q 不是已声明属性", ot.DisplayProperty)
	}
	if len(ot.ResolutionKeys) == 0 {
		return fmt.Errorf("对象类型 %q 缺少 resolution_keys", ot.Name)
	}
	for _, group := range ot.ResolutionKeys {
		for _, k := range group {
			if _, ok := ot.propByName[k]; !ok {
				return fmt.Errorf("resolution_keys 引用未声明属性 %q", k)
			}
		}
	}
	ot.linkByName = map[string]LinkDef{}
	for _, l := range ot.Links {
		if l.Name == "" || l.To == "" {
			return fmt.Errorf("对象类型 %q 存在缺少 name/to 的 link", ot.Name)
		}
		ot.linkByName[l.Name] = l
	}
	return nil
}

// crossCheck 校验跨类型引用与来源单据字段(配置错误拒绝启动)。
func (r *Registry) crossCheck(docTypes *schema.Registry) error {
	for _, ot := range r.byName {
		for _, l := range ot.Links {
			if _, ok := r.byName[l.To]; !ok {
				return fmt.Errorf("ontology: %s.links.%s 指向不存在的对象类型 %q", ot.Name, l.Name, l.To)
			}
		}
		for _, s := range ot.Sources {
			dt, ok := docTypes.Get(s.DocType)
			if !ok {
				return fmt.Errorf("ontology: %s.sources 引用不存在的单据类型 %q", ot.Name, s.DocType)
			}
			docFields := map[string]bool{}
			for _, f := range dt.Fields {
				docFields[f.Name] = true
			}
			for docField, prop := range s.Map {
				if !docFields[docField] {
					return fmt.Errorf("ontology: %s.sources[%s].map 引用单据不存在的字段 %q", ot.Name, s.DocType, docField)
				}
				if _, ok := ot.propByName[prop]; !ok {
					return fmt.Errorf("ontology: %s.sources[%s].map 目标属性 %q 未声明", ot.Name, s.DocType, prop)
				}
			}
			for docField, lm := range s.LinkMap {
				if !docFields[docField] {
					return fmt.Errorf("ontology: %s.sources[%s].link_map 引用单据不存在的字段 %q", ot.Name, s.DocType, docField)
				}
				if _, ok := ot.linkByName[lm.Link]; !ok {
					return fmt.Errorf("ontology: %s.sources[%s].link_map 使用未声明的 link %q", ot.Name, s.DocType, lm.Link)
				}
				target, ok := r.byName[lm.To]
				if !ok {
					return fmt.Errorf("ontology: %s.sources[%s].link_map 目标类型 %q 不存在", ot.Name, s.DocType, lm.To)
				}
				if _, ok := target.propByName[lm.Property]; !ok {
					return fmt.Errorf("ontology: %s.sources[%s].link_map 目标属性 %s.%q 未声明", ot.Name, s.DocType, lm.To, lm.Property)
				}
			}
		}
	}
	return nil
}

/* ---------- Registry 访问 ---------- */

func (r *Registry) Get(name string) (*ObjectType, bool) {
	ot, ok := r.byName[name]
	return ot, ok
}

// BySource 返回会被指定单据类型物化的对象类型。
func (r *Registry) BySource(docType string) []*ObjectType {
	return r.bySource[docType]
}

// TypeInfo 是对象类型摘要(前端 tabs 用)。
type TypeInfo struct {
	Name  string `json:"name"`
	Title string `json:"title"`
}

func (r *Registry) Types() []TypeInfo {
	out := make([]TypeInfo, 0, len(r.byName))
	for _, ot := range r.byName {
		out = append(out, TypeInfo{Name: ot.Name, Title: ot.Title})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

/* ---------- 归一键 ---------- */

// KeyHashes 按 resolution_keys 优先级为属性集生成键哈希;组内任一属性为空则跳过该组。
func (ot *ObjectType) KeyHashes(props map[string]any) []string {
	var out []string
	for _, group := range ot.ResolutionKeys {
		parts := make([]string, 0, len(group))
		ok := true
		for _, k := range group {
			v := normalizeKeyValue(fmt.Sprint(props[k]))
			if props[k] == nil || v == "" {
				ok = false
				break
			}
			parts = append(parts, k+"="+v)
		}
		if ok {
			sum := sha256.Sum256([]byte(ot.Name + "\x00" + strings.Join(parts, "\x00")))
			out = append(out, hex.EncodeToString(sum[:]))
		}
	}
	return out
}

// normalizeKeyValue 归一化键值:去全部空白、全角转半角、小写 —— 字面归一,
// 不做模糊匹配(宁可漏合并进人工队列,不可错合并,见设计方案 §5.1)。
func normalizeKeyValue(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == ' ' || r == '\t' || r == '\n' || r == '　':
			continue
		case r >= 0xFF01 && r <= 0xFF5E: // 全角 ASCII → 半角
			r -= 0xFEE0
		}
		b.WriteRune(r)
	}
	return strings.ToLower(b.String())
}
