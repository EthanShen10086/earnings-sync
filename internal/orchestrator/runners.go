package orchestrator

import (
	"context"
	"fmt"
	"strings"

	"github.com/EthanShen10086/earnings-sync/internal/extract"
)

// ToolFunc is a named side-effect callable from tool nodes.
type ToolFunc func(ctx context.Context, input any) (any, error)

// ToolRegistry maps tool names to implementations.
type ToolRegistry map[string]ToolFunc

// Chatter is the model side used by agent nodes.
type Chatter interface {
	Chat(ctx context.Context, messages []extract.ChatMessage, tools []extract.ToolDef) (extract.Turn, extract.TokenUsage, error)
}

// Registry is the set of pluggable runners injected into the engine.
type Registry struct {
	Tools   ToolRegistry
	Chatter Chatter
	// AgentTools are optional tools the model may call during an agent node.
	AgentTools []extract.ToolDef
	// AgentToolHandler executes one model-requested tool call.
	AgentToolHandler func(ctx context.Context, name, arguments string) (string, error)
}

type toolRunner struct{ tools ToolRegistry }

func (r toolRunner) Execute(ctx context.Context, node Node, input any) (NodeResult, error) {
	fn, ok := r.tools[node.Tool]
	if !ok {
		return NodeResult{}, fmt.Errorf("未注册工具: %s", node.Tool)
	}
	out, err := fn(ctx, input)
	if err != nil {
		return NodeResult{}, err
	}
	return NodeResult{Output: out}, nil
}

type conditionRunner struct{}

func (conditionRunner) Execute(_ context.Context, node Node, input any) (NodeResult, error) {
	trueLabel := node.BranchTrue
	falseLabel := node.BranchFalse
	if trueLabel == "" {
		trueLabel = "true"
	}
	if falseLabel == "" {
		falseLabel = "false"
	}
	var actual any = input
	if m, ok := input.(map[string]any); ok {
		if v, exists := m["value"]; exists {
			actual = v
		} else if v, exists := m["equals"]; exists && node.Equals == nil {
			// allow inline equals in input
			actual = v
		}
	}
	expected := node.Equals
	if expected == nil {
		// truthiness: non-empty / non-false / non-zero
		ok := truthy(actual)
		branch := falseLabel
		if ok {
			branch = trueLabel
		}
		return NodeResult{Output: map[string]any{"branch": branch, "matched": ok, "value": actual}}, nil
	}
	matched := valuesEqual(actual, expected)
	branch := falseLabel
	if matched {
		branch = trueLabel
	}
	return NodeResult{Output: map[string]any{"branch": branch, "matched": matched, "value": actual}}, nil
}

func truthy(v any) bool {
	switch t := v.(type) {
	case nil:
		return false
	case bool:
		return t
	case string:
		s := strings.TrimSpace(t)
		return s != "" && !strings.EqualFold(s, "false") && s != "0"
	case int:
		return t != 0
	case int64:
		return t != 0
	case float64:
		return t != 0
	default:
		return true
	}
}

type agentRunner struct {
	chatter Chatter
	tools   []extract.ToolDef
	handle  func(ctx context.Context, name, arguments string) (string, error)
	maxTurn int
}

func (r agentRunner) Execute(ctx context.Context, node Node, input any) (NodeResult, error) {
	if r.chatter == nil {
		return NodeResult{}, fmt.Errorf("agent 节点需要模型 Chatter，请配置 DEEPSEEK_API_KEY 或注入 Mock")
	}
	maxTurn := r.maxTurn
	if maxTurn <= 0 {
		maxTurn = 4
	}
	user := node.Prompt
	if input != nil {
		user = fmt.Sprintf("%s\n\n输入:\n%v", node.Prompt, input)
	}
	messages := []extract.ChatMessage{
		{Role: "system", Content: "你是编排引擎中的步骤代理。按提示完成任务；需要时调用工具；最后给出简洁结论。"},
		{Role: "user", Content: user},
	}
	var last string
	var actions []string
	for i := 0; i < maxTurn; i++ {
		turn, _, err := r.chatter.Chat(ctx, messages, r.tools)
		if err != nil {
			return NodeResult{}, err
		}
		messages = append(messages, extract.ChatMessage{
			Role: "assistant", Content: turn.Content, Reasoning: turn.Reasoning, Calls: turn.ToolCalls,
		})
		if len(turn.ToolCalls) == 0 {
			last = turn.Content
			break
		}
		if r.handle == nil {
			return NodeResult{}, fmt.Errorf("模型请求工具调用，但未注入 AgentToolHandler")
		}
		for _, call := range turn.ToolCalls {
			out, err := r.handle(ctx, call.Name, call.Arguments)
			if err != nil {
				out = fmt.Sprintf(`{"error":%q}`, err.Error())
			}
			actions = append(actions, call.Name)
			messages = append(messages, extract.ChatMessage{Role: "tool", ToolCall: call.ID, Content: out})
		}
	}
	return NodeResult{Output: map[string]any{
		"content": last,
		"actions": actions,
	}}, nil
}

// runnerFor picks the concrete runner for a node type.
func (r Registry) runnerFor(node Node) (Runner, error) {
	switch node.Type {
	case "tool":
		return toolRunner{tools: r.Tools}, nil
	case "condition":
		return conditionRunner{}, nil
	case "agent":
		return agentRunner{
			chatter: r.Chatter,
			tools:   r.AgentTools,
			handle:  r.AgentToolHandler,
		}, nil
	default:
		return nil, fmt.Errorf("未知节点类型: %s", node.Type)
	}
}
