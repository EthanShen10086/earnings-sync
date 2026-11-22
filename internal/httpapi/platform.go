package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/EthanShen10086/earnings-sync/internal/adapters/audit"
	"github.com/EthanShen10086/earnings-sync/internal/adapters/search"
	"github.com/EthanShen10086/earnings-sync/internal/auth"
	"github.com/EthanShen10086/earnings-sync/internal/model"
	"github.com/EthanShen10086/earnings-sync/internal/observability"
	"github.com/EthanShen10086/earnings-sync/internal/pipeline"
)

func observabilityLog(ticker, jobID, status string) observability.LogEvent {
	return observability.LogEvent{
		Timestamp: time.Now().UTC(),
		Level:     "info",
		Message:   "ingest",
		Service:   "earnings-sync",
		Fields: map[string]string{
			"ticker": ticker,
			"job_id": jobID,
			"status": status,
		},
	}
}

// afterWrite runs pluggable platform side-effects when the matching FEATURE_* is active.
// Missing remote backends already degraded inside infra.Stack to memory/file — this still executes.
func (a API) afterWrite(ctx context.Context, actor string, req pipeline.Request, job *model.Job) {
	if a.Infra == nil || job == nil {
		return
	}
	ticker := strings.ToUpper(strings.TrimSpace(req.Ticker))
	if ticker == "" && job.Ticker != "" {
		ticker = job.Ticker
	}
	id := job.ID
	if actor == "" {
		actor = "anonymous"
	}

	if a.featActive("audit") {
		_ = a.Infra.Audit.Append(ctx, audit.Event{
			At:     time.Now().UTC(),
			Actor:  actor,
			Action: "ingest",
			Target: ticker,
			Meta: map[string]string{
				"job_id": id,
				"status": string(job.Status),
				"force":  strconv.FormatBool(req.Force),
			},
		})
	}

	if a.featActive("search") {
		snippet := ticker + " " + id
		if job.Extraction != nil {
			snippet += " " + job.Extraction.Period.Label()
			for _, f := range job.Extraction.Facts {
				snippet += " " + f.LineItem
			}
		}
		_ = a.Infra.Search.Upsert(ctx, search.Document{
			ID:    id,
			Title: ticker,
			Body:  snippet,
		})
	}

	if a.featActive("redis") || a.featActive("cache") {
		key := "facts:" + ticker
		if raw, err := json.Marshal(map[string]any{"job_id": id, "at": time.Now().UTC()}); err == nil {
			_ = a.Infra.Cache.Set(ctx, key, string(raw), 5*time.Minute)
		}
	}

	if a.featActive("mq") {
		payload, _ := json.Marshal(map[string]string{"job_id": id, "ticker": ticker, "event": "ingest"})
		_, _ = a.Infra.Queue.Publish(ctx, "ingest", payload)
	}

	if a.featActive("object_storage") && len(req.Body) > 0 {
		name := req.Filename
		if name == "" {
			name = id + ".bin"
		}
		key := fmt.Sprintf("filings/%s/%s", ticker, name)
		_, _ = a.Infra.Objects.Put(ctx, key, bytes.NewReader(req.Body), "application/octet-stream")
	}

	if a.featActive("kafka") {
		payload, _ := json.Marshal(map[string]string{"job_id": id, "ticker": ticker})
		if a.Infra != nil && a.Infra.Kafka != nil {
			_ = a.Infra.Kafka.Publish(ctx, "earnings.ingest", ticker, payload)
		} else if a.Feat.Bus != nil {
			_ = a.Feat.Bus.Publish(ctx, "earnings.ingest", ticker, payload)
		}
	}

	if a.featActive("elk") && a.Infra != nil && a.Infra.Shipper != nil {
		_ = a.Infra.Shipper.Ship(ctx, observabilityLog(ticker, id, string(job.Status)))
	}

	if a.featActive("discovery") && a.Infra != nil && a.Infra.Discovery != nil {
		// Touch registry so status stays warm; instances already registered at Open.
		_, _ = a.Infra.Discovery.Discover(ctx, "earnings-sync")
	}
}

func (a API) featActive(name string) bool {
	if a.Feat.Active(name) {
		return true
	}
	// Allow infra-only demos when flags omitted but stack is present and caller forces via env already resolved.
	return false
}

func (a API) actorOf(r *http.Request) string {
	if p, ok := auth.FromContext(r.Context()); ok {
		return p.Subject
	}
	return "anonymous"
}

func (a API) searchHandler(w http.ResponseWriter, r *http.Request) {
	if a.Infra == nil || !a.featActive("search") {
		writeErr(w, http.StatusServiceUnavailable, errStringError("search inactive (set FEATURE_SEARCH=1)"))
		return
	}
	q := r.URL.Query().Get("q")
	limit := 20
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	hits, err := a.Infra.Search.Query(r.Context(), q, limit)
	if err != nil {
		writeErr(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"hits":    hits,
		"backend": a.Infra.Status.Search.Backend,
		"q":       q,
	})
}

func (a API) auditHandler(w http.ResponseWriter, r *http.Request) {
	if a.Infra == nil || !a.featActive("audit") {
		writeErr(w, http.StatusServiceUnavailable, errStringError("audit inactive (set FEATURE_AUDIT=1)"))
		return
	}
	limit := 50
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	events, err := a.Infra.Audit.List(r.Context(), limit)
	if err != nil {
		writeErr(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"events":  events,
		"backend": a.Infra.Status.Audit.Backend,
	})
}

func (a API) platformStatus(w http.ResponseWriter, r *http.Request) {
	body := map[string]any{
		"api_version": APIContractVersion,
		"storage":     a.storagePayload(),
		"features":    a.Feat.Report,
		"active": map[string]bool{
			"redis":          a.featActive("redis"),
			"mq":             a.featActive("mq"),
			"kafka":          a.featActive("kafka"),
			"object_storage": a.featActive("object_storage"),
			"audit":          a.featActive("audit"),
			"search":         a.featActive("search"),
			"discovery":      a.featActive("discovery"),
			"elk":            a.featActive("elk"),
			"llm":            a.Feat.UseLLM(),
			"notify":         a.Feat.UseNotify(),
			"llm_live":       a.Feat.LiveLLM(),
			"notify_live":    a.Feat.LiveNotify(),
		},
	}
	if a.Infra != nil {
		body["infra"] = a.Infra.Status
		body["infra_summary"] = a.Infra.Summary()
		if a.Infra.Gateway != nil {
			body["gateway"] = a.Infra.Gateway.Stats()
		}
		if a.Infra.Kafka != nil {
			body["kafka_events"] = a.Infra.Kafka.Len()
			body["kafka_backend"] = a.Infra.Kafka.Backend()
		}
		if a.featActive("discovery") && a.Infra.Discovery != nil {
			svcs, _ := a.Infra.Discovery.ListServices(r.Context())
			inst, _ := a.Infra.Discovery.Discover(r.Context(), "earnings-sync")
			body["discovery"] = map[string]any{
				"backend":   a.Infra.Discovery.Backend(),
				"services":  svcs,
				"instances": inst,
			}
		}
		if a.Infra.Shipper != nil {
			body["shipper"] = a.Infra.Shipper.Status()
		}
		if len(a.Infra.AlertRules) > 0 {
			body["alert_rules"] = a.Infra.AlertRules
		}
	} else if a.featActive("kafka") && a.Feat.Bus != nil {
		body["kafka_events"] = a.Feat.Bus.Len()
	}
	if a.Commercial.ORM != nil {
		body["commercial"] = map[string]any{
			"orm_backend":     a.Commercial.ORM.Backend(),
			"tenants":         len(a.Commercial.Tenants.List()),
			"canary_releases": len(a.Commercial.Canary.List()),
			"lake_partitions": a.Commercial.Lakehouse.PartitionCount(),
			"sla_drills":      len(a.Commercial.SLA.ListDrills()),
		}
	}
	writeJSON(w, http.StatusOK, body)
}
