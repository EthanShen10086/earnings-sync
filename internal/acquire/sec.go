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
			Items           []string `json:"items"`
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
	req.Header.Set("Accept", "application/json,text/html")
	req.Header.Set("Accept-Encoding", "identity")
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

// LatestFinancials prefers an Item 2.02 earnings release when it is newer than the latest 10-Q or 10-K.
// The 8-K cover page is not the report, so the caller still resolves the exhibit.
func LatestFinancials(ctx context.Context, client *http.Client, baseURL, cik, userAgent string, fiscalYearEndMonth int) (Filing, bool, error) {
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
	req.Header.Set("Accept", "application/json,text/html")
	req.Header.Set("Accept-Encoding", "identity")
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
	return parseFinancials(body, cik, fiscalYearEndMonth)
}

// ExhibitURL finds the earnings exhibit inside an 8-K directory. The cover page itself has no statements.
func ExhibitURL(ctx context.Context, client *http.Client, cik, accession, userAgent string) (string, error) {
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	cik = strings.TrimLeft(cik, "0")
	acc := strings.ReplaceAll(accession, "-", "")
	url := strings.TrimRight(ArchiveRoot, "/") + "/" + cik + "/" + acc + "/index.json"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json,text/html")
	req.Header.Set("Accept-Encoding", "identity")
	res, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	var body []byte
	if res.StatusCode == http.StatusForbidden {
		body, err = secGet(ctx, url, userAgent)
	} else if res.StatusCode >= 300 {
		return "", fmt.Errorf("sec index %s: %s", url, res.Status)
	} else {
		body, err = io.ReadAll(io.LimitReader(res.Body, 2<<20))
	}
	if err != nil {
		return "", err
	}
	name, err := pickExhibit(body)
	if err != nil {
		return "", err
	}
	return secArchiveURL(cik, accession, name), nil
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

func parseFinancials(body []byte, cik string, fiscalYearEndMonth int) (Filing, bool, error) {
	var sub submissions
	if err := json.Unmarshal(body, &sub); err != nil {
		return Filing{}, false, err
	}
	recent := sub.Filings.Recent
	var periodic Filing
	var earnings Filing
	hasPeriodic := false
	hasEarnings := false
	for i := 0; i < len(recent.Form); i++ {
		form := recent.Form[i]
		if i >= len(recent.AccessionNumber) || i >= len(recent.ReportDate) {
			continue
		}
		report, _ := time.Parse("2006-01-02", recent.ReportDate[i])
		doc := ""
		if i < len(recent.PrimaryDocument) {
			doc = recent.PrimaryDocument[i]
		}
		item := ""
		if i < len(recent.Items) {
			item = recent.Items[i]
		}
		filing := Filing{
			Form:       form,
			Accession:  recent.AccessionNumber[i],
			ReportDate: report,
			URL:        secArchiveURL(cik, recent.AccessionNumber[i], doc),
		}
		switch form {
		case "10-Q", "10-K":
			if hasPeriodic {
				continue
			}
			fy, fq := FiscalPeriod(report, fiscalYearEndMonth)
			if form == "10-K" {
				fq = 4
			}
			filing.Period = model.Period{FiscalYear: fy, FiscalQuarter: fq}
			periodic = filing
			hasPeriodic = true
		case "8-K":
			if hasEarnings || !strings.Contains(item, "2.02") {
				continue
			}
			earnings = filing
			hasEarnings = true
		}
		if hasPeriodic && hasEarnings {
			break
		}
	}
	if hasEarnings && (!hasPeriodic || !earnings.ReportDate.Before(periodic.ReportDate)) {
		return earnings, true, nil
	}
	if hasPeriodic {
		return periodic, true, nil
	}
	return Filing{}, false, nil
}

func pickExhibit(body []byte) (string, error) {
	var index struct {
		Directory struct {
			Item []struct {
				Name string `json:"name"`
				Size string `json:"size"`
			} `json:"item"`
		} `json:"directory"`
	}
	if err := json.Unmarshal(body, &index); err != nil {
		return "", err
	}
	best := ""
	bestSize := -1
	for _, item := range index.Directory.Item {
		name := strings.ToLower(item.Name)
		if !strings.HasSuffix(name, ".htm") && !strings.HasSuffix(name, ".html") {
			continue
		}
		if !strings.Contains(name, "ex99") && !strings.Contains(name, "ex-99") && !strings.Contains(name, "991") {
			continue
		}
		size := 0
		for _, r := range item.Size {
			if r < '0' || r > '9' {
				continue
			}
			size = size*10 + int(r-'0')
		}
		if size >= bestSize {
			best = item.Name
			bestSize = size
		}
	}
	if best == "" {
		return "", fmt.Errorf("earnings exhibit not found")
	}
	return best, nil
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
