package pipeline

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/EthanShen10086/earnings-sync/internal/acquire"
	"github.com/EthanShen10086/earnings-sync/internal/config"
	"github.com/EthanShen10086/earnings-sync/internal/extract"
	"github.com/EthanShen10086/earnings-sync/internal/job"
	"github.com/EthanShen10086/earnings-sync/internal/notify"
	"github.com/EthanShen10086/earnings-sync/internal/observe"
	"github.com/EthanShen10086/earnings-sync/internal/sheet"
)

type capture struct {
	n int
}

func (c *capture) Send(context.Context, notify.Message) error {
	c.n++
	return nil
}

func TestDryRunFixtureIsIdempotent(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("..", "..", "testdata", "aapl_2026q4.csv"))
	if err != nil {
		t.Fatal(err)
	}
	reg, err := config.Load(filepath.Join("..", "..", "configs", "companies.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	store, err := job.Open(filepath.Join(t.TempDir(), "jobs.json"))
	if err != nil {
		t.Fatal(err)
	}
	mem := &sheet.Memory{}
	note := &capture{}
	p := &Pipeline{
		Companies: reg,
		Jobs:      store,
		Extractor: extract.Extractor{},
		Sheets:    mem,
		Notifier:  note,
		Obs:       observe.New(),
	}
	req := Request{Ticker: "AAPL", Filename: "aapl_2026q4.csv", Body: body, DryRun: true}
	first, err := p.Ingest(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if first.Status != "succeeded" || first.DryRun != true || first.Notified {
		t.Fatalf("job %+v", first)
	}
	if first.Extraction == nil || first.Extraction.Period.Label() != "2026Q4" {
		t.Fatalf("extraction %+v", first.Extraction)
	}
	if len(first.Takeaway) == 0 || len(first.Charts) == 0 {
		t.Fatalf("takeaway %v charts %v", first.Takeaway, first.Charts)
	}
	if len(mem.Writes) != 0 || note.n != 0 {
		t.Fatal("dry run must not write or notify")
	}
	second, err := p.Ingest(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if second.ID != first.ID {
		t.Fatalf("expected same job %s %s", first.ID, second.ID)
	}
	got, err := p.Get(first.ID)
	if err != nil || got.DocHash == "" {
		t.Fatalf("get %+v %v", got, err)
	}
}

func TestLiveWriteAndNotifyOnce(t *testing.T) {
	body := []byte("fiscal_year,fiscal_quarter,line_item,value,unit\n2026,4,total_revenue,10,billion_usd\n2026,4,net_income,2,billion_usd\n")
	reg, err := config.Load(filepath.Join("..", "..", "configs", "companies.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	store, err := job.Open(filepath.Join(t.TempDir(), "jobs.json"))
	if err != nil {
		t.Fatal(err)
	}
	mem := &sheet.Memory{}
	note := &capture{}
	p := &Pipeline{
		Companies: reg,
		Jobs:      store,
		Sheets:    mem,
		Notifier:  note,
		Obs:       observe.New(),
	}
	req := Request{Ticker: "aapl", Filename: "mini.csv", Body: body}
	job, err := p.Ingest(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if job.Cells == 0 || !job.Notified || note.n != 1 || len(mem.Charts) == 0 {
		t.Fatalf("job %+v writes %d charts %d notes %d", job, len(mem.Writes), len(mem.Charts), note.n)
	}
	if _, err := p.Ingest(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	if note.n != 1 {
		t.Fatalf("notifier called %d", note.n)
	}
	_, err = p.Apply(context.Background(), job.ID, "饼图", false)
	if err == nil {
		t.Fatal("expected instruction miss")
	}
}

func TestDryRunDoesNotBlockRealWrite(t *testing.T) {
	body := []byte("fiscal_year,fiscal_quarter,line_item,value,unit\n2026,4,total_revenue,10,billion_usd\n2026,3,total_revenue,9,billion_usd\n")
	reg, err := config.Load(filepath.Join("..", "..", "configs", "companies.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	store, err := job.Open(filepath.Join(t.TempDir(), "jobs.json"))
	if err != nil {
		t.Fatal(err)
	}
	mem := &sheet.Memory{}
	note := &capture{}
	p := &Pipeline{Companies: reg, Jobs: store, Sheets: mem, Notifier: note, Obs: observe.New()}
	req := Request{Ticker: "AAPL", Filename: "two.csv", Body: body, DryRun: true}
	if _, err := p.Ingest(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	req.DryRun = false
	job, err := p.Ingest(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if job.DryRun || job.Cells != 2 || note.n != 1 || len(mem.Writes) != 2 {
		t.Fatalf("job %+v writes %d notes %d", job, len(mem.Writes), note.n)
	}
}

func TestWatchUsesLatestFilingOnce(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/submissions/") {
			_, _ = w.Write([]byte(`{"filings":{"recent":{
				"accessionNumber":["0000320193-26-000010"],
				"form":["10-Q"],
				"filingDate":["2026-08-01"],
				"reportDate":["2026-06-27"],
				"primaryDocument":["q.csv"]
			}}}`))
			return
		}
		_, _ = w.Write([]byte("fiscal_year,fiscal_quarter,line_item,value,unit\n2026,3,total_revenue,90,billion_usd\n"))
	}))
	defer srv.Close()
	old := acquire.ArchiveRoot
	acquire.ArchiveRoot = srv.URL
	defer func() { acquire.ArchiveRoot = old }()

	reg, err := config.Load(filepath.Join("..", "..", "configs", "companies.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	store, err := job.Open(filepath.Join(t.TempDir(), "jobs.json"))
	if err != nil {
		t.Fatal(err)
	}
	p := &Pipeline{
		Companies:    reg,
		Jobs:         store,
		Sheets:       &sheet.Memory{},
		Obs:          observe.New(),
		SECUserAgent: "earnings-sync-test",
		SECBaseURL:   srv.URL,
		Client:       srv.Client(),
	}
	first, err := p.Watch(context.Background(), "AAPL", true)
	if err != nil || len(first) != 1 || first[0].Extraction == nil || first[0].Extraction.Period.Label() != "2026Q3" {
		t.Fatalf("%v %+v", err, first)
	}
	second, err := p.Watch(context.Background(), "AAPL", true)
	if err != nil || len(second) != 1 || second[0].ID != first[0].ID {
		t.Fatalf("second %v %+v", err, second)
	}
	space, err := p.Watch(context.Background(), "SPACEX", true)
	if err != nil || len(space) != 0 {
		t.Fatalf("spacex %v %+v", err, space)
	}
}

type fixedModel struct {
	body string
}

func (f fixedModel) Name() string { return "fixed" }
func (f fixedModel) Complete(context.Context, string, string) (string, extract.TokenUsage, error) {
	return f.body, extract.TokenUsage{Total: 4}, nil
}

func TestFilingParseWritesBalanceSheet(t *testing.T) {
	reg, err := config.Load(filepath.Join("..", "..", "configs", "companies.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	store, err := job.Open(filepath.Join(t.TempDir(), "jobs.json"))
	if err != nil {
		t.Fatal(err)
	}
	mem := &sheet.Memory{}
	p := &Pipeline{
		Companies: reg,
		Jobs:      store,
		Sheets:    mem,
		Obs:       observe.New(),
		Extractor: extract.Extractor{Model: fixedModel{body: `{"periods":[{"fiscal_year":2026,"fiscal_quarter":4,"facts":[
			{"line_item":"total_revenue","value":114.48},
			{"line_item":"net_income","value":30.71},
			{"line_item":"cash","value":36.31},
			{"line_item":"receivables","value":43.82},
			{"line_item":"inventory","value":14.54},
			{"line_item":"accounts_payable","value":82.25},
			{"line_item":"long_term_debt","value":68.21},
			{"line_item":"equity","value":112.09}
		]}],"analysis":["负债表：现金 36 十亿美元，权益 112 十亿美元"]}`}},
		SECBaseURL: "http://example.test",
	}
	body := []byte(`<html><tr><td>Cash and cash equivalents</td><td>36,305</td></tr></html>`)
	job, err := p.Ingest(context.Background(), Request{Ticker: "AAPL", Filename: "ex991.htm", Body: body})
	if err != nil {
		t.Fatal(err)
	}
	cash, ok := job.Extraction.Number("cash")
	if !ok || cash < 36 || job.Cells == 0 || len(job.Takeaway) == 0 {
		t.Fatalf("job %+v", job)
	}
	found := false
	for _, w := range mem.Writes {
		if w.Sheet == "资产负债表" && w.Value == cash {
			found = true
		}
	}
	if !found {
		t.Fatalf("writes %+v", mem.Writes)
	}
}
