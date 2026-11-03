package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/EthanShen10086/earnings-sync/internal/model"
)

// Message is the Feishu text. The takeaway is always first.
type Message struct {
	Ticker   string
	Period   string
	Takeaway []string
	Warnings []string
	Charts   []model.ChartSpec
	SheetURL string
}

func (m Message) Text() string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s %s\n", m.Ticker, m.Period)
	b.WriteString("需要关注\n")
	if len(m.Takeaway) == 0 {
		b.WriteString("- 这期没有抽出可比较的重点\n")
	}
	for _, line := range m.Takeaway {
		b.WriteString("- " + line + "\n")
	}
	b.WriteString("勾稽\n")
	if len(m.Warnings) == 0 {
		b.WriteString("- 未发现勾稽差异\n")
	}
	for _, line := range m.Warnings {
		b.WriteString("- " + line + "\n")
	}
	if m.SheetURL != "" {
		b.WriteString("表格\n" + m.SheetURL + "\n")
	}
	if len(m.Charts) > 0 {
		b.WriteString("图表\n")
		for _, c := range m.Charts {
			if c.Source == model.SourceModel {
				fmt.Fprintf(&b, "- 建议 %s：%s。%s\n", c.ChartType, c.Title, c.Reason)
				continue
			}
			fmt.Fprintf(&b, "- %s：%s。%s\n", c.ChartType, c.Title, c.Reason)
		}
	}
	return b.String()
}

// Sender delivers a message. Feishu is the first implementation.
type Sender interface {
	Send(ctx context.Context, msg Message) error
}

// Feishu posts to a custom bot webhook.
type Feishu struct {
	Webhook string
	Client  *http.Client
}

func (f Feishu) Send(ctx context.Context, msg Message) error {
	if f.Webhook == "" {
		return fmt.Errorf("FEISHU_WEBHOOK_URL is empty")
	}
	client := f.Client
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	payload, _ := json.Marshal(map[string]any{
		"msg_type": "text",
		"content":  map[string]string{"text": msg.Text()},
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, f.Webhook, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		return fmt.Errorf("feishu: %s", res.Status)
	}
	return nil
}
