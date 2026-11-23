// Package features resolves pluggable advanced capabilities and graceful degradation.
// Secrets and backends are optional: flags off → noop; flags on without remote deps →
// in-process Memory/file stubs that still execute (cache hit, queue deliver, audit append…);
// flags on with deps → same stub adapters (remote clients live under internal/infra).
// The minimum ingest→store→query path never requires advanced flags.
package features

import (
	"os"
	"strings"
)

// State is the resolved outcome for one capability.
type State string

const (
	StateDisabled State = "disabled" // flag off
	StateEnabled  State = "enabled"  // flag on and dependency available
	StateDegraded State = "degraded" // flag on but dependency missing → runnable in-process stub
	StateAutoOff  State = "auto-off" // auto mode, dependency missing → stay off
)

// Want is the configured desire for a feature before dependency checks.
type Want string

const (
	WantOff  Want = "off"
	WantOn   Want = "on"
	WantAuto Want = "auto"
)

// Flags are advanced / optional capabilities. Core storage (Memory/Sheets/PG) is separate.
type Flags struct {
	Redis         Want
	MQ            Want
	Kafka         Want
	ObjectStorage Want
	Audit         Want
	Search        Want
	Discovery     Want
	ELK           Want // log shipper → file / Elasticsearch
	Notify        Want // Feishu; auto = webhook present
	LLM           Want // DeepSeek; auto = API key present
	SheetsGoogle  Want // Google Sheets; auto = creds present
	OTelExport    Want // OTLP; auto = endpoint present
}

// Deps are optional connection strings / credential paths (placeholders OK when empty).
type Deps struct {
	RedisURL         string
	MQURL            string
	KafkaBrokers     string
	ObjectEndpoint   string
	ObjectBucket     string
	DiscoveryURL     string
	Elasticsearch    string
	FeishuWebhook    string
	ModelAPIKey      string
	GoogleCreds      string
	GoogleOAuthToken string
	OTLPEndpoint     string
}

// Status is one resolved capability.
type Status struct {
	Name   string `json:"name"`
	Want   Want   `json:"want"`
	State  State  `json:"state"`
	Detail string `json:"detail,omitempty"`
}

// Bundle is the runtime view of advanced plugins + resolution report.
type Bundle struct {
	Flags   Flags
	Deps    Deps
	Report  []Status
	Cache   Cache
	Queue   Queue
	Bus     EventBus
	Objects ObjectStore
	Audit   Auditor
	Search  Searcher
}

// FromEnv loads flags and dependency placeholders from the process environment.
func FromEnv() (Flags, Deps) {
	return Flags{
			Redis:         parseWant(os.Getenv("FEATURE_REDIS"), WantOff),
			MQ:            parseWant(os.Getenv("FEATURE_MQ"), WantOff),
			Kafka:         parseWant(os.Getenv("FEATURE_KAFKA"), WantOff),
			ObjectStorage: parseWant(os.Getenv("FEATURE_OBJECT_STORAGE"), WantOff),
			Audit:         parseWant(os.Getenv("FEATURE_AUDIT"), WantOff),
			Search:        parseWant(os.Getenv("FEATURE_SEARCH"), WantOff),
			Discovery:     parseWant(os.Getenv("FEATURE_DISCOVERY"), WantOff),
			ELK:           parseWant(firstEnv("FEATURE_ELK", "FEATURE_LOG_SHIPPER"), WantOff),
			Notify:        parseWant(os.Getenv("FEATURE_NOTIFY"), WantAuto),
			LLM:           parseWant(os.Getenv("FEATURE_LLM"), WantAuto),
			SheetsGoogle:  parseWant(os.Getenv("FEATURE_SHEETS_GOOGLE"), WantAuto),
			OTelExport:    parseWant(os.Getenv("FEATURE_OTEL"), WantAuto),
		}, Deps{
			RedisURL:         strings.TrimSpace(os.Getenv("REDIS_URL")),
			MQURL:            strings.TrimSpace(firstEnv("MQ_URL", "AMQP_URL")),
			KafkaBrokers:     strings.TrimSpace(os.Getenv("KAFKA_BROKERS")),
			ObjectEndpoint:   strings.TrimSpace(firstEnv("OBJECT_STORAGE_ENDPOINT", "S3_ENDPOINT")),
			ObjectBucket:     strings.TrimSpace(firstEnv("OBJECT_STORAGE_BUCKET", "S3_BUCKET")),
			DiscoveryURL:     strings.TrimSpace(firstEnv("DISCOVERY_URL", "NACOS_ADDR", "CONSUL_HTTP_ADDR", "EUREKA_URL")),
			Elasticsearch:    strings.TrimSpace(firstEnv("ELASTICSEARCH_URL", "ES_URL")),
			FeishuWebhook:    strings.TrimSpace(os.Getenv("FEISHU_WEBHOOK_URL")),
			ModelAPIKey:      strings.TrimSpace(os.Getenv("DEEPSEEK_API_KEY")),
			GoogleCreds:      strings.TrimSpace(os.Getenv("GOOGLE_APPLICATION_CREDENTIALS")),
			GoogleOAuthToken: strings.TrimSpace(os.Getenv("GOOGLE_OAUTH_TOKEN")),
			OTLPEndpoint:     strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")),
		}
}

// Resolve builds stubs or live adapters according to flags and dependency availability.
// It never fails: missing advanced backends always degrade.
func Resolve(flags Flags, deps Deps) Bundle {
	b := Bundle{
		Flags:   flags,
		Deps:    deps,
		Cache:   NoopCache{},
		Queue:   NoopQueue{},
		Bus:     NoopBus{},
		Objects: NoopObjects{},
		Audit:   NoopAudit{},
		Search:  NoopSearch{},
	}

	b.Report = append(b.Report, resolvePlugin("redis", flags.Redis, deps.RedisURL != "", &b.Cache, func() Cache {
		return &MemoryCache{}
	}))
	b.Report = append(b.Report, resolvePlugin("mq", flags.MQ, deps.MQURL != "", &b.Queue, func() Queue {
		return &MemoryQueue{}
	}))
	b.Report = append(b.Report, resolvePlugin("kafka", flags.Kafka, deps.KafkaBrokers != "", &b.Bus, func() EventBus {
		return &MemoryBus{}
	}))
	b.Report = append(b.Report, resolvePlugin("object_storage", flags.ObjectStorage, deps.ObjectEndpoint != "" && deps.ObjectBucket != "", &b.Objects, func() ObjectStore {
		return &MemoryObjects{}
	}))
	b.Report = append(b.Report, resolvePlugin("audit", flags.Audit, true, &b.Audit, func() Auditor {
		return &MemoryAudit{}
	}))
	b.Report = append(b.Report, resolvePlugin("search", flags.Search, true, &b.Search, func() Searcher {
		return &MemorySearch{}
	}))
	// Discovery / ELK are wired primarily via internal/infra; flag report keeps Active() honest.
	b.Report = append(b.Report, resolveCred("discovery", flags.Discovery, deps.DiscoveryURL != "", "DISCOVERY_URL"))
	b.Report = append(b.Report, resolveCred("elk", flags.ELK, deps.Elasticsearch != "", "ELASTICSEARCH_URL"))

	b.Report = append(b.Report, resolveCred("notify", flags.Notify, deps.FeishuWebhook != "", "FEISHU_WEBHOOK_URL"))
	b.Report = append(b.Report, resolveCred("llm", flags.LLM, deps.ModelAPIKey != "", "DEEPSEEK_API_KEY"))
	hasSheets := deps.GoogleCreds != "" || deps.GoogleOAuthToken != ""
	sheetsHint := "GOOGLE_APPLICATION_CREDENTIALS|GOOGLE_OAUTH_TOKEN"
	b.Report = append(b.Report, resolveCred("sheets_google", flags.SheetsGoogle, hasSheets, sheetsHint))
	b.Report = append(b.Report, resolveCred("otel", flags.OTelExport, deps.OTLPEndpoint != "", "OTEL_EXPORTER_OTLP_ENDPOINT"))
	return b
}

// UseGoogleSheets is true when Google Sheets should be attempted (creds present and not forced off).
func (b Bundle) UseGoogleSheets() bool {
	return wantLive(b.Flags.SheetsGoogle, b.Deps.GoogleCreds != "" || b.Deps.GoogleOAuthToken != "")
}

// UseLLM is true when a model provider should be wired (live key or forced-on stub).
func (b Bundle) UseLLM() bool {
	return wantRunnable(b.Flags.LLM, b.Deps.ModelAPIKey != "")
}

// LiveLLM is true when a real API key is present for the remote model.
func (b Bundle) LiveLLM() bool {
	return wantLive(b.Flags.LLM, b.Deps.ModelAPIKey != "")
}

// UseNotify is true when a notifier should be wired (live webhook or forced-on stub).
func (b Bundle) UseNotify() bool {
	return wantRunnable(b.Flags.Notify, b.Deps.FeishuWebhook != "")
}

// LiveNotify is true when a real Feishu webhook is configured.
func (b Bundle) LiveNotify() bool {
	return wantLive(b.Flags.Notify, b.Deps.FeishuWebhook != "")
}

// UseOTel is true when OTLP export should be attempted.
func (b Bundle) UseOTel() bool {
	return wantLive(b.Flags.OTelExport, b.Deps.OTLPEndpoint != "")
}

// Enabled reports whether a named advanced plugin is fully live (deps present).
func (b Bundle) Enabled(name string) bool {
	for _, s := range b.Report {
		if s.Name == name {
			return s.State == StateEnabled
		}
	}
	return false
}

// Active reports whether a named plugin runs real logic (enabled or degraded stub).
// Flag-off / auto-off plugins are inactive and use noop.
func (b Bundle) Active(name string) bool {
	for _, s := range b.Report {
		if s.Name == name {
			return s.State == StateEnabled || s.State == StateDegraded
		}
	}
	return false
}

// wantRunnable: WantOn always runs (stub if no dep); WantAuto only with dep.
func wantRunnable(w Want, depOK bool) bool {
	switch w {
	case WantOff:
		return false
	case WantOn:
		return true
	default: // auto
		return depOK
	}
}

// wantLive: only when dependency is actually present (no stub path).
func wantLive(w Want, depOK bool) bool {
	switch w {
	case WantOff:
		return false
	case WantOn, WantAuto:
		return depOK
	default:
		return depOK
	}
}

func resolveCred(name string, w Want, depOK bool, depName string) Status {
	st := Status{Name: name, Want: w}
	switch w {
	case WantOff:
		st.State = StateDisabled
		st.Detail = "flag off"
	case WantOn:
		if depOK {
			st.State = StateEnabled
			st.Detail = "live credentials present"
		} else {
			st.State = StateDegraded
			st.Detail = depName + " missing; using local stub provider"
		}
	default: // auto
		if depOK {
			st.State = StateEnabled
			st.Detail = "auto; live credentials present"
		} else {
			st.State = StateAutoOff
			st.Detail = depName + " unset"
		}
	}
	return st
}

func resolvePlugin[T any](name string, w Want, depOK bool, slot *T, live func() T) Status {
	st := Status{Name: name, Want: w}
	switch w {
	case WantOff:
		st.State = StateDisabled
		st.Detail = "flag off; using noop"
		return st
	case WantOn:
		*slot = live()
		if depOK {
			st.State = StateEnabled
			st.Detail = "enabled (in-process stub; remote client via internal/infra when dialable)"
			return st
		}
		st.State = StateDegraded
		st.Detail = "flag on; dependency missing — using in-process stub (not noop)"
		return st
	default: // auto
		if depOK {
			*slot = live()
			st.State = StateEnabled
			st.Detail = "auto-enabled (in-process stub)"
			return st
		}
		st.State = StateAutoOff
		st.Detail = "auto; dependency unset"
		return st
	}
}

func parseWant(raw string, fallback Want) Want {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "":
		return fallback
	case "0", "false", "off", "no", "disabled":
		return WantOff
	case "1", "true", "on", "yes", "enabled":
		return WantOn
	case "auto":
		return WantAuto
	default:
		return fallback
	}
}

func firstEnv(keys ...string) string {
	for _, k := range keys {
		if v := strings.TrimSpace(os.Getenv(k)); v != "" {
			return v
		}
	}
	return ""
}
