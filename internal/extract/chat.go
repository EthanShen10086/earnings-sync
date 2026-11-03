package extract

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// ToolDef is one function the model is allowed to call.
type ToolDef struct {
	Name        string
	Description string
	Parameters  map[string]any
}

// ToolCall is one invocation requested by the model.
type ToolCall struct {
	ID        string
	Name      string
	Arguments string
}

// Turn is one assistant response. ToolCalls empty means the model is done acting.
type Turn struct {
	Content   string
	Reasoning string
	ToolCalls []ToolCall
}

// ChatMessage is one item in a tool-calling conversation.
type ChatMessage struct {
	Role      string
	Content   string
	Reasoning string
	ToolCall  string
	Calls     []ToolCall
}

// Chat sends a tool-capable completion. Retired model names are rewritten to deepseek-flash.
func (p HTTPProvider) Chat(ctx context.Context, messages []ChatMessage, tools []ToolDef) (Turn, TokenUsage, error) {
	if p.APIKey == "" {
		return Turn{}, TokenUsage{}, fmt.Errorf("model api key is empty")
	}
	client := p.Client
	if client == nil {
		client = http.DefaultClient
	}
	payload := map[string]any{
		"model":       p.AgentModel(),
		"messages":    encodeMessages(messages),
		"tools":       encodeTools(tools),
		"temperature": 0,
		"thinking":    map[string]string{"type": "disabled"},
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return Turn{}, TokenUsage{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(p.BaseURL, "/")+"/chat/completions", bytes.NewReader(b))
	if err != nil {
		return Turn{}, TokenUsage{}, err
	}
	req.Header.Set("Authorization", "Bearer "+p.APIKey)
	req.Header.Set("Content-Type", "application/json")
	res, err := client.Do(req)
	if err != nil {
		return Turn{}, TokenUsage{}, err
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, 4<<20))
	if err != nil {
		return Turn{}, TokenUsage{}, err
	}
	if res.StatusCode >= 300 {
		return Turn{}, TokenUsage{}, fmt.Errorf("model %s: %s: %s", p.AgentModel(), res.Status, clipBody(body))
	}
	var parsed chatResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return Turn{}, TokenUsage{}, err
	}
	if len(parsed.Choices) == 0 {
		return Turn{}, TokenUsage{}, fmt.Errorf("model returned no choices")
	}
	msg := parsed.Choices[0].Message
	turn := Turn{Content: msg.Content, Reasoning: msg.ReasoningContent}
	for _, call := range msg.ToolCalls {
		turn.ToolCalls = append(turn.ToolCalls, ToolCall{
			ID: call.ID, Name: call.Function.Name, Arguments: call.Function.Arguments,
		})
	}
	return turn, TokenUsage{Total: parsed.Usage.TotalTokens}, nil
}

// AgentModel is the tool-calling model actually requested.
func (p HTTPProvider) AgentModel() string {
	switch strings.TrimSpace(p.Model) {
	case "", "deepseek-chat", "deepseek-reasoner":
		return "deepseek-flash"
	default:
		return p.Model
	}
}

func encodeMessages(in []ChatMessage) []map[string]any {
	out := make([]map[string]any, 0, len(in))
	for _, m := range in {
		item := map[string]any{"role": m.Role}
		if m.Content != "" {
			item["content"] = m.Content
		}
		if m.Reasoning != "" {
			item["reasoning_content"] = m.Reasoning
		}
		if m.ToolCall != "" {
			item["tool_call_id"] = m.ToolCall
		}
		if len(m.Calls) > 0 {
			calls := make([]map[string]any, 0, len(m.Calls))
			for _, c := range m.Calls {
				calls = append(calls, map[string]any{
					"id":   c.ID,
					"type": "function",
					"function": map[string]string{
						"name":      c.Name,
						"arguments": c.Arguments,
					},
				})
			}
			item["tool_calls"] = calls
		}
		out = append(out, item)
	}
	return out
}

func encodeTools(tools []ToolDef) []map[string]any {
	out := make([]map[string]any, 0, len(tools))
	for _, tool := range tools {
		params := tool.Parameters
		if params == nil {
			params = map[string]any{"type": "object", "properties": map[string]any{}}
		}
		out = append(out, map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        tool.Name,
				"description": tool.Description,
				"parameters":  params,
			},
		})
	}
	return out
}

func clipBody(b []byte) string {
	s := strings.TrimSpace(string(b))
	if len(s) > 240 {
		return s[:240]
	}
	return s
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content          string `json:"content"`
			ReasoningContent string `json:"reasoning_content"`
			ToolCalls        []struct {
				ID       string `json:"id"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"message"`
	} `json:"choices"`
	Usage struct {
		TotalTokens int `json:"total_tokens"`
	} `json:"usage"`
}
