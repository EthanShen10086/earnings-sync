# 财报同步

tRPC-Go 服务。财报进来后，DeepSeek 把数字写入 Google Sheet 的黄色录入格，并给飞书发一条先讲重点的消息。

## 先看这份指引

接通飞书、DeepSeek、已有表格，以及新建表格时要启用的 Google Drive API 和 `GOOGLE_SHARE_WITH`，都按 [docs/connect.md](docs/connect.md) 做。

在项目根目录反复运行：

```bash
go run ./cmd/doctor
```

没通过的那一项会写出要打开的网页，以及 `.env` 里要改的那一行。不要把填好的内容发到对话里。四项都显示「通过」后再导入。

## 导入一次

```bash
go run ./cmd/server -conf trpc_go.yaml
curl -F ticker=AAPL -F file=@testdata/aapl_2026q4.csv http://127.0.0.1:8090/v1/ingest
```

不要加 `dry_run`。成功时：

- 2026Q4 那一行的黄色格子被写上数，灰色公式列自己重算，旁边的图跟着变。
- 飞书群收到要点、勾稽差额和表格链接。
- `data/jobs.json` 里能看到每一段耗时。同一份文件再传一次不会重复写，也不会重复发。
- 结构化 CSV 里的多个财季会合成一次表格批量写入，不把这些数字再送给模型逐格抄写。说明在 [docs/connect.md](docs/connect.md)。

只想先看抽取结果、不碰表格和飞书时，才加 `-F dry_run=true`。

新建一张三张报表：

```bash
go run ./cmd/sheetcreate -title "公司名 财务报表"
```

这一步要先做完 `docs/connect.md` 第 4 节。终端打印链接后，把其中的 spreadsheet id 写进 `configs/companies.yaml`。

## 其他入口

```bash
curl -H 'content-type: application/json' -d '{"ticker":"AAPL","url":"https://example.com/q4.html"}' http://127.0.0.1:8090/v1/ingest
curl -H 'content-type: application/json' -d '{"ticker":"AAPL"}' http://127.0.0.1:8090/v1/watch
curl -H 'content-type: application/json' -d '{"job_id":"<id>","instruction":"画营收饼图"}' http://127.0.0.1:8090/v1/charts/apply
```

公司在 `configs/companies.yaml`。Apple 用现有表格。Alphabet、Tesla 走 SEC，要先建表并填 spreadsheet id。SpaceX 没有公开 10-Q，只能手动传文件。Cursor 里让模型操作同一套工具时，启用 `.cursor/mcp.json` 的 `earnings-sync`，说明在 `skills/earnings-agent/SKILL.md`。

HTTP 在 `:8090`。pprof 在 `127.0.0.1:8091/debug/pprof/`。指标在 `/metrics`。要看 trace，先 `cd deploy && docker compose up -d`，再把 `OTEL_EXPORTER_OTLP_ENDPOINT` 设为 `http://localhost:4318`。

这次实际卡住的问题记在 [docs/questions-and-ux.md](docs/questions-and-ux.md)。
