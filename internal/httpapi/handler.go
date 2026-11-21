package httpapi

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/EthanShen10086/earnings-sync/internal/auth"
	"github.com/EthanShen10086/earnings-sync/internal/infra"
	"github.com/EthanShen10086/earnings-sync/internal/model"
	"github.com/EthanShen10086/earnings-sync/internal/observe"
	"github.com/EthanShen10086/earnings-sync/internal/pipeline"
	"github.com/EthanShen10086/earnings-sync/internal/sheet"
)

// APIContractVersion is the stable JSON contract revision for /v1/*.
const APIContractVersion = "2026-11-21.1"

// API is the HTTP surface of the tRPC service.
type API struct {
	Pipe   *pipeline.Pipeline
	Obs    *observe.Obs
	Facts  sheet.Reader // optional; Postgres or Memory readback for apps/web
	Auth   auth.Config
	Alerts *observe.AlertBoard
	Infra  *infra.Stack // optional data-plane adapters (cache/mq/objects/audit/search)
}

func (a API) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", a.healthz)
	mux.Handle("GET /metrics", a.Obs.MetricsHandler())
	mux.HandleFunc("GET /v1/meta", a.meta)
	mux.HandleFunc("GET /v1/auth/me", a.authMe)
	mux.HandleFunc("GET /v1/auth/providers", a.authProviders)
	mux.HandleFunc("GET /v1/ops/alerts", a.listAlerts)
	mux.HandleFunc("OPTIONS /v1/ops/alerts", optionsOK)
	mux.HandleFunc("OPTIONS /v1/auth/me", optionsOK)
	mux.HandleFunc("OPTIONS /v1/auth/providers", optionsOK)
	mux.HandleFunc("OPTIONS /v1/meta", optionsOK)
	mux.HandleFunc("POST /v1/ingest", a.ingest)
	mux.HandleFunc("POST /v1/watch", a.watch)
	mux.HandleFunc("POST /v1/charts/recommend", a.recommend)
	mux.HandleFunc("POST /v1/charts/apply", a.apply)
	mux.HandleFunc("GET /v1/jobs/{id}", a.getJob)
	mux.HandleFunc("GET /v1/facts", a.listFacts)
	mux.HandleFunc("OPTIONS /v1/facts", optionsOK)
	h := auth.Middleware(a.authConfig(), mux)
	if a.Infra != nil && a.Infra.Gateway != nil {
		h = a.Infra.Gateway.Middleware(h)
	}
	return h
}

func (a API) healthz(w http.ResponseWriter, _ *http.Request) {
	body := map[string]any{"status": "ok"}
	if a.Infra != nil {
		body["infra"] = a.Infra.Status
		body["infra_degraded"] = a.Infra.Degraded()
	}
	writeJSON(w, http.StatusOK, body)
}

func (a API) authConfig() auth.Config {
	if a.Auth.Mode != "" {
		return a.Auth
	}
	return auth.LoadConfig()
}

func (a API) meta(w http.ResponseWriter, r *http.Request) {
	cfg := a.authConfig()
	writeJSON(w, http.StatusOK, map[string]any{
		"api_version":  APIContractVersion,
		"service":      "earnings-sync",
		"auth_mode":    cfg.Mode,
		"storage_hint": "STORAGE_MODE=sheets-only|pg-only|dual",
		"docs":         []string{"docs/api-contract.md", "docs/observability.md"},
		"endpoints": []map[string]string{
			{"method": "GET", "path": "/healthz", "auth": "public"},
			{"method": "GET", "path": "/metrics", "auth": "public"},
			{"method": "GET", "path": "/v1/meta", "auth": "public"},
			{"method": "GET", "path": "/v1/auth/providers", "auth": "public"},
			{"method": "GET", "path": "/v1/auth/me", "auth": "required"},
			{"method": "GET", "path": "/v1/facts", "auth": "required", "query": "ticker"},
			{"method": "GET", "path": "/v1/jobs/{id}", "auth": "required"},
			{"method": "POST", "path": "/v1/ingest", "auth": "required"},
			{"method": "GET", "path": "/v1/ops/alerts", "auth": "required"},
		},
		"capabilities": map[string]bool{
			"facts_read":      true,
			"ingest_write":    true,
			"auth_bypass":     cfg.Mode == auth.ModeBypass,
			"auth_bearer":     cfg.Mode == auth.ModeBearer,
			"auth_oidc_stub":  cfg.Mode == auth.ModeOIDC,
			"ops_alerts_stub": true,
			"otel_optional":   true,
		},
	})
}

func (a API) authMe(w http.ResponseWriter, r *http.Request) {
	p, ok := auth.FromContext(r.Context())
	if !ok {
		writeErr(w, http.StatusUnauthorized, errStringError("unauthenticated"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"principal": p})
}

func (a API) authProviders(w http.ResponseWriter, r *http.Request) {
	cfg := a.authConfig()
	writeJSON(w, http.StatusOK, map[string]any{
		"mode":      cfg.Mode,
		"providers": cfg.Providers(),
	})
}

func (a API) listAlerts(w http.ResponseWriter, r *http.Request) {
	board := a.Alerts
	if board == nil {
		board = observe.NewAlertBoard()
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"alerts": board.List(),
		"note":   "Local stub board; see deploy/alert-rules.yml and docs/observability.md",
	})
}

func (a API) listFacts(w http.ResponseWriter, r *http.Request) {
	reader := a.Facts
	if reader == nil && a.Pipe != nil {
		reader = sheet.ReaderFrom(a.Pipe.Sheets)
	}
	if reader == nil {
		writeErr(w, http.StatusServiceUnavailable, errStringError("fact reader unavailable (set STORAGE_MODE=pg-only|dual with DATABASE_URL, or use Memory store)"))
		return
	}
	rows, err := reader.ListFacts(r.Context(), r.URL.Query().Get("ticker"))
	if err != nil {
		writeErr(w, http.StatusBadGateway, err)
		return
	}
	if rows == nil {
		rows = []sheet.FactRow{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"facts": rows,
		"meta": map[string]any{
			"api_version": APIContractVersion,
			"count":       len(rows),
			"ticker":      r.URL.Query().Get("ticker"),
		},
	})
}

func optionsOK(w http.ResponseWriter, r *http.Request) {
	cors(w)
	w.WriteHeader(http.StatusNoContent)
}

func cors(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
}

type errStringError string

func (e errStringError) Error() string { return string(e) }

func (a API) ingest(w http.ResponseWriter, r *http.Request) {
	req, err := readIngest(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	job, err := a.Pipe.Ingest(r.Context(), req)
	if err != nil && job == nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	status := http.StatusOK
	if job != nil && job.Status == model.StatusFailed {
		status = http.StatusUnprocessableEntity
	}
	writeJSON(w, status, map[string]any{"job": job, "error": errString(err)})
}

func (a API) watch(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Ticker string `json:"ticker"`
		DryRun bool   `json:"dry_run"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil && err != io.EOF {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	jobs, err := a.Pipe.Watch(r.Context(), body.Ticker, body.DryRun)
	if err != nil {
		writeErr(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"jobs": jobs})
}

func (a API) recommend(w http.ResponseWriter, r *http.Request) {
	var body struct {
		JobID string `json:"job_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	charts, err := a.Pipe.Recommend(r.Context(), body.JobID)
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"charts": charts})
}

func (a API) apply(w http.ResponseWriter, r *http.Request) {
	var body struct {
		JobID       string `json:"job_id"`
		Instruction string `json:"instruction"`
		ApplyRules  bool   `json:"apply_rules"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	charts, err := a.Pipe.Apply(r.Context(), body.JobID, body.Instruction, body.ApplyRules)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"charts": charts})
}

func (a API) getJob(w http.ResponseWriter, r *http.Request) {
	job, err := a.Pipe.Get(r.PathValue("id"))
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"job": job})
}

func readIngest(r *http.Request) (pipeline.Request, error) {
	if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/") {
		if err := r.ParseMultipartForm(25 << 20); err != nil {
			return pipeline.Request{}, err
		}
		req := pipeline.Request{
			Ticker: r.FormValue("ticker"),
			Series: r.FormValue("series"),
			URL:    r.FormValue("url"),
			DryRun: r.FormValue("dry_run") == "true" || r.FormValue("dry_run") == "1",
			Force:  r.FormValue("force") == "true",
		}
		file, hdr, err := r.FormFile("file")
		if err == nil {
			defer file.Close()
			req.Body, err = io.ReadAll(io.LimitReader(file, 20<<20))
			if err != nil {
				return pipeline.Request{}, err
			}
			req.Filename = hdr.Filename
		}
		return req, nil
	}
	var body struct {
		Ticker string `json:"ticker"`
		Series string `json:"series"`
		URL    string `json:"url"`
		DryRun bool   `json:"dry_run"`
		Force  bool   `json:"force"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		return pipeline.Request{}, err
	}
	return pipeline.Request{Ticker: body.Ticker, Series: body.Series, URL: body.URL, DryRun: body.DryRun, Force: body.Force}, nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	cors(w)
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-API-Version", APIContractVersion)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
