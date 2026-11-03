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
