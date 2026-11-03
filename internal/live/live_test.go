package live

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/EthanShen10086/earnings-sync/internal/acquire"
	"github.com/EthanShen10086/earnings-sync/internal/app"
	"github.com/EthanShen10086/earnings-sync/internal/config"
	"github.com/EthanShen10086/earnings-sync/internal/notify"
	"github.com/EthanShen10086/earnings-sync/internal/pipeline"
	"github.com/EthanShen10086/earnings-sync/internal/sheet"
)

func TestLiveFeishu(t *testing.T) {
	if os.Getenv("EARNINGS_LIVE") == "" {
		t.Skip("设置 EARNINGS_LIVE=1 才向真实飞书发消息")
	}
	config.LoadDotEnv(filepath.Join("..", "..", ".env"))
	webhook := os.Getenv("FEISHU_WEBHOOK_URL")
	if webhook == "" {
		t.Fatal("FEISHU_WEBHOOK_URL 为空。按 go run ./cmd/doctor 的飞书步骤填写 .env")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	err := (notify.Feishu{Webhook: webhook, Secret: os.Getenv("FEISHU_WEBHOOK_SECRET")}).Send(ctx, notify.Message{
		Ticker:   "AAPL",
		Period:   "2026Q4",
		Takeaway: []string{"这是 earnings-sync 的真实通知测试，确认写表之外的消息通道可用。"},
		Warnings: []string{"分部合计与总营收相差 0.01 十亿美元"},
		SheetURL: "https://docs.google.com/spreadsheets/d/1ePLqJ8emZAZ-cYp90w5TvA8ptAxjLFr2CcrL9zQWEE0",
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestLiveSheetRead(t *testing.T) {
	if os.Getenv("EARNINGS_LIVE") == "" {
		t.Skip("设置 EARNINGS_LIVE=1 才访问真实表格")
	}
	config.LoadDotEnv(filepath.Join("..", "..", ".env"))
	creds := os.Getenv("GOOGLE_APPLICATION_CREDENTIALS")
	if creds == "" {
		t.Fatal("GOOGLE_APPLICATION_CREDENTIALS 为空。按 go run ./cmd/doctor 的 Google 步骤创建服务账号，并把表格共享给 client_email")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	g, err := sheet.NewGoogle(ctx, creds)
	if err != nil {
		t.Fatal(err)
	}
	if err := g.Ping(ctx, "1ePLqJ8emZAZ-cYp90w5TvA8ptAxjLFr2CcrL9zQWEE0"); err != nil {
		t.Fatal(err)
	}
}

func TestLiveApple2026Q4(t *testing.T) {
	if os.Getenv("EARNINGS_LIVE") == "" {
		t.Skip("设置 EARNINGS_LIVE=1 才写入真实表格并发送飞书")
	}
	config.LoadDotEnv(filepath.Join("..", "..", ".env"))
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("COMPANIES_FILE", filepath.Join(root, "configs", "companies.yaml"))
	t.Setenv("JOB_STORE_PATH", filepath.Join(root, "data", "jobs.json"))
	settings := config.LoadSettings()
	pipe, _, err := app.Build(settings)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 170*time.Second)
	defer cancel()
	filing, ok, err := acquire.LatestFinancials(ctx, nil, "", "0000320193", settings.SECUserAgent, 9)
	if err != nil || !ok || filing.Form != "8-K" {
		t.Fatalf("latest filing %+v %v %v", filing, ok, err)
	}
	exhibit, err := acquire.ExhibitURL(ctx, nil, "0000320193", filing.Accession, settings.SECUserAgent)
	if err != nil {
		t.Fatal(err)
	}
	job, err := pipe.Ingest(ctx, pipeline.Request{Ticker: "AAPL", URL: exhibit, Force: true})
	if err != nil {
		t.Fatal(err)
	}
	if job.Status != "succeeded" || job.DryRun || !job.Notified || job.Extraction == nil || job.Extraction.Period.Label() != "2026Q4" {
		t.Fatalf("status=%s dry=%v period=%v notified=%v err=%s", job.Status, job.DryRun, job.Extraction, job.Notified, job.Error)
	}
	cash, _ := job.Extraction.Number("cash")
	equity, _ := job.Extraction.Number("equity")
	rev, _ := job.Extraction.Number("total_revenue")
	if cash < 36 || cash > 37 || equity < 110 || equity > 114 || rev < 114 || rev > 115 {
		t.Fatalf("cash %v equity %v revenue %v", cash, equity, rev)
	}
	if len(job.Takeaway) < 4 {
		t.Fatalf("analysis %v", job.Takeaway)
	}
	g, err := sheet.NewGoogle(ctx, settings.GoogleCreds)
	if err != nil {
		t.Fatal(err)
	}
	values, err := g.Values(ctx, "1ePLqJ8emZAZ-cYp90w5TvA8ptAxjLFr2CcrL9zQWEE0", "资产负债表!A2:X80")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, row := range values {
		year, yok := sheet.AsInt(row, 0)
		quarter, qok := sheet.AsInt(row, 1)
		if !yok || !qok || year != 2026 || quarter != 4 || len(row) < 24 {
			continue
		}
		got := number(row[3])
		if got < 36 || got > 37 {
			t.Fatalf("balance cash %#v", row[3])
		}
		found = true
	}
	if !found {
		t.Fatal("2026Q4 balance sheet row was not written")
	}
	t.Logf("2026Q4 cells=%d cash=%.2f equity=%.2f", job.Cells, cash, equity)
	for _, line := range job.Takeaway {
		t.Log(line)
	}
}

func TestLiveAnalysisCharts(t *testing.T) {
	if os.Getenv("EARNINGS_LIVE") == "" {
		t.Skip("设置 EARNINGS_LIVE=1 才把分析图写进真实表格")
	}
	config.LoadDotEnv(filepath.Join("..", "..", ".env"))
	settings := config.LoadSettings()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	g, err := sheet.NewGoogle(ctx, settings.GoogleCreds)
	if err != nil {
		t.Fatal(err)
	}
	n, err := g.ApplyCharts(ctx, "1ePLqJ8emZAZ-cYp90w5TvA8ptAxjLFr2CcrL9zQWEE0", nil)
	if err != nil {
		t.Fatal(err)
	}
	hardware := number(mustCell(t, g, ctx, "利润表!B121"))
	cashSec := number(mustCell(t, g, ctx, "资产负债表!B121"))
	ocf := number(mustCell(t, g, ctx, "现金流量表!B121"))
	if hardware < 80 || hardware > 85 || cashSec < 130 || cashSec > 145 || ocf < 27 || ocf > 30 {
		t.Fatalf("hardware %.2f cash+securities %.2f ocf %.2f charts_added %d", hardware, cashSec, ocf, n)
	}
	t.Logf("charts_added=%d hardware=%.2f cash_securities=%.2f ocf=%.2f", n, hardware, cashSec, ocf)
}

func mustCell(t *testing.T, g *sheet.Google, ctx context.Context, a1 string) any {
	t.Helper()
	vals, err := g.Values(ctx, "1ePLqJ8emZAZ-cYp90w5TvA8ptAxjLFr2CcrL9zQWEE0", a1)
	if err != nil || len(vals) == 0 || len(vals[0]) == 0 {
		t.Fatalf("%s %v", a1, err)
	}
	return vals[0][0]
}

func number(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case string:
		f, _ := strconv.ParseFloat(n, 64)
		return f
	default:
		return 0
	}
}
