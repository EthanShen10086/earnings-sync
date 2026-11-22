package kafka_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/EthanShen10086/earnings-sync/internal/adapters/kafka"
)

func TestMemoryPublishSubscribe(t *testing.T) {
	ctx := context.Background()
	bus := kafka.NewMemory()
	got := make(chan string, 1)
	subCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() {
		_ = bus.Subscribe(subCtx, "t", func(e kafka.Event) error {
			got <- string(e.Payload)
			return nil
		})
	}()
	time.Sleep(20 * time.Millisecond)
	if err := bus.Publish(ctx, "t", "k", []byte("hi")); err != nil {
		t.Fatal(err)
	}
	select {
	case body := <-got:
		if body != "hi" {
			t.Fatalf("body=%q", body)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout")
	}
	if bus.Len() != 1 {
		t.Fatalf("len=%d", bus.Len())
	}
}

func TestFilePersistsAcrossOpen(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "events.jsonl")
	b1, err := kafka.NewFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := b1.Publish(ctx, "ingest", "AAPL", []byte(`{"ok":1}`)); err != nil {
		t.Fatal(err)
	}
	_ = b1.Close()

	b2, err := kafka.NewFile(path)
	if err != nil {
		t.Fatal(err)
	}
	defer b2.Close()
	if b2.Len() != 1 {
		t.Fatalf("expected reload len=1 got %d", b2.Len())
	}
	snap := b2.Snapshot()
	if snap[0].Topic != "ingest" || snap[0].Key != "AAPL" {
		t.Fatalf("snap=%+v", snap[0])
	}
}

func TestOpenWithoutBrokers(t *testing.T) {
	ctx := context.Background()
	bus, st, err := kafka.Open(ctx, "", "", false)
	if err != nil {
		t.Fatal(err)
	}
	defer bus.Close()
	if st.Backend != "memory" || st.Configured {
		t.Fatalf("%+v", st)
	}
	path := filepath.Join(t.TempDir(), "k.jsonl")
	bus2, st2, err := kafka.Open(ctx, "", path, false)
	if err != nil {
		t.Fatal(err)
	}
	defer bus2.Close()
	if st2.Backend != "file" {
		t.Fatalf("%+v", st2)
	}
}

func TestOpenUnreachableBrokersDegrade(t *testing.T) {
	ctx := context.Background()
	bus, st, err := kafka.Open(ctx, "127.0.0.1:1", "", false)
	if err != nil {
		t.Fatal(err)
	}
	defer bus.Close()
	if !st.Degraded || st.Backend != "memory" {
		t.Fatalf("%+v", st)
	}
	_, _, err = kafka.Open(ctx, "127.0.0.1:1", "", true)
	if err == nil {
		t.Fatal("strict should fail")
	}
}
