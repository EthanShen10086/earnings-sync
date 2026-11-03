package notify

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFeishuWebhook(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		got = string(b)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	err := (Feishu{Webhook: srv.URL, Client: srv.Client()}).Send(context.Background(), Message{
		Ticker: "AAPL", Period: "2026Q4", Takeaway: []string{"服务增速高于硬件"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "服务增速高于硬件") || !strings.Contains(got, "msg_type") {
		t.Fatal(got)
	}
}
