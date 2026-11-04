package model

import "time"

const (
	StatusRunning   = "running"
	StatusSucceeded = "succeeded"
	StatusFailed    = "failed"

	StatementIncome   = "income"
	StatementBalance  = "balance"
	StatementCashflow = "cashflow"

	UnitBillionUSD   = "billion_usd"
	UnitMillionUSD   = "million_usd"
	UnitUSD          = "usd"
	UnitRatio        = "ratio"
	UnitPerShare     = "usd_per_share"
	UnitMillionUnits = "million_units"
	UnitText         = "text"

	ChartColumn = "COLUMN"
	ChartPie    = "PIE"
	SourceRule  = "rule"
	SourceModel = "model"
)

// Period is a fiscal quarter, not a calendar quarter.
type Period struct {
	FiscalYear    int
	FiscalQuarter int
}

func (p Period) Label() string {
	if p.FiscalYear == 0 || p.FiscalQuarter == 0 {
		return ""
	}
	return itoa(p.FiscalYear) + "Q" + itoa(p.FiscalQuarter)
}

func (p Period) Valid() bool {
	return p.FiscalYear >= 1990 && p.FiscalYear <= 2100 && p.FiscalQuarter >= 1 && p.FiscalQuarter <= 4
}

// Fact is one reported number or note for a period.
type Fact struct {
	Statement string
	LineItem  string
	Value     *float64
	Text      string
	Unit      string
}

// Extraction is the normalized three-statement snapshot.
type Extraction struct {
	Ticker string
	Period Period
	Facts  []Fact
	Unit   string
	// Columns overrides the default sheet column for a line item. It is not stored.
	Columns map[string]string `json:"-"`
}

func (e Extraction) Fact(item string) (Fact, bool) {
	for _, f := range e.Facts {
		if f.LineItem == item {
			return f, true
		}
	}
	return Fact{}, false
}

func (e Extraction) Number(item string) (float64, bool) {
	f, ok := e.Fact(item)
	if !ok || f.Value == nil {
		return 0, false
	}
	return *f.Value, true
}

// Stage is one pipeline step, kept so a job can be debugged after the fact.
type Stage struct {
	Name       string `json:"name"`
	DurationMS int64  `json:"duration_ms"`
	Error      string `json:"error,omitempty"`
}

// ChartSpec is a recommendation or a chart to create.
type ChartSpec struct {
	Title     string `json:"title"`
	ChartType string `json:"chart_type"`
	Reason    string `json:"reason"`
	Source    string `json:"source"`
	Sheet     string `json:"sheet,omitempty"`
}

// Job is the durable record of one document.
type Job struct {
	ID         string      `json:"id"`
	Ticker     string      `json:"ticker"`
	Status     string      `json:"status"`
	DocHash    string      `json:"doc_hash"`
	Locator    string      `json:"locator,omitempty"`
	Source     string      `json:"source,omitempty"`
	DryRun     bool        `json:"dry_run"`
	Stages     []Stage     `json:"stages,omitempty"`
	Actions    []string    `json:"actions,omitempty"`
	Extraction *Extraction `json:"extraction,omitempty"`
	Warnings   []string    `json:"warnings,omitempty"`
	Takeaway   []string    `json:"takeaway,omitempty"`
	Charts     []ChartSpec `json:"charts,omitempty"`
	SheetURL   string      `json:"sheet_url,omitempty"`
	Notified   bool        `json:"notified"`
	Cells      int         `json:"cells_written,omitempty"`
	Tokens     int         `json:"model_tokens,omitempty"`
	Error      string      `json:"error,omitempty"`
	CreatedAt  time.Time   `json:"created_at"`
	UpdatedAt  time.Time   `json:"updated_at"`
}

// Document is the bytes acquired from a URL, file, or filing.
type Document struct {
	Name     string
	MIME     string
	Body     []byte
	Text     string
	Source   string
	Locator  string
	Filename string
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [12]byte
	i := len(b)
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
