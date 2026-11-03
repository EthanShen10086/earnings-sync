package extract

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// TokenUsage is what the provider reports, used for the token metric.
type TokenUsage struct {
	Total int
}

// Provider is a chat model. DeepSeek is the default; any OpenAI-compatible API can replace it.
type Provider interface {
	Name() string
	Complete(ctx context.Context, system, user string) (string, TokenUsage, error)
}

// HTTPProvider calls an OpenAI-compatible chat completions endpoint.
type HTTPProvider struct {
	NameStr string
	BaseURL string
	APIKey  string
	Model   string
	Client  *http.Client
}

func (p HTTPProvider) Name() string {
	if p.NameStr != "" {
		return p.NameStr
	}
	return "openai-compatible"
}

func (p HTTPProvider) Complete(ctx context.Context, system, user string) (string, TokenUsage, error) {
	if p.APIKey == "" {
		return "", TokenUsage{}, fmt.Errorf("model api key is empty")
	}
	client := p.Client
	if client == nil {
		client = &http.Client{Timeout: 150 * time.Second}
	}
	base := strings.TrimRight(p.BaseURL, "/")
	payload := map[string]any{
		"model": p.AgentModel(),
		"messages": []map[string]string{
			{"role": "system", "content": system},
			{"role": "user", "content": user},
		},
		"temperature":     0,
		"response_format": map[string]string{"type": "json_object"},
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return "", TokenUsage{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/chat/completions", bytes.NewReader(b))
	if err != nil {
		return "", TokenUsage{}, err
	}
	req.Header.Set("Authorization", "Bearer "+p.APIKey)
	req.Header.Set("Content-Type", "application/json")
	res, err := client.Do(req)
	if err != nil {
		return "", TokenUsage{}, err
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, 4<<20))
	if err != nil {
		return "", TokenUsage{}, err
	}
	if res.StatusCode >= 300 {
		return "", TokenUsage{}, fmt.Errorf("model %s: %s", p.Name(), res.Status)
	}
	var parsed struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage struct {
			TotalTokens int `json:"total_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", TokenUsage{}, err
	}
	if len(parsed.Choices) == 0 {
		return "", TokenUsage{}, fmt.Errorf("model returned no choices")
	}
	slog.Info("deepseek completion", "model", p.AgentModel(), "tokens", parsed.Usage.TotalTokens)
	return parsed.Choices[0].Message.Content, TokenUsage{Total: parsed.Usage.TotalTokens}, nil
}

// StripFence removes a markdown json fence if the model added one.
func StripFence(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "```") {
		s = strings.TrimPrefix(s, "```json")
		s = strings.TrimPrefix(s, "```")
		s = strings.TrimSuffix(s, "```")
	}
	return strings.TrimSpace(s)
}
