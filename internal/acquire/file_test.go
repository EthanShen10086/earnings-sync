package acquire

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/xuri/excelize/v2"
)

func TestCSVAndXLSXText(t *testing.T) {
	doc, err := ReadFile("aapl.csv", []byte("fiscal_year,fiscal_quarter\n2026,4\n"))
	if err != nil || !strings.Contains(doc.Text, "2026") {
		t.Fatalf("csv %+v %v", doc, err)
	}
	f := excelize.NewFile()
	_ = f.SetCellValue("Sheet1", "A1", "total_revenue")
	_ = f.SetCellValue("Sheet1", "B1", "114.48")
	buf, err := f.WriteToBuffer()
	if err != nil {
		t.Fatal(err)
	}
	doc, err = ReadFile("book.xlsx", buf.Bytes())
	if err != nil || !strings.Contains(doc.Text, "114.48") {
		t.Fatalf("xlsx %v %s", err, doc.Text)
	}
}

func TestFetchURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") == "" {
			t.Error("missing user agent")
		}
		_, _ = w.Write([]byte("total_revenue 114.48"))
	}))
	defer srv.Close()
	doc, err := FetchURL(context.Background(), srv.Client(), srv.URL+"/report.txt", "earnings-sync-test")
	if err != nil || !strings.Contains(doc.Text, "114.48") {
		t.Fatalf("%v %s", err, doc.Text)
	}
}

func TestFiscalPeriod(t *testing.T) {
	sep := time.Date(2025, 9, 28, 0, 0, 0, 0, time.UTC)
	y, q := FiscalPeriod(sep, 9)
	if y != 2025 || q != 4 {
		t.Fatalf("sep got %d Q%d", y, q)
	}
	dec := time.Date(2025, 12, 28, 0, 0, 0, 0, time.UTC)
	y, q = FiscalPeriod(dec, 9)
	if y != 2026 || q != 1 {
		t.Fatalf("dec got %d Q%d", y, q)
	}
	cal := time.Date(2026, 3, 31, 0, 0, 0, 0, time.UTC)
	y, q = FiscalPeriod(cal, 12)
	if y != 2026 || q != 1 {
		t.Fatalf("calendar got %d Q%d", y, q)
	}
}

func TestLatestReport(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/submissions/CIK") {
			t.Errorf("path %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{
		  "cik":"320193",
		  "filings":{"recent":{
		    "accessionNumber":["0000320193-26-000001","0000320193-26-000002"],
		    "form":["8-K","10-Q"],
		    "filingDate":["2026-07-30","2026-08-01"],
		    "reportDate":["2026-07-30","2026-06-27"],
		    "primaryDocument":["a.htm","q.htm"]
		  }}
		}`))
	}))
	defer srv.Close()
	f, ok, err := LatestReport(context.Background(), srv.Client(), srv.URL, "0000320193", "earnings-sync-test", 9)
	if err != nil || !ok || f.Form != "10-Q" {
		t.Fatalf("%+v %v %v", f, ok, err)
	}
	if !strings.Contains(f.URL, "q.htm") || f.Period.FiscalQuarter == 0 {
		t.Fatalf("filing incomplete %+v", f)
	}
}

func TestEarningsReleaseBeatsOlder10Q(t *testing.T) {
	body := []byte(`{"filings":{"recent":{
		"accessionNumber":["0000320193-26-000024","0000320193-26-000020"],
		"form":["8-K","10-Q"],
		"filingDate":["2026-11-02","2026-07-31"],
		"reportDate":["2026-11-02","2026-06-27"],
		"primaryDocument":["cover.htm","q.htm"],
		"items":["2.02,9.01",""]
	}}}`)
	f, ok, err := parseFinancials(body, "320193", 9)
	if err != nil || !ok || f.Form != "8-K" || f.Period.Valid() {
		t.Fatalf("%+v %v %v", f, ok, err)
	}
	name, err := pickExhibit([]byte(`{"directory":{"item":[
		{"name":"aapl-20261102.htm","size":"100"},
		{"name":"a8-kex991q42026.htm","size":"172036"}
	]}}`))
	if err != nil || name != "a8-kex991q42026.htm" {
		t.Fatalf("%s %v", name, err)
	}
}

func TestStatementTextKeepsRows(t *testing.T) {
	html := `<html><body><p>Apple reports fourth quarter results</p><table><tr><td>Cash and cash equivalents</td><td>36,305</td></tr></table></body></html>`
	text := StatementText(html)
	if !strings.Contains(text, "Cash and cash equivalents | 36,305") || !strings.Contains(text, "fourth quarter") {
		t.Fatal(text)
	}
}
