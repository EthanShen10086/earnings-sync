package config

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/EthanShen10086/earnings-sync/internal/model"
	"gopkg.in/yaml.v3"
)

// Source is one place a company publishes filings.
type Source struct {
	Type string `yaml:"type"`
	CIK  string `yaml:"cik"`
	URL  string `yaml:"url"`
}

// Segment is one revenue or earnings line that differs by company. Apple uses iPhone; Amazon uses AWS.
type Segment struct {
	Key    string `yaml:"key"`
	Column string `yaml:"column"`
	Header string `yaml:"header"`
}

// Line is one yellow input cell. Sheet defaults to 利润表.
// A company that sets lines does not use Apple's column positions.
type Line struct {
	Key    string `yaml:"key"`
	Sheet  string `yaml:"sheet,omitempty"`
	Column string `yaml:"column"`
	Header string `yaml:"header,omitempty"`
}

// Company is a tracked issuer. SpreadsheetID may be empty until a template is copied.
type Company struct {
	Ticker             string    `yaml:"ticker"`
	Name               string    `yaml:"name"`
	FiscalYearEndMonth int       `yaml:"fiscal_year_end_month"`
	SpreadsheetID      string    `yaml:"spreadsheet_id"`
	Segments           []Segment `yaml:"segments,omitempty"`
	Lines              []Line    `yaml:"lines,omitempty"`
	SegmentTotal       string    `yaml:"segment_total,omitempty"`
	Prompt             string    `yaml:"prompt,omitempty"`
	Sources            []Source  `yaml:"sources"`
}

func (c Company) SheetURL() string {
	if c.SpreadsheetID == "" {
		return ""
	}
	return "https://docs.google.com/spreadsheets/d/" + c.SpreadsheetID
}

// UsesMappedLayout is true when yellow cells are listed explicitly.
// Those companies must not fall back to Apple's column positions.
func (c Company) UsesMappedLayout() bool {
	return len(c.Lines) > 0
}

// SegmentColumns maps a company-specific line onto a 利润表 column.
func (c Company) SegmentColumns() map[string]string {
	if len(c.Segments) == 0 {
		return nil
	}
	out := map[string]string{}
	for _, s := range c.Segments {
		if s.Key != "" && s.Column != "" {
			out[s.Key] = strings.ToUpper(s.Column)
		}
	}
	return out
}

// InputCells maps every yellow input, including other statements, as "表!列".
func (c Company) InputCells() map[string]string {
	if !c.UsesMappedLayout() {
		return nil
	}
	out := map[string]string{}
	for _, s := range c.Segments {
		if s.Key != "" && s.Column != "" {
			out[s.Key] = "利润表!" + strings.ToUpper(s.Column)
		}
	}
	for _, ln := range c.Lines {
		if ln.Key == "" || ln.Column == "" {
			continue
		}
		sheet := ln.Sheet
		if sheet == "" {
			sheet = "利润表"
		}
		out[ln.Key] = sheet + "!" + strings.ToUpper(ln.Column)
	}
	return out
}

// Bind attaches this company's columns to an extraction before reconcile or write.
func (c Company) Bind(ex *model.Extraction) {
	if ex == nil {
		return
	}
	if ex.Ticker == "" {
		ex.Ticker = c.Ticker
	}
	ex.Columns = c.SegmentColumns()
	ex.Cells = c.InputCells()
	ex.MappedOnly = c.UsesMappedLayout()
	ex.SegmentTotal = c.SegmentTotal
}

// AllowedLineItems is the extraction vocabulary for this company.
// Product lines belong to that company. Apple is the only one whose product columns are the default template.
func (c Company) AllowedLineItems() string {
	const common = "total_revenue, gross_margin, net_income, diluted_eps, cash, short_term_investments, receivables, inventory, other_current_assets, long_term_investments, ppe_net, other_noncurrent_assets, accounts_payable, short_term_debt, deferred_revenue, other_current_liabilities, long_term_debt, other_noncurrent_liabilities, equity, operating_cash_flow, capex, free_cash_flow, capital_return, dividend_per_share, depreciation, other_operating, other_investing, other_financing, note"
	if c.UsesMappedLayout() {
		var b strings.Builder
		seen := map[string]struct{}{}
		write := func(key, header string) {
			if key == "" {
				return
			}
			if _, ok := seen[key]; ok {
				return
			}
			seen[key] = struct{}{}
			if header != "" {
				fmt.Fprintf(&b, "%s（%s）, ", key, header)
				return
			}
			fmt.Fprintf(&b, "%s, ", key)
		}
		for _, s := range c.Segments {
			write(s.Key, s.Header)
		}
		for _, ln := range c.Lines {
			write(ln.Key, ln.Header)
		}
		return strings.TrimSuffix(b.String(), ", ")
	}
	if len(c.Segments) > 0 {
		var b strings.Builder
		for _, s := range c.Segments {
			fmt.Fprintf(&b, "%s（%s，这是业务分部，不是公司总营收）, ", s.Key, s.Header)
		}
		b.WriteString(common)
		return b.String()
	}
	if c.Ticker == "AAPL" {
		return "iphone_revenue, mac_revenue, ipad_revenue, wearables_revenue, services_revenue, iphone_units, iphone_yoy, mac_yoy, ipad_yoy, wearables_yoy, services_yoy, " + common
	}
	return common
}

// File is the on-disk company registry.
type File struct {
	Companies []Company `yaml:"companies"`
}

// Registry looks companies up by ticker.
type Registry struct {
	path     string
	byTicker map[string]Company
	list     []Company
}

func Load(path string) (*Registry, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var f File
	if err := yaml.Unmarshal(b, &f); err != nil {
		return nil, err
	}
	r := &Registry{path: path, byTicker: map[string]Company{}}
	seen := map[string]struct{}{}
	for _, c := range f.Companies {
		c.Ticker = strings.ToUpper(strings.TrimSpace(c.Ticker))
		if c.Ticker == "" {
			return nil, fmt.Errorf("company is missing ticker")
		}
		if _, ok := seen[c.Ticker]; ok {
			return nil, fmt.Errorf("duplicate ticker %s", c.Ticker)
		}
		if c.FiscalYearEndMonth < 1 || c.FiscalYearEndMonth > 12 {
			return nil, fmt.Errorf("%s fiscal_year_end_month must be 1-12", c.Ticker)
		}
		seen[c.Ticker] = struct{}{}
		r.byTicker[c.Ticker] = c
		r.list = append(r.list, c)
	}
	if len(r.list) == 0 {
		return nil, fmt.Errorf("no companies configured")
	}
	return r, nil
}

func (r *Registry) Get(ticker string) (Company, bool) {
	c, ok := r.byTicker[strings.ToUpper(ticker)]
	return c, ok
}

func (r *Registry) List() []Company {
	out := make([]Company, len(r.list))
	copy(out, r.list)
	return out
}

// SetSpreadsheet remembers a workbook created for this ticker and writes it back to the registry file.
func (r *Registry) SetSpreadsheet(ticker, id string) error {
	ticker = strings.ToUpper(strings.TrimSpace(ticker))
	c, ok := r.byTicker[ticker]
	if !ok {
		return fmt.Errorf("unknown ticker %s", ticker)
	}
	c.SpreadsheetID = id
	r.byTicker[ticker] = c
	for i := range r.list {
		if r.list[i].Ticker == ticker {
			r.list[i].SpreadsheetID = id
		}
	}
	if r.path == "" {
		return nil
	}
	b, err := os.ReadFile(r.path)
	if err != nil {
		return err
	}
	updated, err := replaceSpreadsheetID(string(b), ticker, id)
	if err != nil {
		return err
	}
	return os.WriteFile(r.path, []byte(updated), 0o644)
}

func replaceSpreadsheetID(src, ticker, id string) (string, error) {
	lines := strings.Split(src, "\n")
	in := false
	tickerLine := -1
	for i, line := range lines {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "- ticker:") {
			in = strings.TrimSpace(strings.TrimPrefix(trim, "- ticker:")) == ticker
			if in {
				tickerLine = i
			}
			continue
		}
		if in && strings.HasPrefix(trim, "spreadsheet_id:") {
			indent := line[:len(line)-len(strings.TrimLeft(line, " "))]
			lines[i] = indent + "spreadsheet_id: " + id
			return strings.Join(lines, "\n"), nil
		}
		if in && strings.HasPrefix(trim, "- ticker:") {
			break
		}
	}
	if tickerLine < 0 {
		return "", fmt.Errorf("ticker %s was not found in registry", ticker)
	}
	insert := "    spreadsheet_id: " + id
	out := append([]string{}, lines[:tickerLine+1]...)
	out = append(out, insert)
	out = append(out, lines[tickerLine+1:]...)
	return strings.Join(out, "\n"), nil
}

// Settings comes from the environment so secrets stay out of the repo.
type Settings struct {
	CompaniesFile   string
	JobStorePath    string
	ModelAPIKey     string
	ModelBaseURL    string
	ModelName       string
	FeishuWebhook   string
	FeishuSecret    string
	GoogleCreds     string
	GoogleShareWith string
	OTLPEndpoint    string
	SECUserAgent    string
}

func LoadSettings() Settings {
	LoadDotEnv(".env")
	return Settings{
		CompaniesFile:   env("COMPANIES_FILE", "configs/companies.yaml"),
		JobStorePath:    env("JOB_STORE_PATH", "data/jobs.json"),
		ModelAPIKey:     os.Getenv("DEEPSEEK_API_KEY"),
		ModelBaseURL:    env("DEEPSEEK_BASE_URL", "https://api.deepseek.com"),
		ModelName:       env("DEEPSEEK_MODEL", "deepseek-flash"),
		FeishuWebhook:   os.Getenv("FEISHU_WEBHOOK_URL"),
		FeishuSecret:    os.Getenv("FEISHU_WEBHOOK_SECRET"),
		GoogleCreds:     os.Getenv("GOOGLE_APPLICATION_CREDENTIALS"),
		GoogleShareWith: os.Getenv("GOOGLE_SHARE_WITH"),
		OTLPEndpoint:    os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"),
		SECUserAgent:    env("SEC_USER_AGENT", "earnings-sync/0.1 (research; set SEC_USER_AGENT)"),
	}
}

// LoadDotEnv reads KEY=VALUE lines. Existing environment variables win.
func LoadDotEnv(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		val = strings.Trim(strings.TrimSpace(val), `"'`)
		if key == "" || os.Getenv(key) != "" {
			continue
		}
		_ = os.Setenv(key, val)
	}
}

func env(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}
