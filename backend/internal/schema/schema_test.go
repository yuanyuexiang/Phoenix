package schema

import (
	"os"
	"path/filepath"
	"testing"
)

func writeYAML(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLoadOK(t *testing.T) {
	dir := t.TempDir()
	writeYAML(t, dir, "invoice.yaml", `
name: invoice
title: 发票
fields:
  - name: invoice_no
    label: 发票号码
    rule: {required: true, pattern: '\d{8,20}'}
`)
	r, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	dt, ok := r.Get("invoice")
	if !ok || dt.Title != "发票" || len(dt.Fields) != 1 {
		t.Fatalf("加载结果不符: %+v", dt)
	}
	if names := r.Names(); len(names) != 1 || names[0] != "invoice" {
		t.Fatalf("Names() = %v", names)
	}
}

func TestLoadRejectsBadSchema(t *testing.T) {
	cases := map[string]string{
		"缺字段": "name: x\nfields: []\n",
		"重复字段": `
name: x
fields:
  - {name: a, label: 甲}
  - {name: a, label: 乙}
`,
		"非法正则": `
name: x
fields:
  - {name: a, label: 甲, rule: {pattern: '('}}
`,
	}
	for label, content := range cases {
		dir := t.TempDir()
		writeYAML(t, dir, "bad.yaml", content)
		if _, err := Load(dir); err == nil {
			t.Errorf("%s: 期望加载失败,实际成功", label)
		}
	}
}

func TestLoadEmptyDir(t *testing.T) {
	if _, err := Load(t.TempDir()); err == nil {
		t.Fatal("空目录应报错")
	}
}
