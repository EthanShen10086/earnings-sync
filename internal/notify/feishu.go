package notify

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
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

// Feishu posts to a custom bot webhook. Secret is the optional signing secret.
type Feishu struct {
	Webhook string
	Secret  string
	Client  *http.Client
}

// Sign matches Feishu's custom-bot signature: HMAC-SHA256 of an empty string,
// keyed by "<unix>\n<secret>".
func Sign(stamp, secret string) string {
	mac := hmac.New(sha256.New, []byte(stamp+"\n"+secret))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

func (f Feishu) Send(ctx context.Context, msg Message) error {
	if f.Webhook == "" {
		return fmt.Errorf("FEISHU_WEBHOOK_URL is empty")
	}
	client := f.Client
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	body := map[string]any{
		"msg_type": "text",
		"content":  map[string]string{"text": msg.Text()},
	}
	if f.Secret != "" {
		stamp := strconv.FormatInt(time.Now().Unix(), 10)
		body["timestamp"] = stamp
		body["sign"] = Sign(stamp, f.Secret)
	}
	payload, _ := json.Marshal(body)
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
	raw, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
	if res.StatusCode >= 300 {
		return fmt.Errorf("飞书 HTTP %s：%s", res.Status, trim(raw))
	}
	var parsed struct {
		Code          int    `json:"code"`
		StatusCode    int    `json:"StatusCode"`
		Msg           string `json:"msg"`
		StatusMessage string `json:"StatusMessage"`
	}
	if json.Unmarshal(raw, &parsed) == nil {
		code := parsed.Code
		if code == 0 {
			code = parsed.StatusCode
		}
		msg := parsed.Msg
		if msg == "" {
			msg = parsed.StatusMessage
		}
		if code != 0 {
			return fmt.Errorf("飞书拒绝了消息：%s（code %d）。检查 FEISHU_WEBHOOK_URL 和 FEISHU_WEBHOOK_SECRET 是否来自同一个机器人", msg, code)
		}
	}
	return nil
}

func trim(b []byte) string {
	s := strings.TrimSpace(string(b))
	if len(s) > 180 {
		return s[:180]
	}
	return s
}
