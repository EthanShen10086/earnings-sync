package acquire

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/EthanShen10086/earnings-sync/internal/model"
)

// Filing is one SEC document worth ingesting.
type Filing struct {
	Form       string
	Accession  string
	ReportDate time.Time
	URL        string
	Period     model.Period
}

type submissions struct {
	CIK     string `json:"cik"`
	Filings struct {
		Recent struct {
			AccessionNumber []string `json:"accessionNumber"`
			Form            []string `json:"form"`
			FilingDate      []string `json:"filingDate"`
			ReportDate      []string `json:"reportDate"`
			PrimaryDocument []string `json:"primaryDocument"`
		} `json:"recent"`
	} `json:"filings"`
}

// LatestReport returns the newest 10-Q or 10-K for a CIK.
// baseURL defaults to https://data.sec.gov when empty.
func LatestReport(ctx context.Context, client *http.Client, baseURL, cik, userAgent string, fiscalYearEndMonth int) (Filing, bool, error) {
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	if baseURL == "" {
		baseURL = "https://data.sec.gov"
	}
	cik = strings.TrimLeft(cik, "0")
	if cik == "" {
		return Filing{}, false, fmt.Errorf("empty cik")
	}
	padded := cik
	for len(padded) < 10 {
		padded = "0" + padded
	}
	url := strings.TrimRight(baseURL, "/") + "/submissions/CIK" + padded + ".json"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return Filing{}, false, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json")
	res, err := client.Do(req)
	if err != nil {
		return Filing{}, false, err
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, 8<<20))
	if err != nil {
		return Filing{}, false, err
	}
	if res.StatusCode >= 300 {
		return Filing{}, false, fmt.Errorf("sec submissions: %s", res.Status)
	}
	return parseLatest(body, cik, fiscalYearEndMonth)
}

func parseLatest(body []byte, cik string, fiscalYearEndMonth int) (Filing, bool, error) {
	var sub submissions
	if err := json.Unmarshal(body, &sub); err != nil {
		return Filing{}, false, err
	}
	recent := sub.Filings.Recent
	n := len(recent.Form)
	for i := 0; i < n; i++ {
		form := recent.Form[i]
		if form != "10-Q" && form != "10-K" {
			continue
		}
		if i >= len(recent.AccessionNumber) || i >= len(recent.ReportDate) {
			continue
		}
		report, _ := time.Parse("2006-01-02", recent.ReportDate[i])
		acc := recent.AccessionNumber[i]
		doc := ""
		if i < len(recent.PrimaryDocument) {
			doc = recent.PrimaryDocument[i]
		}
		filingURL := secArchiveURL(cik, acc, doc)
		fy, fq := FiscalPeriod(report, fiscalYearEndMonth)
		if form == "10-K" {
			fq = 4
		}
		return Filing{
			Form:       form,
			Accession:  acc,
			ReportDate: report,
			URL:        filingURL,
			Period:     model.Period{FiscalYear: fy, FiscalQuarter: fq},
		}, true, nil
	}
	return Filing{}, false, nil
}

func secArchiveURL(cik, accession, primary string) string {
	acc := strings.ReplaceAll(accession, "-", "")
	return strings.TrimRight(ArchiveRoot, "/") + "/" + cik + "/" + acc + "/" + primary
}

// ArchiveRoot is the EDGAR archive prefix. Tests point it at a local server.
var ArchiveRoot = "https://www.sec.gov/Archives/edgar/data"

// FiscalPeriod maps a report date onto a fiscal year and quarter.
// fiscalYearEndMonth is the month the fiscal year closes (Apple is 9).
func FiscalPeriod(report time.Time, fiscalYearEndMonth int) (year, quarter int) {
	if report.IsZero() || fiscalYearEndMonth < 1 || fiscalYearEndMonth > 12 {
		return 0, 0
	}
	m := int(report.Month())
	start := fiscalYearEndMonth%12 + 1
	delta := (m - start + 12) % 12
	quarter = delta/3 + 1
	year = report.Year()
	if m > fiscalYearEndMonth {
		year++
	}
	return year, quarter
}
