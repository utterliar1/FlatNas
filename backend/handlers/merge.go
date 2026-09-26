package handlers

import "encoding/json"

// Conflict 描述一次无法自动消解的字段级冲突。合并策略总体为「服务端权威」，
// 但会尽最大努力保留双方各自未冲突的改动；本结构用于把裁决结果回报给前端，
// 便于在必要时提示用户（非阻塞）。
type Conflict struct {
	Scope      string `json:"scope"`                // 冲突所在字段：appConfig / groups / widgets / ...
	ID         string `json:"id,omitempty"`         // 集合元素 id（groups/widgets）
	Field      string `json:"field,omitempty"`      // 对象内字段名或 "order"
	Resolution string `json:"resolution"`           // 最终采用值来源："server" | "local"
	Note       string `json:"note,omitempty"`       // 冲突类型说明
}

// mergeableKeys 是参与三方合并的顶层字段。其余顶层字段沿用既有「缺省则保留既有文件值」
// 的兼容逻辑，不进入合并。
var mergeableKeys = []string{"groups", "widgets", "groupOrder", "appConfig", "rssFeeds", "rssCategories"}

// jsonEqual 以 Go 稳定的 JSON 序列化（map 键有序）比较两个任意 JSON 值。
func jsonEqual(a, b interface{}) bool {
	ab, err1 := json.Marshal(a)
	bb, err2 := json.Marshal(b)
	if err1 != nil || err2 != nil {
		return false
	}
	return string(ab) == string(bb)
}

type presence int

const (
	presAll presence = iota
	presServerDeleted
	presLocalDeleted
	presBothDeleted
	presBothAdded
	presLocalAdded
	presServerAdded
)

// classify 依据 base/local/server 三方是否含该键，归一为一种存在性状态。
func classify(bOK, lOK, sOK bool) presence {
	switch {
	case bOK && lOK && sOK:
		return presAll
	case bOK && lOK && !sOK:
		return presServerDeleted
	case bOK && !lOK && sOK:
		return presLocalDeleted
	case bOK && !lOK && !sOK:
		return presBothDeleted
	case !bOK && lOK && sOK:
		return presBothAdded
	case !bOK && lOK && !sOK:
		return presLocalAdded
	default: // !bOK && !lOK && sOK
		return presServerAdded
	}
}

// mergeScalarValue 以三方规则合并一个不可细分的值，返回合并值及是否保留。
// 规则：本地未改→服务端；服务端未改→本地；双方同改同值→采用；双方改且不同→服务端权威+冲突；
// 一侧删除另一侧未改→跟随删除；一侧删除另一侧改过→保留改过的一侧并记冲突。
func mergeScalarValue(bv, lv, sv interface{}, bOK, lOK, sOK bool, scope, id, field string, conflicts *[]Conflict) (interface{}, bool) {
	switch classify(bOK, lOK, sOK) {
	case presAll:
		if jsonEqual(lv, sv) {
			return lv, true
		}
		if jsonEqual(lv, bv) {
			return sv, true
		}
		if jsonEqual(sv, bv) {
			return lv, true
		}
		*conflicts = append(*conflicts, Conflict{Scope: scope, ID: id, Field: field, Resolution: "server", Note: "both_modified"})
		return sv, true
	case presServerDeleted:
		if jsonEqual(lv, bv) {
			return nil, false // 本地未改 → 跟随服务端删除
		}
		*conflicts = append(*conflicts, Conflict{Scope: scope, ID: id, Field: field, Resolution: "local", Note: "server_deleted_local_modified"})
		return lv, true
	case presLocalDeleted:
		if jsonEqual(sv, bv) {
			return nil, false // 服务端未改 → 跟随本地删除
		}
		*conflicts = append(*conflicts, Conflict{Scope: scope, ID: id, Field: field, Resolution: "server", Note: "local_deleted_server_modified"})
		return sv, true
	case presBothDeleted:
		return nil, false
	case presBothAdded:
		if jsonEqual(lv, sv) {
			return lv, true
		}
		*conflicts = append(*conflicts, Conflict{Scope: scope, ID: id, Field: field, Resolution: "server", Note: "both_added"})
		return sv, true
	case presLocalAdded:
		return lv, true
	default: // presServerAdded
		return sv, true
	}
}

// mergeMapFields 对两个对象（appConfig 或集合元素对象）做逐键三方合并。
func mergeMapFields(bm, lm, sm map[string]interface{}, scope, id string, conflicts *[]Conflict) map[string]interface{} {
	keys := map[string]bool{}
	for k := range bm {
		keys[k] = true
	}
	for k := range lm {
		keys[k] = true
	}
	for k := range sm {
		keys[k] = true
	}
	out := make(map[string]interface{}, len(keys))
	for k := range keys {
		bv, bOK := bm[k]
		lv, lOK := lm[k]
		sv, sOK := sm[k]
		v, keep := mergeScalarValue(bv, lv, sv, bOK, lOK, sOK, scope, id, k, conflicts)
		if keep {
			out[k] = v
		}
	}
	return out
}

// splitIDList 把 [{id:...}] 形式的数组拆成 id→元素 映射与出现顺序。
func splitIDList(list []interface{}) (map[string]interface{}, []string) {
	m := make(map[string]interface{}, len(list))
	order := make([]string, 0, len(list))
	for _, it := range list {
		im, ok := it.(map[string]interface{})
		if !ok {
			continue
		}
		id, _ := im["id"].(string)
		if id == "" {
			continue
		}
		if _, dup := m[id]; !dup {
			order = append(order, id)
		}
		m[id] = im
	}
	return m, order
}

func filterOrder(order []string, present map[string]interface{}) []string {
	out := make([]string, 0, len(order))
	for _, id := range order {
		if _, ok := present[id]; ok {
			out = append(out, id)
		}
	}
	return out
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// mergeIDList 按 id 对集合（groups/widgets）做三方合并，并按规则重建顺序。
func mergeIDList(bm, lm, sm map[string]interface{}, border, lorder, sorder []string, scope string, conflicts *[]Conflict) []interface{} {
	ids := map[string]bool{}
	for k := range bm {
		ids[k] = true
	}
	for k := range lm {
		ids[k] = true
	}
	for k := range sm {
		ids[k] = true
	}
	out := make(map[string]interface{}, len(ids))
	for id := range ids {
		bv, bOK := bm[id]
		lv, lOK := lm[id]
		sv, sOK := sm[id]
		switch classify(bOK, lOK, sOK) {
		case presAll:
			if jsonEqual(lv, sv) {
				out[id] = lv
			} else if jsonEqual(lv, bv) {
				out[id] = sv
			} else if jsonEqual(sv, bv) {
				out[id] = lv
			} else {
				// 双方都改了同一项 → 浅层逐字段合并（可同时保留「改标题」与「加书签」）
				bmm, _ := bv.(map[string]interface{})
				lmm, _ := lv.(map[string]interface{})
				smm, _ := sv.(map[string]interface{})
				out[id] = mergeMapFields(bmm, lmm, smm, scope, id, conflicts)
			}
		case presServerDeleted:
			if !jsonEqual(lv, bv) {
				out[id] = lv
				*conflicts = append(*conflicts, Conflict{Scope: scope, ID: id, Resolution: "local", Note: "server_deleted_local_modified"})
			}
		case presLocalDeleted:
			if !jsonEqual(sv, bv) {
				out[id] = sv
				*conflicts = append(*conflicts, Conflict{Scope: scope, ID: id, Resolution: "server", Note: "local_deleted_server_modified"})
			}
		case presBothDeleted:
			// 双方都删除 → 不保留
		case presBothAdded:
			if jsonEqual(lv, sv) {
				out[id] = lv
			} else {
				out[id] = sv
				*conflicts = append(*conflicts, Conflict{Scope: scope, ID: id, Resolution: "server", Note: "both_added"})
			}
		case presLocalAdded:
			out[id] = lv
		default: // presServerAdded
			out[id] = sv
		}
	}

	// 顺序合并：本地未重排→用服务端顺序；服务端未重排→用本地顺序；双方都重排→服务端+冲突。
	bo := filterOrder(border, out)
	lo := filterOrder(lorder, out)
	so := filterOrder(sorder, out)
	var chosen []string
	if equalStrings(lo, bo) {
		chosen = so
	} else if equalStrings(so, bo) {
		chosen = lo
	} else if equalStrings(lo, so) {
		// 双方各自重排，但落到了同一个顺序 → 没有分歧需要裁决，不应记冲突
		// （否则会平白弹一次「合并冲突」提示，而两端数据其实完全一致）。
		chosen = so
	} else {
		chosen = so
		*conflicts = append(*conflicts, Conflict{Scope: scope, Field: "order", Resolution: "server", Note: "both_reordered"})
	}
	seen := make(map[string]bool, len(out))
	result := make([]interface{}, 0, len(out))
	for _, id := range chosen {
		if _, ok := out[id]; ok && !seen[id] {
			result = append(result, out[id])
			seen[id] = true
		}
	}
	// 兜底：merged 中尚未出现的新增项（服务端优先，再本地）
	for _, id := range so {
		if _, ok := out[id]; ok && !seen[id] {
			result = append(result, out[id])
			seen[id] = true
		}
	}
	for _, id := range lo {
		if _, ok := out[id]; ok && !seen[id] {
			result = append(result, out[id])
			seen[id] = true
		}
	}
	return result
}

func asMap(v interface{}) (map[string]interface{}, bool) {
	m, ok := v.(map[string]interface{})
	return m, ok
}

func asSlice(v interface{}) ([]interface{}, bool) {
	s, ok := v.([]interface{})
	return s, ok
}

// mergeDocument 对文档做逐字段三方合并，返回合并结果与冲突清单。
// base=上次同步基线，local=本端当前提交，server=服务端最新文件。
func mergeDocument(base, local, server map[string]interface{}) (map[string]interface{}, []Conflict) {
	conflicts := []Conflict{}
	out := make(map[string]interface{}, len(local))
	for k, v := range local {
		out[k] = v
	}

	// appConfig：逐键合并
	bm, bOK := asMap(base["appConfig"])
	lm, lOK := asMap(local["appConfig"])
	sm, sOK := asMap(server["appConfig"])
	if lOK {
		switch {
		case bOK && sOK:
			out["appConfig"] = mergeMapFields(bm, lm, sm, "appConfig", "", &conflicts)
		default:
			// 缺基线或缺服务端值：保守保留本地
			out["appConfig"] = lm
		}
	} else if sOK {
		out["appConfig"] = sm
	}

	// groups / widgets：按 id 合并（含新增/删除/顺序）
	for _, key := range []string{"groups", "widgets"} {
		bl, bOK2 := asSlice(base[key])
		ll, lOK2 := asSlice(local[key])
		sl, sOK2 := asSlice(server[key])
		if !lOK2 && !sOK2 {
			continue
		}
		if !bOK2 {
			// 无基线：本地优先，避免覆盖客户端刚计算的完整状态
			if lOK2 {
				out[key] = ll
			} else {
				out[key] = sl
			}
			continue
		}
		bm2, border := splitIDList(bl)
		lm2, lorder := splitIDList(ll)
		sm2, sorder := splitIDList(sl)
		out[key] = mergeIDList(bm2, lm2, sm2, border, lorder, sorder, key, &conflicts)
	}

	// 其余字段：整体标量规则
	for _, key := range []string{"groupOrder", "rssFeeds", "rssCategories"} {
		bv, bOK3 := base[key]
		lv, lOK3 := local[key]
		sv, sOK3 := server[key]
		if !bOK3 {
			if lOK3 {
				out[key] = lv
			} else if sOK3 {
				out[key] = sv
			} else {
				delete(out, key)
			}
			continue
		}
		v, keep := mergeScalarValue(bv, lv, sv, bOK3, lOK3, sOK3, key, "", "", &conflicts)
		if keep {
			out[key] = v
		} else {
			delete(out, key)
		}
	}

	return out, conflicts
}

// comparableSnapshot 抽取参与合并的字段，用于判断「合并结果是否引入了本端之外的变化」。
func comparableSnapshot(doc map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(mergeableKeys))
	for _, k := range mergeableKeys {
		if v, ok := doc[k]; ok {
			out[k] = v
		}
	}
	return out
}
