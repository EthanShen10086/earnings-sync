package httpapi

import (
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/EthanShen10086/earnings-sync/internal/canary"
	"github.com/EthanShen10086/earnings-sync/internal/hitl"
	"github.com/EthanShen10086/earnings-sync/internal/lakehouse"
	"github.com/EthanShen10086/earnings-sync/internal/orm"
	"github.com/EthanShen10086/earnings-sync/internal/sla"
	"github.com/EthanShen10086/earnings-sync/internal/tenant"
)

// Commercial is the optional multi-tenant / canary / HITL / lakehouse / SLA surface.
// Nil fields stay inactive; EnsureCommercial() fills in-process stubs.
type Commercial struct {
	Tenants   *tenant.Registry
	Canary    *canary.Controller
	HITL      *hitl.Ring
	Lakehouse *lakehouse.Store
	SLA       *sla.Board
	ORM       *orm.DB
}

// EnsureCommercial attaches in-process stubs when missing (no cluster required).
func (a *API) EnsureCommercial() {
	if a.Commercial.Tenants == nil {
		a.Commercial.Tenants = tenant.NewRegistry()
	}
	if a.Commercial.Canary == nil {
		a.Commercial.Canary = canary.NewController()
	}
	if a.Commercial.HITL == nil {
		a.Commercial.HITL = hitl.NewRing()
	}
	if a.Commercial.ORM == nil {
		a.Commercial.ORM = orm.Memory()
	}
	if a.Commercial.Lakehouse == nil {
		a.Commercial.Lakehouse = lakehouse.NewStore(a.Commercial.ORM)
	}
	if a.Commercial.SLA == nil {
		a.Commercial.SLA = sla.NewBoard()
	}
}

func (a API) registerCommercial(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/commercial/status", a.commercialStatus)
	mux.HandleFunc("GET /v1/tenants", a.listTenants)
	mux.HandleFunc("POST /v1/tenants", a.upsertTenant)
	mux.HandleFunc("POST /v1/canary/releases", a.canaryStart)
	mux.HandleFunc("POST /v1/canary/releases/{id}/canary", a.canarySet)
	mux.HandleFunc("POST /v1/canary/releases/{id}/promote", a.canaryPromote)
	mux.HandleFunc("POST /v1/canary/releases/{id}/rollback", a.canaryRollback)
	mux.HandleFunc("GET /v1/canary/releases/{id}", a.canaryGet)
	mux.HandleFunc("POST /v1/canary/releases/{id}/pick", a.canaryPick)
	mux.HandleFunc("POST /v1/hitl/ask", a.hitlAsk)
	mux.HandleFunc("POST /v1/hitl/{id}/decide", a.hitlDecide)
	mux.HandleFunc("GET /v1/hitl/{id}", a.hitlGet)
	mux.HandleFunc("POST /v1/lakehouse/facts", a.lakeAppend)
	mux.HandleFunc("POST /v1/lakehouse/exports", a.lakeExportStart)
	mux.HandleFunc("POST /v1/lakehouse/exports/{id}/run", a.lakeExportRun)
	mux.HandleFunc("GET /v1/lakehouse/exports/{id}", a.lakeExportGet)
	mux.HandleFunc("POST /v1/sla/drills", a.slaSchedule)
	mux.HandleFunc("POST /v1/sla/drills/{id}/run", a.slaRun)
	mux.HandleFunc("GET /v1/sla/drills", a.slaList)
	mux.HandleFunc("OPTIONS /v1/commercial/status", optionsOK)
}

func (a API) commercialStatus(w http.ResponseWriter, r *http.Request) {
	body := map[string]any{
		"orm_backend":      a.Commercial.ORM.Backend(),
		"tenants":          len(a.Commercial.Tenants.List()),
		"canary_releases":  len(a.Commercial.Canary.List()),
		"lake_partitions":  a.Commercial.Lakehouse.PartitionCount(),
		"sla_drills":       len(a.Commercial.SLA.ListDrills()),
		"sheets_pg_dual":   true,
		"cluster_required": false,
	}
	if t, ok := tenant.FromContext(r.Context()); ok {
		body["tenant"] = t
	}
	writeJSON(w, http.StatusOK, body)
}

func (a API) listTenants(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"tenants": a.Commercial.Tenants.List()})
}

func (a API) upsertTenant(w http.ResponseWriter, r *http.Request) {
	var body tenant.Tenant
	if err := readJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	body.Active = true
	if err := a.Commercial.Tenants.Upsert(body); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	got, _ := a.Commercial.Tenants.Get(body.ID)
	writeJSON(w, http.StatusOK, got)
}

func (a API) canaryStart(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID       string `json:"id"`
		FlowID   string `json:"flow_id"`
		Stable   string `json:"stable_version"`
		Canary   string `json:"canary_version"`
		TenantID string `json:"tenant_id"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	rel, err := a.Commercial.Canary.Start(body.ID, body.FlowID, body.Stable, body.Canary, body.TenantID)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, rel)
}

func (a API) canarySet(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Percent int `json:"percent"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	rel, err := a.Commercial.Canary.SetCanary(r.PathValue("id"), body.Percent)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, rel)
}

func (a API) canaryPromote(w http.ResponseWriter, r *http.Request) {
	rel, err := a.Commercial.Canary.Promote(r.PathValue("id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, rel)
}

func (a API) canaryRollback(w http.ResponseWriter, r *http.Request) {
	rel, err := a.Commercial.Canary.Rollback(r.PathValue("id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, rel)
}

func (a API) canaryGet(w http.ResponseWriter, r *http.Request) {
	rel, ok := a.Commercial.Canary.Get(r.PathValue("id"))
	if !ok {
		writeErr(w, http.StatusNotFound, errStringError("release not found"))
		return
	}
	writeJSON(w, http.StatusOK, rel)
}

func (a API) canaryPick(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Key string `json:"key"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	ver, bucket, err := a.Commercial.Canary.PickVersion(r.PathValue("id"), body.Key)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"version": ver, "bucket": bucket})
}

func (a API) hitlAsk(w http.ResponseWriter, r *http.Request) {
	var body hitl.Request
	if err := readJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	req, err := a.Commercial.HITL.Ask(body)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, req)
}

func (a API) hitlDecide(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Approve bool   `json:"approve"`
		Actor   string `json:"actor"`
		Reason  string `json:"reason"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if body.Actor == "" {
		body.Actor = a.actorOf(r)
	}
	req, err := a.Commercial.HITL.Decide(r.PathValue("id"), body.Actor, body.Approve, body.Reason)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, req)
}

func (a API) hitlGet(w http.ResponseWriter, r *http.Request) {
	req, ok := a.Commercial.HITL.Get(r.PathValue("id"))
	if !ok {
		writeErr(w, http.StatusNotFound, errStringError("intervention not found"))
		return
	}
	writeJSON(w, http.StatusOK, req)
}

func (a API) lakeAppend(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Facts []lakehouse.Fact `json:"facts"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if t, ok := tenant.FromContext(r.Context()); ok {
		for i := range body.Facts {
			if body.Facts[i].TenantID == "" {
				body.Facts[i].TenantID = t.ID
			}
		}
	}
	key, n, err := a.Commercial.Lakehouse.AppendFacts(r.Context(), body.Facts)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"partition": key, "rows": n})
}

func (a API) lakeExportStart(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID        string                 `json:"id"`
		Partition lakehouse.PartitionKey `json:"partition"`
		Sink      string                 `json:"sink"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	job, err := a.Commercial.Lakehouse.StartExport(body.ID, body.Partition, body.Sink)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, job)
}

func (a API) lakeExportRun(w http.ResponseWriter, r *http.Request) {
	job, err := a.Commercial.Lakehouse.RunExport(r.PathValue("id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, job)
}

func (a API) lakeExportGet(w http.ResponseWriter, r *http.Request) {
	job, ok := a.Commercial.Lakehouse.GetExport(r.PathValue("id"))
	if !ok {
		writeErr(w, http.StatusNotFound, errStringError("export not found"))
		return
	}
	writeJSON(w, http.StatusOK, job)
}

func (a API) slaSchedule(w http.ResponseWriter, r *http.Request) {
	var body sla.Drill
	if err := readJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	// Allow JSON to pass durations as nanoseconds via dedicated fields if needed;
	// default targets applied inside ScheduleDrill.
	d, err := a.Commercial.SLA.ScheduleDrill(body)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, d)
}

func (a API) slaRun(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ObservedMs int64 `json:"observed_ms"`
	}
	_ = readJSON(r, &body)
	observed := time.Duration(body.ObservedMs) * time.Millisecond
	d, err := a.Commercial.SLA.RunDrill(r.PathValue("id"), observed)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, d)
}

func (a API) slaList(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"drills": a.Commercial.SLA.ListDrills(),
		"checks": a.Commercial.SLA.RecentChecks(20),
	})
}

func readJSON(r *http.Request, dst any) error {
	defer r.Body.Close()
	dec := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	if err := dec.Decode(dst); err != nil {
		if err == io.EOF {
			return nil
		}
		return err
	}
	return nil
}
