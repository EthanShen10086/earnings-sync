package orchestrator

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// LoadFile reads a FlowDefinition from a YAML or JSON file.
func LoadFile(path string) (FlowDefinition, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return FlowDefinition{}, err
	}
	return Parse(b)
}

// Parse decodes YAML/JSON bytes into a FlowDefinition and validates it.
func Parse(raw []byte) (FlowDefinition, error) {
	var def FlowDefinition
	if err := yaml.Unmarshal(raw, &def); err != nil {
		return FlowDefinition{}, fmt.Errorf("解析 flow: %w", err)
	}
	if err := Validate(def); err != nil {
		return FlowDefinition{}, err
	}
	return def, nil
}

// Validate checks structural rules needed by the engine.
func Validate(def FlowDefinition) error {
	if len(def.Nodes) == 0 {
		return fmt.Errorf("flow 至少需要一个 node")
	}
	seen := make(map[string]struct{}, len(def.Nodes))
	for i, n := range def.Nodes {
		if strings.TrimSpace(n.ID) == "" {
			return fmt.Errorf("nodes[%d] 缺少 id", i)
		}
		if _, ok := seen[n.ID]; ok {
			return fmt.Errorf("重复的 node id: %s", n.ID)
		}
		seen[n.ID] = struct{}{}
		switch n.Type {
		case "agent", "tool", "condition":
		default:
			return fmt.Errorf("node %s: 不支持的 type %q（支持 agent|tool|condition）", n.ID, n.Type)
		}
		if n.Type == "tool" && strings.TrimSpace(n.Tool) == "" {
			return fmt.Errorf("node %s: tool 类型必须设置 tool 名", n.ID)
		}
		if n.Type == "agent" && strings.TrimSpace(n.Prompt) == "" {
			return fmt.Errorf("node %s: agent 类型必须设置 prompt", n.ID)
		}
		if n.NeedsMode != "" && n.NeedsMode != "all" && n.NeedsMode != "any" {
			return fmt.Errorf("node %s: needsMode 只能是 all 或 any", n.ID)
		}
		if n.OnFailure != "" && n.OnFailure != "stop" && n.OnFailure != "continue" {
			return fmt.Errorf("node %s: onFailure 只能是 stop 或 continue", n.ID)
		}
		if n.Retry != nil && n.Retry.MaxAttempts < 1 {
			return fmt.Errorf("node %s: retry.maxAttempts 至少为 1", n.ID)
		}
	}
	for _, n := range def.Nodes {
		for _, dep := range n.Needs {
			if _, ok := seen[dep]; !ok {
				return fmt.Errorf("node %s: needs 引用了不存在的节点 %s", n.ID, dep)
			}
		}
	}
	return nil
}
