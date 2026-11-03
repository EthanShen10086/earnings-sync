package live

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

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
	body, err := os.ReadFile(filepath.Join(root, "testdata", "aapl_2026q4.csv"))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	job, err := pipe.Ingest(ctx, pipeline.Request{
		Ticker:   "AAPL",
		Filename: "aapl_2026q4.csv",
		Body:     body,
	})
	if err != nil {
		t.Fatal(err)
	}
	if job.Status != "succeeded" || job.DryRun || !job.Notified || job.Cells < 15 {
		t.Fatalf("status=%s dry=%v cells=%d notified=%v err=%s", job.Status, job.DryRun, job.Cells, job.Notified, job.Error)
	}
	if job.Extraction == nil || job.Extraction.Period.Label() != "2026Q4" {
		t.Fatalf("period %+v", job.Extraction)
	}
	g, err := sheet.NewGoogle(ctx, settings.GoogleCreds)
	if err != nil {
		t.Fatal(err)
	}
	values, err := g.Values(ctx, "1ePLqJ8emZAZ-cYp90w5TvA8ptAxjLFr2CcrL9zQWEE0", "利润表!A2:J80")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, row := range values {
		year, yok := sheet.AsInt(row, 0)
		quarter, qok := sheet.AsInt(row, 1)
		if !yok || !qok || year != 2026 || quarter != 4 || len(row) < 10 {
			continue
		}
		rev, okNum := row[9].(float64)
		if !okNum {
			if s, ok := row[9].(string); ok {
				if n, err := strconv.ParseFloat(s, 64); err == nil {
					rev, okNum = n, true
				}
			}
		}
		if !okNum || rev < 114.47 || rev > 114.49 {
			t.Fatalf("2026Q4 revenue cell %#v", row[9])
		}
		found = true
	}
	if !found {
		t.Fatal("2026Q4 row was not written")
	}
	t.Logf("2026Q4 cells=%d notified=%v", job.Cells, job.Notified)
}
