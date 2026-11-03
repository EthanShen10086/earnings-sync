package extract

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHTTPProvider(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("auth %s", r.Header.Get("Authorization"))
		}
		b, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(b), "deepseek-flash") {
			t.Fatalf("body %s", b)
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"ok\":true}"}}],"usage":{"total_tokens":12}}`))
	}))
	defer srv.Close()
	p := HTTPProvider{BaseURL: srv.URL, APIKey: "test-key", Model: "deepseek-chat", Client: srv.Client()}
	text, usage, err := p.Complete(context.Background(), "sys", "user")
	if err != nil || !strings.Contains(text, "ok") || usage.Total != 12 {
		t.Fatalf("%s %v %v", text, usage, err)
	}
}
