package handlers

import (
	"encoding/json"
	"sort"
	"testing"
)

// ---------------------------------------------------------------------------
// 本文件是 merge.go（文档三方合并引擎）的表驱动单测。
// mergeDocument 是纯函数（入参出参都是已解析的 JSON 文档），因此可以直接单测，
// 无需起 gin / 读写用户数据文件。此前该逻辑只有外部 Python 黑盒验证
// （tools/archive/research_tests/merge_verify.py、tools/ikuai_deploy/online_merge_verify.py），
// 现在补上包内回归，改动合并规则时能被 go test 立刻拦住。
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// 测试辅助
// ---------------------------------------------------------------------------

// parseDoc 把 JSON 字面量解析为合并引擎使用的文档形态（map[string]interface{}）。
func parseDoc(t *testing.T, raw string) map[string]interface{} {
	t.Helper()
	if raw == "" {
		return map[string]interface{}{}
	}
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		t.Fatalf("解析文档 JSON 失败: %v\n%s", err, raw)
	}
	return m
}

// conflictSig 把冲突清单归一为可比较的字符串。
// 因为 mergeIDList / mergeMapFields 遍历 map 的顺序在 Go 中是不确定的，
// 冲突的产生顺序也不确定，所以断言前必须先排序。
func conflictSig(cs []Conflict) []string {
	out := make([]string, 0, len(cs))
	for _, c := range cs {
		key := c.Scope
		if c.ID != "" {
			key += "[" + c.ID + "]"
		}
		if c.Field != "" {
			key += "." + c.Field
		}
		out = append(out, key+"|"+c.Resolution+"|"+c.Note)
	}
	sort.Strings(out)
	return out
}

func checkConflicts(t *testing.T, got []Conflict, want []string) {
	t.Helper()
	g := conflictSig(got)
	w := append([]string(nil), want...)
	sort.Strings(w)
	if len(g) != len(w) {
		t.Fatalf("冲突条数不符:\n got = %v\nwant = %v", g, w)
	}
	for i := range g {
		if g[i] != w[i] {
			t.Fatalf("冲突内容不符:\n got = %v\nwant = %v", g, w)
		}
	}
}

// checkDoc 用 jsonEqual 比较合并结果与期望 JSON（map 键序无关，数组顺序敏感）。
func checkDoc(t *testing.T, got map[string]interface{}, wantJSON string) {
	t.Helper()
	want := parseDoc(t, wantJSON)
	if !jsonEqual(got, want) {
		gb, _ := json.Marshal(got)
		wb, _ := json.Marshal(want)
		t.Fatalf("合并结果不符\n got = %s\nwant = %s", gb, wb)
	}
}

func runMerge(t *testing.T, baseJSON, localJSON, serverJSON string) (map[string]interface{}, []Conflict) {
	t.Helper()
	return mergeDocument(parseDoc(t, baseJSON), parseDoc(t, localJSON), parseDoc(t, serverJSON))
}

// ---------------------------------------------------------------------------
// jsonEqual
// ---------------------------------------------------------------------------

func TestJSONEqual(t *testing.T) {
	cases := []struct {
		name string
		a, b interface{}
		want bool
	}{
		{"map 键序无关", parseDoc(t, `{"a":1,"b":2}`), parseDoc(t, `{"b":2,"a":1}`), true},
		{"数组顺序敏感", parseDoc(t, `{"l":[1,2]}`), parseDoc(t, `{"l":[2,1]}`), false},
		{"数字类型不同（float64 vs int）", float64(1), 1, true},
		{"nil 相等", nil, nil, true},
		{"nil 与非 nil", nil, 0, false},
		{"不可序列化值 → false", make(chan int), 1, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := jsonEqual(c.a, c.b); got != c.want {
				t.Fatalf("jsonEqual(%#v, %#v) = %v, want %v", c.a, c.b, got, c.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// classify：存在性八态
// ---------------------------------------------------------------------------

func TestClassify(t *testing.T) {
	cases := []struct {
		name          string
		bOK, lOK, sOK bool
		want          presence
	}{
		{"三方均有", true, true, true, presAll},
		{"服务端删除（base+local 有）", true, true, false, presServerDeleted},
		{"本地删除（base+server 有）", true, false, true, presLocalDeleted},
		{"双方都删除", true, false, false, presBothDeleted},
		{"双方都新增", false, true, true, presBothAdded},
		{"仅本地新增", false, true, false, presLocalAdded},
		{"仅服务端新增", false, false, true, presServerAdded},
		{"三方皆无（不可达，走 default）", false, false, false, presServerAdded},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := classify(c.bOK, c.lOK, c.sOK); got != c.want {
				t.Fatalf("classify(%v,%v,%v) = %v, want %v", c.bOK, c.lOK, c.sOK, got, c.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// mergeScalarValue：不可细分值的三方规则
// ---------------------------------------------------------------------------

func TestMergeScalarValue(t *testing.T) {
	type tc struct {
		name          string
		b, l, s       interface{}
		bOK, lOK, sOK bool
		wantVal       interface{}
		wantKeep      bool
		wantNotes     []string
	}
	cases := []tc{
		{
			name: "三方同值 → 原样保留",
			b:    "v", l: "v", s: "v", bOK: true, lOK: true, sOK: true,
			wantVal: "v", wantKeep: true,
		},
		{
			name: "本地未改 / 服务端改 → 用服务端",
			b:    "v1", l: "v1", s: "v2", bOK: true, lOK: true, sOK: true,
			wantVal: "v2", wantKeep: true,
		},
		{
			name: "服务端未改 / 本地改 → 用本地",
			b:    "v1", l: "v2", s: "v1", bOK: true, lOK: true, sOK: true,
			wantVal: "v2", wantKeep: true,
		},
		{
			name: "双方改且同值 → 采用（不冲突）",
			b:    "v1", l: "v2", s: "v2", bOK: true, lOK: true, sOK: true,
			wantVal: "v2", wantKeep: true,
		},
		{
			name: "双方改且不同 → 服务端权威 + 冲突",
			b:    "v1", l: "v2", s: "v3", bOK: true, lOK: true, sOK: true,
			wantVal: "v3", wantKeep: true, wantNotes: []string{"both_modified"},
		},
		{
			name: "服务端删除 / 本地未改 → 跟随删除",
			b:    "v1", l: "v1", s: nil, bOK: true, lOK: true, sOK: false,
			wantVal: nil, wantKeep: false,
		},
		{
			name: "服务端删除 / 本地改过 → 保留本地 + 冲突",
			b:    "v1", l: "v2", s: nil, bOK: true, lOK: true, sOK: false,
			wantVal: "v2", wantKeep: true, wantNotes: []string{"server_deleted_local_modified"},
		},
		{
			name: "本地删除 / 服务端未改 → 跟随删除",
			b:    "v1", l: nil, s: "v1", bOK: true, lOK: false, sOK: true,
			wantVal: nil, wantKeep: false,
		},
		{
			name: "本地删除 / 服务端改过 → 保留服务端 + 冲突",
			b:    "v1", l: nil, s: "v2", bOK: true, lOK: false, sOK: true,
			wantVal: "v2", wantKeep: true, wantNotes: []string{"local_deleted_server_modified"},
		},
		{
			name: "双方都删除 → 不保留",
			b:    "v1", l: nil, s: nil, bOK: true, lOK: false, sOK: false,
			wantVal: nil, wantKeep: false,
		},
		{
			name: "双方都新增且同值 → 采用（不冲突）",
			b:    nil, l: "v1", s: "v1", bOK: false, lOK: true, sOK: true,
			wantVal: "v1", wantKeep: true,
		},
		{
			name: "双方都新增但不同 → 服务端 + 冲突",
			b:    nil, l: "v1", s: "v2", bOK: false, lOK: true, sOK: true,
			wantVal: "v2", wantKeep: true, wantNotes: []string{"both_added"},
		},
		{
			name: "仅本地新增 → 采用本地",
			b:    nil, l: "v1", s: nil, bOK: false, lOK: true, sOK: false,
			wantVal: "v1", wantKeep: true,
		},
		{
			name: "仅服务端新增 → 采用服务端",
			b:    nil, l: nil, s: "v1", bOK: false, lOK: false, sOK: true,
			wantVal: "v1", wantKeep: true,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var conflicts []Conflict
			got, keep := mergeScalarValue(c.b, c.l, c.s, c.bOK, c.lOK, c.sOK, "demo", "id1", "field1", &conflicts)
			if keep != c.wantKeep {
				t.Fatalf("keep = %v, want %v", keep, c.wantKeep)
			}
			if !jsonEqual(got, c.wantVal) {
				t.Fatalf("value = %#v, want %#v", got, c.wantVal)
			}
			var notes []string
			for _, cf := range conflicts {
				notes = append(notes, cf.Note)
			}
			sort.Strings(notes)
			want := append([]string(nil), c.wantNotes...)
			sort.Strings(want)
			if len(notes) != len(want) {
				t.Fatalf("冲突说明不符: got %v, want %v", notes, want)
			}
			for i := range notes {
				if notes[i] != want[i] {
					t.Fatalf("冲突说明不符: got %v, want %v", notes, want)
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// mergeMapFields：对象逐键合并
// ---------------------------------------------------------------------------

func TestMergeMapFieldsPerKey(t *testing.T) {
	// 双方各改不同键 → 两者都保留；双方都没了的键 → 丢弃
	base := parseDoc(t, `{"theme":"dark","size":12,"dropped":"x"}`)
	local := parseDoc(t, `{"theme":"light","size":12,"added":"new"}`)
	server := parseDoc(t, `{"theme":"dark","size":20}`)

	var conflicts []Conflict
	got := mergeMapFields(base, local, server, "appConfig", "", &conflicts)

	checkDoc(t, got, `{"theme":"light","size":20,"added":"new"}`)
	checkConflicts(t, conflicts, nil)
}

func TestMergeMapFieldsSameKeyConflict(t *testing.T) {
	base := parseDoc(t, `{"a":1}`)
	local := parseDoc(t, `{"a":2}`)
	server := parseDoc(t, `{"a":3}`)

	var conflicts []Conflict
	got := mergeMapFields(base, local, server, "appConfig", "", &conflicts)

	checkDoc(t, got, `{"a":3}`)
	checkConflicts(t, conflicts, []string{"appConfig.a|server|both_modified"})
}

func TestMergeMapFieldsLocalDeletePropagates(t *testing.T) {
	base := parseDoc(t, `{"a":1,"b":2}`)
	local := parseDoc(t, `{"a":1}`)
	server := parseDoc(t, `{"a":1,"b":2}`)

	var conflicts []Conflict
	got := mergeMapFields(base, local, server, "appConfig", "", &conflicts)

	checkDoc(t, got, `{"a":1}`)
	checkConflicts(t, conflicts, nil)
}

// ---------------------------------------------------------------------------
// splitIDList / filterOrder / equalStrings
// ---------------------------------------------------------------------------

func TestSplitIDList(t *testing.T) {
	list := []interface{}{
		parseDoc(t, `{"id":"a","v":1}`),
		parseDoc(t, `{"id":"a","v":2}`), // 重复 id → 后值覆盖，顺序只记一次
		"not-an-object",                 // 非对象 → 跳过
		parseDoc(t, `{"v":3}`),          // 缺 id → 跳过
		parseDoc(t, `{"id":"","v":4}`),  // 空 id → 跳过
		parseDoc(t, `{"id":"b","v":5}`),
	}

	m, order := splitIDList(list)

	if len(m) != 2 {
		t.Fatalf("映射大小 = %d, want 2 (%v)", len(m), m)
	}
	if !equalStrings(order, []string{"a", "b"}) {
		t.Fatalf("顺序 = %v, want [a b]", order)
	}
	if got := m["a"]; !jsonEqual(got, parseDoc(t, `{"id":"a","v":2}`)) {
		t.Fatalf("重复 id 应保留最后一个值, got %#v", got)
	}
}

func TestFilterOrder(t *testing.T) {
	// 被删除的 id 必须从顺序里剔除，否则会留下空洞
	got := filterOrder([]string{"a", "b", "c"}, map[string]interface{}{"a": 1, "c": 3})
	if !equalStrings(got, []string{"a", "c"}) {
		t.Fatalf("filterOrder = %v, want [a c]", got)
	}
	if got := filterOrder(nil, map[string]interface{}{"a": 1}); len(got) != 0 {
		t.Fatalf("空顺序应返回空切片, got %v", got)
	}
}

func TestEqualStrings(t *testing.T) {
	cases := []struct {
		name string
		a, b []string
		want bool
	}{
		{"同内容同序", []string{"a", "b"}, []string{"a", "b"}, true},
		{"同内容异序", []string{"a", "b"}, []string{"b", "a"}, false},
		{"长度不同", []string{"a"}, []string{"a", "b"}, false},
		{"两个空", nil, []string{}, true},
		{"一空一非空", nil, []string{"a"}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := equalStrings(c.a, c.b); got != c.want {
				t.Fatalf("equalStrings(%v, %v) = %v, want %v", c.a, c.b, got, c.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// mergeDocument：appConfig
// ---------------------------------------------------------------------------

func TestMergeDocumentAppConfigPerKey(t *testing.T) {
	out, conflicts := runMerge(t,
		`{"appConfig":{"theme":"dark","size":12,"autoSave":true},"username":"u"}`,
		`{"appConfig":{"theme":"light","size":12,"autoSave":true},"username":"u"}`,
		`{"appConfig":{"theme":"dark","size":20,"autoSave":true},"username":"u"}`,
	)

	checkDoc(t, out, `{"appConfig":{"theme":"light","size":20,"autoSave":true},"username":"u"}`)
	checkConflicts(t, conflicts, nil)
}

func TestMergeDocumentTopLevelNonMergeableCarriedFromLocal(t *testing.T) {
	// 不在 mergeableKeys 里的顶层字段沿用「以本地提交为准」，不参与合并
	out, conflicts := runMerge(t,
		`{"customField":"base","appConfig":{}}`,
		`{"customField":"local","appConfig":{}}`,
		`{"customField":"server","appConfig":{}}`,
	)

	if out["customField"] != "local" {
		t.Fatalf("非合并顶层字段应取本地值, got %#v", out["customField"])
	}
	checkConflicts(t, conflicts, nil)
}

// ---------------------------------------------------------------------------
// mergeDocument：groups / widgets 按 id
// ---------------------------------------------------------------------------

func TestMergeDocumentGroupLocalAddKept(t *testing.T) {
	out, conflicts := runMerge(t,
		`{"groups":[{"id":"g1","title":"A"}]}`,
		`{"groups":[{"id":"g1","title":"A"},{"id":"new1","title":"N"}]}`,
		`{"groups":[{"id":"g1","title":"A"}]}`,
	)

	checkDoc(t, out, `{"groups":[{"id":"g1","title":"A"},{"id":"new1","title":"N"}]}`)
	checkConflicts(t, conflicts, nil)
}

func TestMergeDocumentServerDeleteFollowedWhenLocalUntouched(t *testing.T) {
	out, conflicts := runMerge(t,
		`{"groups":[{"id":"g1","title":"A"},{"id":"g2","title":"B"}]}`,
		`{"groups":[{"id":"g1","title":"A"},{"id":"g2","title":"B"}]}`,
		`{"groups":[{"id":"g1","title":"A"}]}`,
	)

	checkDoc(t, out, `{"groups":[{"id":"g1","title":"A"}]}`)
	checkConflicts(t, conflicts, nil)
}

func TestMergeDocumentServerDeleteKeptWhenLocalModified(t *testing.T) {
	out, conflicts := runMerge(t,
		`{"groups":[{"id":"g1","title":"A"},{"id":"g2","title":"B"}]}`,
		`{"groups":[{"id":"g1","title":"A"},{"id":"g2","title":"B2"}]}`,
		`{"groups":[{"id":"g1","title":"A"}]}`,
	)

	checkDoc(t, out, `{"groups":[{"id":"g1","title":"A"},{"id":"g2","title":"B2"}]}`)
	checkConflicts(t, conflicts, []string{"groups[g2]|local|server_deleted_local_modified"})
}

func TestMergeDocumentLocalDeleteKeptWhenServerModified(t *testing.T) {
	out, conflicts := runMerge(t,
		`{"groups":[{"id":"g1","title":"A"},{"id":"g2","title":"B"}]}`,
		`{"groups":[{"id":"g1","title":"A"}]}`,
		`{"groups":[{"id":"g1","title":"A"},{"id":"g2","title":"B2"}]}`,
	)

	checkDoc(t, out, `{"groups":[{"id":"g1","title":"A"},{"id":"g2","title":"B2"}]}`)
	checkConflicts(t, conflicts, []string{"groups[g2]|server|local_deleted_server_modified"})
}

func TestMergeDocumentBothDeleteDrops(t *testing.T) {
	out, conflicts := runMerge(t,
		`{"groups":[{"id":"g1","title":"A"},{"id":"g2","title":"B"}]}`,
		`{"groups":[{"id":"g1","title":"A"}]}`,
		`{"groups":[{"id":"g1","title":"A"}]}`,
	)

	checkDoc(t, out, `{"groups":[{"id":"g1","title":"A"}]}`)
	checkConflicts(t, conflicts, nil)
}

func TestMergeDocumentBothModifiedSameEntryMergesFieldsShallowly(t *testing.T) {
	// 双方改了同一项的**不同字段** → 逐字段合并，两边改动都保留，且不报冲突
	out, conflicts := runMerge(t,
		`{"widgets":[{"id":"w1","title":"标题","icon":"icon-a"}]}`,
		`{"widgets":[{"id":"w1","title":"改后标题","icon":"icon-a"}]}`,
		`{"widgets":[{"id":"w1","title":"标题","icon":"icon-b"}]}`,
	)

	checkDoc(t, out, `{"widgets":[{"id":"w1","title":"改后标题","icon":"icon-b"}]}`)
	checkConflicts(t, conflicts, nil)
}

func TestMergeDocumentBothModifiedSameFieldServerWins(t *testing.T) {
	// 双方改了同一项的**同一个字段** → 浅层合并时该字段走服务端权威 + 冲突
	out, conflicts := runMerge(t,
		`{"widgets":[{"id":"w1","title":"原标题"}]}`,
		`{"widgets":[{"id":"w1","title":"本地标题"}]}`,
		`{"widgets":[{"id":"w1","title":"服务端标题"}]}`,
	)

	checkDoc(t, out, `{"widgets":[{"id":"w1","title":"服务端标题"}]}`)
	checkConflicts(t, conflicts, []string{"widgets[w1].title|server|both_modified"})
}

func TestMergeDocumentBothAddSameIDDifferentContent(t *testing.T) {
	out, conflicts := runMerge(t,
		`{"groups":[{"id":"g1","title":"A"}]}`,
		`{"groups":[{"id":"g1","title":"A"},{"id":"gx","title":"本地"}]}`,
		`{"groups":[{"id":"g1","title":"A"},{"id":"gx","title":"服务端"}]}`,
	)

	checkDoc(t, out, `{"groups":[{"id":"g1","title":"A"},{"id":"gx","title":"服务端"}]}`)
	checkConflicts(t, conflicts, []string{"groups[gx]|server|both_added"})
}

func TestMergeDocumentConcurrentDifferentAddsBothKept(t *testing.T) {
	// 两端各自新增了**不同**的分组：集合内容两边都在（不丢数据），
	// 但顺序数组必然不同 → 按既有规则上报一条顺序冲突，并采用服务端顺序，
	// 其余未在顺序中出现的新增项追加在末尾（服务端优先，再本地）。
	out, conflicts := runMerge(t,
		`{"groups":[{"id":"g1","title":"A"}]}`,
		`{"groups":[{"id":"g1","title":"A"},{"id":"lx","title":"本地新增"}]}`,
		`{"groups":[{"id":"g1","title":"A"},{"id":"sx","title":"服务端新增"}]}`,
	)

	checkDoc(t, out, `{"groups":[{"id":"g1","title":"A"},{"id":"sx","title":"服务端新增"},{"id":"lx","title":"本地新增"}]}`)
	checkConflicts(t, conflicts, []string{"groups.order|server|both_reordered"})
}

// ---------------------------------------------------------------------------
// mergeDocument：顺序合并
// ---------------------------------------------------------------------------

func TestMergeDocumentLocalReorderKeptWhenServerUntouched(t *testing.T) {
	out, conflicts := runMerge(t,
		`{"groups":[{"id":"g1"},{"id":"g2"}]}`,
		`{"groups":[{"id":"g2"},{"id":"g1"}]}`,
		`{"groups":[{"id":"g1"},{"id":"g2"}]}`,
	)

	checkDoc(t, out, `{"groups":[{"id":"g2"},{"id":"g1"}]}`)
	checkConflicts(t, conflicts, nil)
}

func TestMergeDocumentServerReorderKeptWhenLocalUntouched(t *testing.T) {
	out, conflicts := runMerge(t,
		`{"groups":[{"id":"g1"},{"id":"g2"}]}`,
		`{"groups":[{"id":"g1"},{"id":"g2"}]}`,
		`{"groups":[{"id":"g2"},{"id":"g1"}]}`,
	)

	checkDoc(t, out, `{"groups":[{"id":"g2"},{"id":"g1"}]}`)
	checkConflicts(t, conflicts, nil)
}

func TestMergeDocumentBothReorderDifferentServerWins(t *testing.T) {
	out, conflicts := runMerge(t,
		`{"groups":[{"id":"g1"},{"id":"g2"},{"id":"g3"}]}`,
		`{"groups":[{"id":"g3"},{"id":"g1"},{"id":"g2"}]}`,
		`{"groups":[{"id":"g2"},{"id":"g1"},{"id":"g3"}]}`,
	)

	checkDoc(t, out, `{"groups":[{"id":"g2"},{"id":"g1"},{"id":"g3"}]}`)
	checkConflicts(t, conflicts, []string{"groups.order|server|both_reordered"})
}

func TestMergeDocumentBothReorderIdenticalNoConflict(t *testing.T) {
	// 两端重排成了**同一个**顺序 → 没有任何需要裁决的分歧，不应上报冲突
	out, conflicts := runMerge(t,
		`{"groups":[{"id":"g1"},{"id":"g2"}]}`,
		`{"groups":[{"id":"g2"},{"id":"g1"}]}`,
		`{"groups":[{"id":"g2"},{"id":"g1"}]}`,
	)

	checkDoc(t, out, `{"groups":[{"id":"g2"},{"id":"g1"}]}`)
	checkConflicts(t, conflicts, nil)
}

func TestMergeDocumentOrderCompactedAfterDelete(t *testing.T) {
	// 顺序里被删掉的 id 必须剔除；剔除后双方顺序一致 → 不报顺序冲突
	out, conflicts := runMerge(t,
		`{"groups":[{"id":"g1"},{"id":"g2"},{"id":"g3"}]}`,
		`{"groups":[{"id":"g3"},{"id":"g1"}]}`,
		`{"groups":[{"id":"g1"},{"id":"g3"}]}`,
	)

	checkDoc(t, out, `{"groups":[{"id":"g3"},{"id":"g1"}]}`)
	checkConflicts(t, conflicts, nil)
}

// ---------------------------------------------------------------------------
// mergeDocument：无基线的兼容路径
// ---------------------------------------------------------------------------

func TestMergeDocumentNoBaseUsesLocalWholesale(t *testing.T) {
	// 载荷没有携带基线（旧客户端）→ groups/widgets 整体取本地，不做逐 id 合并
	out, conflicts := runMerge(t,
		`{"appConfig":{}}`,
		`{"appConfig":{},"groups":[{"id":"local-only"}]}`,
		`{"appConfig":{},"groups":[{"id":"server-only"}]}`,
	)

	checkDoc(t, out, `{"appConfig":{},"groups":[{"id":"local-only"}]}`)
	checkConflicts(t, conflicts, nil)
}

func TestMergeDocumentNoBaseFallsBackToServerWhenLocalMissing(t *testing.T) {
	out, conflicts := runMerge(t,
		`{"appConfig":{}}`,
		`{"appConfig":{}}`,
		`{"appConfig":{},"groups":[{"id":"server-only"}]}`,
	)

	checkDoc(t, out, `{"appConfig":{},"groups":[{"id":"server-only"}]}`)
	checkConflicts(t, conflicts, nil)
}

// ---------------------------------------------------------------------------
// mergeDocument：其余标量字段（groupOrder / rssFeeds / rssCategories）
// ---------------------------------------------------------------------------

func TestMergeDocumentGroupOrderBothChangedServerWins(t *testing.T) {
	// 真实客户端（stores/save.ts 的 buildCleanBody）始终把 groupOrder 作为数组提交，
	// 所以「双方都改且不同」才是线上最常见的形态 → 服务端权威 + 冲突
	out, conflicts := runMerge(t,
		`{"groupOrder":["g1","g2"]}`,
		`{"groupOrder":["g2","g1"]}`,
		`{"groupOrder":["g1"]}`,
	)

	checkDoc(t, out, `{"groupOrder":["g1"]}`)
	checkConflicts(t, conflicts, []string{"groupOrder|server|both_modified"})
}

func TestMergeDocumentGroupOrderLocalChangeKeptWhenServerUntouched(t *testing.T) {
	out, conflicts := runMerge(t,
		`{"groupOrder":["g1","g2"]}`,
		`{"groupOrder":["g2","g1"]}`,
		`{"groupOrder":["g1","g2"]}`,
	)

	checkDoc(t, out, `{"groupOrder":["g2","g1"]}`)
	checkConflicts(t, conflicts, nil)
}

// 纯函数契约：本端未提交该键、服务端与基线一致 → 结果中该键被删除。
//
// ⚠️ 这条路径经 SaveData 不可达：入库前有一轮「用既有文件补齐缺失顶层键」的循环
// （handlers/data.go），它保证 serverDoc 里存在的键 localDoc 一定也存在（sOK ⇒ lOK），
// 因此 presLocalDeleted 只可能在单测里构造出来。保留断言是为了钉住 mergeDocument 的契约。
func TestMergeDocumentAbsentKeyDeletedWhenServerUnchanged(t *testing.T) {
	out, conflicts := runMerge(t,
		`{"groupOrder":["g1","g2"]}`,
		`{}`,
		`{"groupOrder":["g1","g2"]}`,
	)

	if _, exists := out["groupOrder"]; exists {
		t.Fatalf("本端未提交且服务端未改 → 该键应被删除, got %#v", out["groupOrder"])
	}
	checkConflicts(t, conflicts, nil)
}

func TestMergeDocumentRssFeedsScalarRules(t *testing.T) {
	out, conflicts := runMerge(t,
		`{"rssFeeds":[{"url":"a"}]}`,
		`{"rssFeeds":[{"url":"b"}]}`,
		`{"rssFeeds":[{"url":"a"}]}`,
	)

	checkDoc(t, out, `{"rssFeeds":[{"url":"b"}]}`)
	checkConflicts(t, conflicts, nil)
}

// ---------------------------------------------------------------------------
// comparableSnapshot
// ---------------------------------------------------------------------------

func TestComparableSnapshot(t *testing.T) {
	doc := parseDoc(t, `{"appConfig":{"a":1},"groups":[],"rssFeeds":[],"groupOrder":[],"rssCategories":[],"widgets":[],"username":"u","extra":1}`)
	snap := comparableSnapshot(doc)

	for _, k := range mergeableKeys {
		if _, ok := snap[k]; !ok {
			t.Fatalf("快照应包含可合并字段 %q", k)
		}
	}
	if len(snap) != len(mergeableKeys) {
		t.Fatalf("快照只应包含 mergeableKeys, got %v", snap)
	}
	if _, ok := snap["username"]; ok {
		t.Fatalf("快照不应包含非合并字段 username")
	}
	if _, ok := snap["extra"]; ok {
		t.Fatalf("快照不应包含非合并字段 extra")
	}
}
