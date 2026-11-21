package orchestrator

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
)

// Resolve walks a FlowValue tree and replaces {from:...} refs with concrete data.
func Resolve(value any, inputs map[string]any, nodes map[string]*NodeState) (any, error) {
	switch v := value.(type) {
	case nil:
		return nil, nil
	case map[string]any:
		if from, ok := v["from"].(string); ok {
			return resolveRef(from, v, inputs, nodes)
		}
		out := make(map[string]any, len(v))
		for k, child := range v {
			r, err := Resolve(child, inputs, nodes)
			if err != nil {
				return nil, err
			}
			out[k] = r
		}
		return out, nil
	case []any:
		out := make([]any, len(v))
		for i, child := range v {
			r, err := Resolve(child, inputs, nodes)
			if err != nil {
				return nil, err
			}
			out[i] = r
		}
		return out, nil
	default:
		return v, nil
	}
}

func resolveRef(from string, ref map[string]any, inputs map[string]any, nodes map[string]*NodeState) (any, error) {
	path, _ := ref["path"].(string)
	switch from {
	case "input":
		key, _ := ref["key"].(string)
		if key == "" {
			return nil, fmt.Errorf("input 引用缺少 key")
		}
		val, ok := inputs[key]
		if !ok {
			// Optional inputs may be omitted; required ones are checked in Engine.Run.
			return nil, nil
		}
		return dig(val, path)
	case "node":
		nodeID, _ := ref["nodeId"].(string)
		if nodeID == "" {
			return nil, fmt.Errorf("node 引用缺少 nodeId")
		}
		st, ok := nodes[nodeID]
		if !ok {
			return nil, fmt.Errorf("未知节点: %s", nodeID)
		}
		return dig(st.Output, path)
	case "nodes":
		rawIDs, _ := ref["nodeIds"].([]any)
		as, _ := ref["as"].(string)
		if as == "" {
			as = "record"
		}
		ids := make([]string, 0, len(rawIDs))
		for _, item := range rawIDs {
			if s, ok := item.(string); ok {
				ids = append(ids, s)
			}
		}
		record := make(map[string]any, len(ids))
		for _, id := range ids {
			st, ok := nodes[id]
			if !ok {
				return nil, fmt.Errorf("未知节点: %s", id)
			}
			val, err := dig(st.Output, path)
			if err != nil {
				return nil, err
			}
			record[id] = val
		}
		if as != "record" {
			return nil, fmt.Errorf("nodes 引用暂只支持 as: record")
		}
		return record, nil
	default:
		return nil, fmt.Errorf("不支持的 from: %s", from)
	}
}

func dig(val any, path string) (any, error) {
	if path == "" {
		return val, nil
	}
	cur := val
	for _, part := range strings.Split(path, ".") {
		if part == "" {
			continue
		}
		m, ok := asMap(cur)
		if !ok {
			return nil, fmt.Errorf("无法按路径 %q 继续取值", path)
		}
		next, ok := m[part]
		if !ok {
			return nil, fmt.Errorf("路径不存在: %s", path)
		}
		cur = next
	}
	return cur, nil
}

func asMap(v any) (map[string]any, bool) {
	switch m := v.(type) {
	case map[string]any:
		return m, true
	case map[string]string:
		out := make(map[string]any, len(m))
		for k, val := range m {
			out[k] = val
		}
		return out, true
	default:
		rv := reflect.ValueOf(v)
		if !rv.IsValid() || rv.Kind() != reflect.Map {
			return nil, false
		}
		out := make(map[string]any, rv.Len())
		for _, key := range rv.MapKeys() {
			out[fmt.Sprint(key.Interface())] = rv.MapIndex(key).Interface()
		}
		return out, true
	}
}

func valuesEqual(a, b any) bool {
	if a == nil && b == nil {
		return true
	}
	if sa, ok := asStringish(a); ok {
		if sb, ok := asStringish(b); ok {
			return sa == sb
		}
	}
	if ba, ok := asBool(a); ok {
		if bb, ok := asBool(b); ok {
			return ba == bb
		}
	}
	return reflect.DeepEqual(normalizeNumber(a), normalizeNumber(b))
}

func asStringish(v any) (string, bool) {
	switch t := v.(type) {
	case string:
		return t, true
	case fmt.Stringer:
		return t.String(), true
	default:
		return "", false
	}
}

func asBool(v any) (bool, bool) {
	switch t := v.(type) {
	case bool:
		return t, true
	case string:
		switch strings.ToLower(strings.TrimSpace(t)) {
		case "true", "1", "yes":
			return true, true
		case "false", "0", "no":
			return false, true
		}
	}
	return false, false
}

func normalizeNumber(v any) any {
	switch t := v.(type) {
	case int:
		return float64(t)
	case int64:
		return float64(t)
	case float32:
		return float64(t)
	case float64:
		return t
	case string:
		if n, err := strconv.ParseFloat(t, 64); err == nil {
			return n
		}
	}
	return v
}

// CollectNodeRefs walks a value tree and returns referenced node IDs.
func CollectNodeRefs(value any) []string {
	seen := map[string]struct{}{}
	var walk func(any)
	walk = func(v any) {
		switch t := v.(type) {
		case map[string]any:
			if from, _ := t["from"].(string); from == "node" {
				if id, _ := t["nodeId"].(string); id != "" {
					seen[id] = struct{}{}
				}
			}
			if from, _ := t["from"].(string); from == "nodes" {
				if ids, ok := t["nodeIds"].([]any); ok {
					for _, item := range ids {
						if s, ok := item.(string); ok {
							seen[s] = struct{}{}
						}
					}
				}
			}
			for _, child := range t {
				walk(child)
			}
		case []any:
			for _, child := range t {
				walk(child)
			}
		}
	}
	walk(value)
	out := make([]string, 0, len(seen))
	for id := range seen {
		out = append(out, id)
	}
	return out
}
