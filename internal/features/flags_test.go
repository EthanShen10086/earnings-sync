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

func TestResolveFlagOnWithoutDepsDegradesToRunnableStub(t *testing.T) {
	b := Resolve(Flags{Redis: WantOn, Kafka: WantOn, MQ: WantOn, ObjectStorage: WantOn, Audit: WantOn, Search: WantOn}, Deps{})
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
	if !b.Active("redis") || !b.Active("kafka") || !b.Active("mq") {
		t.Fatalf("degraded plugins must stay Active: %+v", b.Report)
	}
	if b.Enabled("redis") {
		t.Fatal("Enabled should be false without deps")
	}
	ctx := context.Background()
	if err := b.Cache.Set(ctx, "k", "v", time.Minute); err != nil {
		t.Fatal(err)
	}
	if v, ok, _ := b.Cache.Get(ctx, "k"); !ok || v != "v" {
		t.Fatalf("degraded cache must store: ok=%v v=%q", ok, v)
	}
	if err := b.Queue.Enqueue(ctx, "t", []byte("msg")); err != nil {
		t.Fatal(err)
	}
	if b.Queue.Len("t") != 1 {
		t.Fatal("degraded queue must enqueue")
	}
	if err := b.Bus.Publish(ctx, "events", "k1", []byte("{}")); err != nil {
		t.Fatal(err)
	}
	if b.Bus.Len() != 1 {
		t.Fatal("degraded bus must retain events")
	}
	if err := b.Objects.Put(ctx, "a", []byte("blob")); err != nil {
		t.Fatal(err)
	}
	if body, ok, _ := b.Objects.Get(ctx, "a"); !ok || string(body) != "blob" {
		t.Fatalf("degraded objects: ok=%v body=%q", ok, body)
	}
	if by["audit"].State != StateEnabled {
		t.Fatalf("audit needs no remote dep: %+v", by["audit"])
	}
	if err := b.Audit.Record(ctx, "write", "test", "ok"); err != nil {
		t.Fatal(err)
	}
	if b.Audit.Len() != 1 {
		t.Fatalf("audit len=%d", b.Audit.Len())
	}
	_ = b.Search.Index(ctx, "doc1", "revenue rose")
	ids, err := b.Search.Find(ctx, "revenue")
	if err != nil || len(ids) != 1 {
		t.Fatalf("search: %v %v", ids, err)
	}
}

func TestCredForcedOnUsesStubWithoutSecrets(t *testing.T) {
	b := Resolve(Flags{LLM: WantOn, Notify: WantOn}, Deps{})
	if !b.UseLLM() || b.LiveLLM() {
		t.Fatalf("LLM should be stub runnable: use=%v live=%v", b.UseLLM(), b.LiveLLM())
	}
	if !b.UseNotify() || b.LiveNotify() {
		t.Fatalf("Notify should be stub runnable: use=%v live=%v", b.UseNotify(), b.LiveNotify())
	}
	by := map[string]Status{}
	for _, s := range b.Report {
		by[s.Name] = s
	}
	if by["llm"].State != StateDegraded || by["notify"].State != StateDegraded {
		t.Fatalf("expected degraded stubs: %+v", by)
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
	// WantOn without webhook → stub still runnable.
	if !b.UseNotify() || b.LiveNotify() {
		t.Fatalf("UseNotify stub: use=%v live=%v", b.UseNotify(), b.LiveNotify())
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
