package features

import (
	"context"
	"testing"
	"time"
)

func TestResolveFlagOffUsesNoop(t *testing.T) {
	b := Resolve(Flags{
		Redis: WantOff,
		MQ:    WantOff,
		Kafka: WantOff,
	}, Deps{RedisURL: "redis://localhost:6379", MQURL: "amqp://x", KafkaBrokers: "localhost:9092"})
	if b.Enabled("redis") || b.Enabled("mq") || b.Enabled("kafka") {
		t.Fatalf("expected disabled: %+v", b.Report)
	}
	if err := b.Cache.Set(context.Background(), "k", "v", time.Second); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := b.Cache.Get(context.Background(), "k"); ok {
		t.Fatal("noop cache should not store")
	}
}

func TestResolveFlagOnWithoutDepsDegrades(t *testing.T) {
	b := Resolve(Flags{Redis: WantOn, Kafka: WantOn, Audit: WantOn}, Deps{})
	by := map[string]Status{}
	for _, s := range b.Report {
		by[s.Name] = s
	}
	if by["redis"].State != StateDegraded {
		t.Fatalf("redis: %+v", by["redis"])
	}
	if by["kafka"].State != StateDegraded {
		t.Fatalf("kafka: %+v", by["kafka"])
	}
	if by["audit"].State != StateEnabled {
		t.Fatalf("audit needs no remote dep: %+v", by["audit"])
	}
	if err := b.Audit.Record(context.Background(), "write", "test", "ok"); err != nil {
		t.Fatal(err)
	}
	if b.Audit.Len() != 1 {
		t.Fatalf("audit len=%d", b.Audit.Len())
	}
}

func TestResolveFlagOnWithDepsEnables(t *testing.T) {
	b := Resolve(Flags{
		Redis:         WantOn,
		MQ:            WantOn,
		ObjectStorage: WantOn,
		Search:        WantOn,
	}, Deps{
		RedisURL:       "redis://127.0.0.1:6379",
		MQURL:          "amqp://guest@localhost/",
		ObjectEndpoint: "http://127.0.0.1:9000",
		ObjectBucket:   "filings",
	})
	if !b.Enabled("redis") || !b.Enabled("mq") || !b.Enabled("object_storage") || !b.Enabled("search") {
		t.Fatalf("expected enabled: %+v", b.Report)
	}
	ctx := context.Background()
	if err := b.Cache.Set(ctx, "a", "1", 0); err != nil {
		t.Fatal(err)
	}
	if v, ok, err := b.Cache.Get(ctx, "a"); err != nil || !ok || v != "1" {
		t.Fatalf("cache get: v=%q ok=%v err=%v", v, ok, err)
	}
	if err := b.Queue.Enqueue(ctx, "notify", []byte("hi")); err != nil {
		t.Fatal(err)
	}
	if b.Queue.Len("notify") != 1 {
		t.Fatal("queue")
	}
	if err := b.Objects.Put(ctx, "doc/1", []byte("filing")); err != nil {
		t.Fatal(err)
	}
	body, ok, err := b.Objects.Get(ctx, "doc/1")
	if err != nil || !ok || string(body) != "filing" {
		t.Fatalf("objects: %q ok=%v err=%v", body, ok, err)
	}
	_ = b.Search.Index(ctx, "f1", "total revenue rose")
	ids, err := b.Search.Find(ctx, "revenue")
	if err != nil || len(ids) != 1 || ids[0] != "f1" {
		t.Fatalf("search: %v %v", ids, err)
	}
}

func TestCredAutoAndForcedOff(t *testing.T) {
	b := Resolve(Flags{
		LLM:          WantAuto,
		Notify:       WantOn,
		SheetsGoogle: WantOff,
	}, Deps{ModelAPIKey: "sk", FeishuWebhook: "", GoogleCreds: "/tmp/x.json"})
	by := map[string]Status{}
	for _, s := range b.Report {
		by[s.Name] = s
	}
	if by["llm"].State != StateEnabled {
		t.Fatalf("llm: %+v", by["llm"])
	}
	if by["notify"].State != StateDegraded {
		t.Fatalf("notify: %+v", by["notify"])
	}
	if by["sheets_google"].State != StateDisabled {
		t.Fatalf("sheets: %+v", by["sheets_google"])
	}
	if b.UseGoogleSheets() {
		t.Fatal("UseGoogleSheets should be false when flag off")
	}
	if !b.UseLLM() {
		t.Fatal("UseLLM")
	}
	if b.UseNotify() {
		t.Fatal("UseNotify should be false without webhook")
	}
}

func TestParseWant(t *testing.T) {
	if parseWant("ON", WantOff) != WantOn {
		t.Fatal()
	}
	if parseWant("", WantAuto) != WantAuto {
		t.Fatal()
	}
	if parseWant("0", WantOn) != WantOff {
		t.Fatal()
	}
}
