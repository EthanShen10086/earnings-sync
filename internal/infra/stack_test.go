package infra_test

import (
	"bytes"
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/EthanShen10086/earnings-sync/internal/adapters/audit"
	"github.com/EthanShen10086/earnings-sync/internal/adapters/cache"
	"github.com/EthanShen10086/earnings-sync/internal/adapters/gateway"
	"github.com/EthanShen10086/earnings-sync/internal/adapters/mq"
	"github.com/EthanShen10086/earnings-sync/internal/adapters/search"
	"github.com/EthanShen10086/earnings-sync/internal/config"
	"github.com/EthanShen10086/earnings-sync/internal/infra"
	"github.com/EthanShen10086/earnings-sync/internal/observability"
)

func TestOpenDegradesWithoutBackends(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	settings := config.Settings{
		RedisURL:         "127.0.0.1:1", // closed port
		NATSURL:          "127.0.0.1:1",
		ObjectStoreDir:   filepath.Join(dir, "obj"),
		AuditLogPath:     filepath.Join(dir, "audit.jsonl"),
		SearchIndexPath:  filepath.Join(dir, "search"),
		CDNBaseURL:       "https://cdn.example.test",
		RateLimitRPS:     10,
		RateLimitBurst:   2,
		InfraStrict:      false,
		DiscoveryBackend: "nacos",
		DiscoveryURL:     "",
		KafkaBrokers:     "127.0.0.1:1",
		KafkaEventLog:    filepath.Join(dir, "kafka.jsonl"),
		LogShipperPath:   filepath.Join(dir, "logs", "app.jsonl"),
		AlertRulesPath:   filepath.Join("..", "..", "deploy", "alert-rules.yml"),
	}
	stack, err := infra.Open(ctx, settings)
	if err != nil {
		t.Fatalf("Open should not fail when degraded: %v", err)
	}
	defer stack.Close()

	if !stack.Degraded() {
		t.Fatalf("expected degraded stack, status=%+v", stack.Status)
	}
	if stack.Status.Cache.Backend != "memory" {
		t.Fatalf("cache backend=%s", stack.Status.Cache.Backend)
	}
	if stack.Status.Queue.Backend != "memory" {
		t.Fatalf("queue backend=%s", stack.Status.Queue.Backend)
	}
	if stack.Status.Objects.CDN.Enabled != true {
		t.Fatalf("cdn should be enabled placeholder")
	}

	if err := stack.Cache.Set(ctx, "k", "v", time.Minute); err != nil {
		t.Fatal(err)
	}
	v, ok, err := stack.Cache.Get(ctx, "k")
	if err != nil || !ok || v != "v" {
		t.Fatalf("cache get: v=%q ok=%v err=%v", v, ok, err)
	}

	got := make(chan string, 1)
	subCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() {
		_ = stack.Queue.Subscribe(subCtx, "t", func(m mq.Message) error {
			got <- string(m.Body)
			return nil
		})
	}()
	// Give subscribe a moment to register on memory queue.
	time.Sleep(20 * time.Millisecond)
	if _, err := stack.Queue.Publish(ctx, "t", []byte("hi")); err != nil {
		t.Fatal(err)
	}
	select {
	case body := <-got:
		if body != "hi" {
			t.Fatalf("body=%q", body)
		}
	case <-time.After(time.Second):
		t.Fatal("memory queue did not deliver")
	}

	obj, err := stack.Objects.Put(ctx, "filings/a.txt", bytes.NewBufferString("pdf"), "text/plain")
	if err != nil {
		t.Fatal(err)
	}
	if obj.URL != "https://cdn.example.test/filings/a.txt" {
		t.Fatalf("cdn url=%q", obj.URL)
	}

	if err := stack.Audit.Append(ctx, audit.Event{Actor: "test", Action: "write", Target: "aapl"}); err != nil {
		t.Fatal(err)
	}
	events, err := stack.Audit.List(ctx, 10)
	if err != nil || len(events) != 1 {
		t.Fatalf("audit list: %v len=%d", err, len(events))
	}

	if err := stack.Search.Upsert(ctx, search.Document{ID: "1", Title: "AAPL", Body: "revenue up"}); err != nil {
		t.Fatal(err)
	}
	hits, err := stack.Search.Query(ctx, "aapl", 5)
	if err != nil || len(hits) != 1 {
		t.Fatalf("search: %v hits=%d", err, len(hits))
	}

	if !stack.Gateway.Allow("1.1.1.1") {
		t.Fatal("first request should pass")
	}
	if stack.Status.Discovery.Backend != "nacos-memory" {
		t.Fatalf("discovery=%+v", stack.Status.Discovery)
	}
	inst, err := stack.Discovery.Discover(ctx, "earnings-sync")
	if err != nil || len(inst) != 1 {
		t.Fatalf("self-register: %v %+v", err, inst)
	}
	if err := stack.Kafka.Publish(ctx, "t", "k", []byte("{}")); err != nil {
		t.Fatal(err)
	}
	if stack.Kafka.Len() != 1 {
		t.Fatal("kafka file bus")
	}
	if stack.Status.AlertRules < 1 {
		t.Fatalf("alert rules not loaded: %d", stack.Status.AlertRules)
	}
	if err := stack.Shipper.Ship(ctx, observability.LogEvent{Level: "info", Message: "stack-test"}); err != nil {
		t.Fatal(err)
	}
}

func TestOpenStrictFailsOnBadRedis(t *testing.T) {
	ctx := context.Background()
	_, err := infra.Open(ctx, config.Settings{
		RedisURL:    "127.0.0.1:1",
		InfraStrict: true,
	})
	if err == nil {
		t.Fatal("expected strict open to fail")
	}
}

func TestMemoryCacheAndLimiter(t *testing.T) {
	c := cache.NewMemory()
	ctx := context.Background()
	_ = c.Set(ctx, "a", "1", 0)
	v, ok, err := c.Get(ctx, "a")
	if err != nil || !ok || v != "1" {
		t.Fatalf("got %q %v %v", v, ok, err)
	}
	lim := gateway.NewLimiter(1, 1)
	if !lim.Allow("x") {
		t.Fatal("burst")
	}
	if lim.Allow("x") {
		t.Fatal("should rate limit")
	}
}

func TestOpenUnconfiguredMinimumPath(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	stack, err := infra.Open(ctx, config.Settings{
		ObjectStoreDir:   filepath.Join(dir, "obj"),
		AuditLogPath:     filepath.Join(dir, "a.jsonl"),
		SearchIndexPath:  filepath.Join(dir, "s"),
		KafkaEventLog:    filepath.Join(dir, "kafka.jsonl"),
		LogShipperPath:   filepath.Join(dir, "app.jsonl"),
		DiscoveryBackend: "memory",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer stack.Close()
	if stack.Degraded() {
		t.Fatalf("unconfigured should not count as degraded dial failure: %+v", stack.Status)
	}
	if stack.Summary() == "" {
		t.Fatal("empty summary")
	}
}
