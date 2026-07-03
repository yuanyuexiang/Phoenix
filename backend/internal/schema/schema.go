// Package schema 实现可配置的单据类型字段 schema。
//
// 每种单据类型(合同、发票……)对应 configs/doctypes/ 下的一个 YAML 文件,
// 声明要提取哪些字段以及各字段的校验规则。新增单据类型只需加一个 YAML,
// 无需改代码——这是"字段清单待客户确认"落地为架构特性的地方。
package schema

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"

	"gopkg.in/yaml.v3"
)

// Rule 是字段的校验规则。零值表示不校验。
type Rule struct {
	Required bool     `yaml:"required" json:"required"`
	Pattern  string   `yaml:"pattern,omitempty" json:"pattern,omitempty"` // 正则,匹配整个值
	Enum     []string `yaml:"enum,omitempty" json:"enum,omitempty"`
}

// FieldSpec 声明一个待提取字段。
type FieldSpec struct {
	Name        string   `yaml:"name" json:"name"`   // 字段英文名,入库用
	Label       string   `yaml:"label" json:"label"` // 中文标签,提取与展示用
	Description string   `yaml:"description,omitempty" json:"description,omitempty"`
	Aliases     []string `yaml:"aliases,omitempty" json:"aliases,omitempty"` // 文档中可能出现的其他叫法
	Rule        Rule     `yaml:"rule,omitempty" json:"rule"`
}

// DocType 是一种单据类型的完整定义。
type DocType struct {
	Name        string      `yaml:"name" json:"name"`
	Title       string      `yaml:"title" json:"title"`
	Description string      `yaml:"description,omitempty" json:"description,omitempty"`
	Fields      []FieldSpec `yaml:"fields" json:"fields"`
}

// Registry 持有全部已加载的单据类型。
type Registry struct {
	byName map[string]*DocType
}

// Load 读取目录下所有 *.yaml 单据类型定义并做基本合法性检查。
func Load(dir string) (*Registry, error) {
	files, err := filepath.Glob(filepath.Join(dir, "*.yaml"))
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("schema: %s 下没有单据类型定义(*.yaml)", dir)
	}
	r := &Registry{byName: map[string]*DocType{}}
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			return nil, err
		}
		var dt DocType
		if err := yaml.Unmarshal(data, &dt); err != nil {
			return nil, fmt.Errorf("schema: 解析 %s 失败: %w", f, err)
		}
		if err := dt.check(); err != nil {
			return nil, fmt.Errorf("schema: %s: %w", f, err)
		}
		if _, dup := r.byName[dt.Name]; dup {
			return nil, fmt.Errorf("schema: 单据类型 %q 重复定义(%s)", dt.Name, f)
		}
		r.byName[dt.Name] = &dt
	}
	return r, nil
}

func (dt *DocType) check() error {
	if dt.Name == "" {
		return fmt.Errorf("缺少 name")
	}
	if len(dt.Fields) == 0 {
		return fmt.Errorf("单据类型 %q 没有定义任何字段", dt.Name)
	}
	seen := map[string]bool{}
	for _, f := range dt.Fields {
		if f.Name == "" || f.Label == "" {
			return fmt.Errorf("单据类型 %q 存在缺少 name/label 的字段", dt.Name)
		}
		if seen[f.Name] {
			return fmt.Errorf("单据类型 %q 字段 %q 重复", dt.Name, f.Name)
		}
		seen[f.Name] = true
		if f.Rule.Pattern != "" {
			if _, err := regexp.Compile(f.Rule.Pattern); err != nil {
				return fmt.Errorf("字段 %q 的 pattern 非法: %w", f.Name, err)
			}
		}
	}
	return nil
}

// Get 按名称取单据类型。
func (r *Registry) Get(name string) (*DocType, bool) {
	dt, ok := r.byName[name]
	return dt, ok
}

// Names 返回全部单据类型名,字典序。
func (r *Registry) Names() []string {
	names := make([]string, 0, len(r.byName))
	for n := range r.byName {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}
