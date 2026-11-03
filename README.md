# 财报同步

tRPC-Go 服务。财报从链接、文件或 SEC 进来，抽成利润表、资产负债表、现金流量表，写入对应公司的 Google Sheet，并给飞书发一条先讲重点的消息。

公司、模型和通知都可以换。Apple、Alphabet、Tesla 已写在 `configs/companies.yaml`。SpaceX 没有公开 10-Q，只能手动投喂。

## 跑起来

```bash
cp .env.example .env
# 填 DEEPSEEK_API_KEY、FEISHU_WEBHOOK_URL、GOOGLE_APPLICATION_CREDENTIALS

go test ./...
go run ./cmd/server -conf trpc_go.yaml
```

HTTP 在 `:8090`。tRPC admin（含 pprof）在 `127.0.0.1:8091`，例如 `/debug/pprof/`。

只抽取、不写表、不发飞书：

```bash
curl -s -F ticker=AAPL -F dry_run=true -F file=@testdata/aapl_2026q4.csv \
  http://127.0.0.1:8090/v1/ingest
```

正式写入：

```bash
curl -s -F ticker=AAPL -F file=@report.pdf http://127.0.0.1:8090/v1/ingest
curl -s -H 'content-type: application/json' -d '{"ticker":"AAPL","url":"https://example.com/q4.html"}' \
  http://127.0.0.1:8090/v1/ingest
curl -s -H 'content-type: application/json' -d '{"ticker":"AAPL"}' http://127.0.0.1:8090/v1/watch
curl -s http://127.0.0.1:8090/v1/jobs/<job_id>
curl -s -H 'content-type: application/json' -d '{"job_id":"<job_id>"}' \
  http://127.0.0.1:8090/v1/charts/recommend
curl -s -H 'content-type: application/json' -d '{"job_id":"<job_id>","instruction":"画营收饼图"}' \
  http://127.0.0.1:8090/v1/charts/apply
```

同一份文件不会重复写表，也不会重复发飞书。任务记录在 `data/jobs.json`，里面有每一段耗时和错误。

## 表格

沿用一行一个财季的三张表。服务只改黄色录入格，不改公式列。Apple 使用已有表格。新公司先复制那份模板，把 spreadsheet id 填进配置，并把表格共享给服务账号。

`图表口径` 仍在表里。规则会建议柱状图或饼图；模型可以额外提建议，但不会自动画进表，除非调用 `ApplyCharts`。

## 可观测

```bash
cd deploy && docker compose up -d
```

设置 `OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4318` 后，trace 进 Jaeger（http://localhost:16686）。`/metrics` 给 Prometheus。日志是 JSON，带 `trace_id`。

换模型时改 `DEEPSEEK_BASE_URL` 和 `DEEPSEEK_MODEL`，调用方不用改。
