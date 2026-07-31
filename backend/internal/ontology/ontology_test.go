package ontology

import "testing"

func TestNormalizeKeyValue(t *testing.T) {
	cases := map[string]string{
		"凤凰软件服务有限公司":    "凤凰软件服务有限公司",
		" 凤凰软件 服务有限公司 ": "凤凰软件服务有限公司",
		"ＡＢＣ Tech":      "abctech",
		"Ａ１　Ｂ２":         "a1b2",
	}
	for in, want := range cases {
		if got := normalizeKeyValue(in); got != want {
			t.Errorf("normalizeKeyValue(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestKeyHashes(t *testing.T) {
	ot := &ObjectType{
		Name:           "company",
		ResolutionKeys: [][]string{{"tax_no"}, {"name"}},
	}
	// 两组键都在:两个哈希,税号优先
	h2 := ot.KeyHashes(map[string]any{"tax_no": "91330000X", "name": "凤凰软件"})
	if len(h2) != 2 {
		t.Fatalf("应生成 2 组键,得到 %d", len(h2))
	}
	// 只有名称:一组
	h1 := ot.KeyHashes(map[string]any{"name": "凤凰软件"})
	if len(h1) != 1 || h1[0] != h2[1] {
		t.Fatalf("名称键应与双键场景的第二组一致")
	}
	// 名称写法不同但归一化后相同 → 哈希一致
	hn := ot.KeyHashes(map[string]any{"name": " 凤凰软件 "})
	if hn[0] != h1[0] {
		t.Fatal("归一化后相同的名称应得到相同键哈希")
	}
	// 全空:无键
	if len(ot.KeyHashes(map[string]any{})) != 0 {
		t.Fatal("无可用属性时不应生成键")
	}
}

func TestRegistryLinkTitle(t *testing.T) {
	r := &Registry{byName: map[string]*ObjectType{
		"invoice": {Name: "invoice", Links: []LinkDef{{Name: "seller", Label: "销售方", To: "company"}}},
	}}
	if got := r.LinkTitle("invoice", "seller"); got != "销售方" {
		t.Fatalf("LinkTitle = %q, want 销售方", got)
	}
	if got := r.LinkTitle("invoice", "unknown"); got != "unknown" {
		t.Fatalf("未知关系应回退内部标识,得到 %q", got)
	}
}
