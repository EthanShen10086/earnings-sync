// Package features resolves pluggable advanced capabilities and graceful degradation.
// Secrets and backends are optional: flags off → noop; flags on without deps → degrade to noop;
// flags on with deps → enable. The minimum ingest→store→query path never requires them.
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
	StateDegraded State = "degraded" // flag on but dependency missing → stub/noop
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
	Notify        Want // Feishu; auto = webhook present
	LLM           Want // DeepSeek; auto = API key present
	SheetsGoogle  Want // Google Sheets; auto = creds present
	OTelExport    Want // OTLP; auto = endpoint present
}

// Deps are optional connection strings / credential paths (placeholders OK when empty).
type Deps struct {
	RedisURL       string
	MQURL          string
	KafkaBrokers   string
	ObjectEndpoint string
	ObjectBucket   string
	FeishuWebhook  string
	ModelAPIKey    string
	GoogleCreds    string
	OTLPEndpoint   string
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
			Notify:        parseWant(os.Getenv("FEATURE_NOTIFY"), WantAuto),
			LLM:           parseWant(os.Getenv("FEATURE_LLM"), WantAuto),
			SheetsGoogle:  parseWant(os.Getenv("FEATURE_SHEETS_GOOGLE"), WantAuto),
			OTelExport:    parseWant(os.Getenv("FEATURE_OTEL"), WantAuto),
		}, Deps{
			RedisURL:       strings.TrimSpace(os.Getenv("REDIS_URL")),
			MQURL:          strings.TrimSpace(firstEnv("MQ_URL", "AMQP_URL")),
			KafkaBrokers:   strings.TrimSpace(os.Getenv("KAFKA_BROKERS")),
			ObjectEndpoint: strings.TrimSpace(firstEnv("OBJECT_STORAGE_ENDPOINT", "S3_ENDPOINT")),
			ObjectBucket:   strings.TrimSpace(firstEnv("OBJECT_STORAGE_BUCKET", "S3_BUCKET")),
			FeishuWebhook:  strings.TrimSpace(os.Getenv("FEISHU_WEBHOOK_URL")),
			ModelAPIKey:    strings.TrimSpace(os.Getenv("DEEPSEEK_API_KEY")),
			GoogleCreds:    strings.TrimSpace(os.Getenv("GOOGLE_APPLICATION_CREDENTIALS")),
			OTLPEndpoint:   strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")),
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

	b.Report = append(b.Report, resolveCred("notify", flags.Notify, deps.FeishuWebhook != "", "FEISHU_WEBHOOK_URL"))
	b.Report = append(b.Report, resolveCred("llm", flags.LLM, deps.ModelAPIKey != "", "DEEPSEEK_API_KEY"))
	b.Report = append(b.Report, resolveCred("sheets_google", flags.SheetsGoogle, deps.GoogleCreds != "", "GOOGLE_APPLICATION_CREDENTIALS"))
	b.Report = append(b.Report, resolveCred("otel", flags.OTelExport, deps.OTLPEndpoint != "", "OTEL_EXPORTER_OTLP_ENDPOINT"))
	return b
}

// UseGoogleSheets is true when Google Sheets should be attempted (creds present and not forced off).
func (b Bundle) UseGoogleSheets() bool {
	return wantActive(b.Flags.SheetsGoogle, b.Deps.GoogleCreds != "")
}

// UseLLM is true when the model provider should be wired.
func (b Bundle) UseLLM() bool {
	return wantActive(b.Flags.LLM, b.Deps.ModelAPIKey != "")
}

// UseNotify is true when Feishu (or other notifier) should be wired.
func (b Bundle) UseNotify() bool {
	return wantActive(b.Flags.Notify, b.Deps.FeishuWebhook != "")
}

// UseOTel is true when OTLP export should be attempted.
func (b Bundle) UseOTel() bool {
	return wantActive(b.Flags.OTelExport, b.Deps.OTLPEndpoint != "")
}

// Enabled reports whether a named advanced plugin is live (not stub).
func (b Bundle) Enabled(name string) bool {
	for _, s := range b.Report {
		if s.Name == name {
			return s.State == StateEnabled
		}
	}
	return false
}

func wantActive(w Want, depOK bool) bool {
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
		} else {
			st.State = StateDegraded
			st.Detail = depName + " missing; capability skipped"
		}
	default: // auto
		if depOK {
			st.State = StateEnabled
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
		if depOK {
			*slot = live()
			st.State = StateEnabled
			st.Detail = "enabled (in-process adapter; remote client optional later)"
			return st
		}
		st.State = StateDegraded
		st.Detail = "flag on but dependency missing; using noop"
		return st
	default: // auto
		if depOK {
			*slot = live()
			st.State = StateEnabled
			st.Detail = "auto-enabled"
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
