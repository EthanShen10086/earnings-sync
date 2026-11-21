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
- 任务记录在 `data/jobs.json`（无 `DATABASE_URL`）或 PostgreSQL `jobs` 表；同一份文件再传一次不会重复写，也不会重复发。
- 结构化 CSV 里的多个财季会合成一次表格批量写入，不把这些数字再送给模型逐格抄写。说明在 [docs/connect.md](docs/connect.md)。

只想先看抽取结果、不碰表格和飞书时，才加 `-F dry_run=true`。

宏观数据走同一个入口，不进利润表。序列写在 `configs/macro.yaml`，`CN` 是中国月度指标。行是指标，列是 `202610` 这样的月份，分成累计同比、当期同比、环比三块。正数绿色，负数红色。PMI 是指数，其余按百分数显示，`4.6` 表示 4.6%。

```bash
go run ./cmd/sheetcreate -series CN
curl -F series=CN -F file=@testdata/cn_macro.csv http://127.0.0.1:8090/v1/ingest
```

长表列是 `period,basis,indicator,value`。宽表和汇总表一样：某一行以「累计同比」开头，后面跟月份，再往下是指标。两个都叫「扣除房地产开发投资」的行，上面一行是固投口径，下面一行是民间投资口径；长表里请写成 `fai_ex_property` 和 `private_ex_property`。同一份文件再传一次不会重复写。

历年同期的金额表是另一张工作表，不和月度增速混在一起。`CN` 里已经有「历年1-10月」：行是年份，列是固投、社零、房地产开发投资、商品房面积和销售额。最底下四行是公式，同比和较历年同期最高会随新年份重算。旁边两张柱状图从 2006 年起，商品房销售额那张的标题会写成距历史高点大约少了多少万亿。

```bash
curl -F series=CN -F file=@testdata/cn_jan_oct.csv http://127.0.0.1:8090/v1/ingest
```

以后要加「历年1-9月」或全年，在 `configs/macro.yaml` 的 `panels` 再写一块 `layout: level`，填 `window`、指标列、要保留的派生行和图。程序按窗口把文件送进对应的表，不会另写一套导入。宽表第一格写「历年1-9月」，长表则用 `year,window,indicator,value`。没有对应窗口时，导入会直接说要在配置里加哪一块。

新的一季写在同一张表的下一行，不新开文件，也不覆盖旧季度。启动服务时，已经配了 `spreadsheet_id` 的公司会自己去拉最新财报；这份文件如果已经成功写入过，不会再调用模型。黄色格子是录入，灰色公式和图会跟着新行重算。

一家公司的业务分部写在 `configs/companies.yaml` 的 `segments` 里，每项是 `key`、`column`、`header`。AWS 是 Amazon 利润表里的一列，不是另一张表。换 Google、Tesla 或别的公司，改的是这一段配置，不新增程序文件。下一季追加后，「各业务收入怎么变」会把每一列都画长。利润能留下多少、家底和债务、现金怎么来怎么走，三张表共用，不随公司改名。

新建一张三张报表：

```bash
go run ./cmd/sheetcreate -ticker MSFT
```

这一步要先做完 `docs/connect.md` 第 4 节。给出 `-ticker` 时，表头用这家公司的分部，并把 spreadsheet id 写回配置。服务账号建不了表时，用你自己的 Google 账号建一份同样的三张表，共享给服务账号后再把 id 写进配置。

## 双写存储（Sheets + PostgreSQL）

保留 Google Sheets，并增加 PostgreSQL Writer；ingest / 编排写入走同一 `sheet.Store` 抽象，可由环境变量选择：

- `STORAGE_MODE=sheets-only|pg-only|dual`
- `STORAGE_FAIL_POLICY=all|partial`
- `DATABASE_URL=postgres://earnings:earnings@127.0.0.1:5432/earnings?sslmode=disable`

```bash
make pg-up          # deploy/docker-compose.yml 起 Postgres（Redis 用 --profile cache）
make infra-up       # 可选：Redis + NATS；缺省时应用降级到 memory，不影响最小链路
make test           # 含双写与 infra 降级单测；CI 会带真实 PG
```

说明与验收步骤：[docs/dual-write.md](docs/dual-write.md)。Jobs/runs 跨进程幂等：[docs/jobs-runs-pg.md](docs/jobs-runs-pg.md)。数据面占位（缓存/MQ/对象存储/CDN/网关/审计/搜索/k8s）：[docs/infra.md](docs/infra.md)。最小只读页：[apps/web](apps/web)。平台愿景分期见 Project store `docs/platform-roadmap.md`。

## AI 编排引擎

可配置的本地 Flow Runtime（YAML DAG：tool / agent / condition）。财报 flow 通过 `pipeline_ingest` 调用真实 `pipeline.Ingest`。

- 架构 / 扩展（新 tool·agent·flow）/ 维护 / 故障排查：[docs/orchestrator.md](docs/orchestrator.md)
- 商用就绪评估（代码体量、可观测、闭环、差距）：见 Project 文档 `docs/commercial-readiness.md`（Agent Store）

```bash
make orchestrate-check # 健康检查：flow + 工具 + store 可写
make orchestrate       # fixture dry-run（真实解析，不写表）
make orchestrate-live  # fixture 非 dry：写入 Memory；导出 writes / runs / events
```

落盘（无 `DATABASE_URL` 时可查询）：

- `data/orchestrator-runs.json` — 每次 flow 的 RunState（含 request/trace id、步骤耗时）
- `data/orchestrator-events.json` — 产品闭环事件 `flow.start|success|fail`、`step.*`
- `data/orchestrator-e2e-writes.json` — live 写入证据（`-dump-writes`）
- `data/orchestrator-jobs.json` — pipeline job 幂等与分段耗时

设置 `DATABASE_URL` 后，上述 jobs/runs/events 改写 PostgreSQL（`jobs` / `flow_runs` / `product_events`），跨进程仍可按幂等键恢复。

## 其他入口

```bash
curl -H 'content-type: application/json' -d '{"ticker":"AAPL","url":"https://example.com/q4.html"}' http://127.0.0.1:8090/v1/ingest
curl -H 'content-type: application/json' -d '{"ticker":"AAPL"}' http://127.0.0.1:8090/v1/watch
curl -H 'content-type: application/json' -d '{"job_id":"<id>","instruction":"画营收饼图"}' http://127.0.0.1:8090/v1/charts/apply
```

公司在 `configs/companies.yaml`。Apple 用现有表格。Alphabet、Tesla 走 SEC，要先建表并填 spreadsheet id。SpaceX 没有公开 10-Q，只能手动传文件。Cursor 里让模型操作同一套工具时，启用 `.cursor/mcp.json` 的 `earnings-sync`，说明在 `skills/earnings-agent/SKILL.md`。

HTTP 在 `:8090`（含 `GET /v1/facts?ticker=AAPL`）。pprof 在 `127.0.0.1:8091/debug/pprof/`。指标在 `/metrics`。要看 trace：`docker compose -f deploy/docker-compose.yml --profile observe up -d`，再把 `OTEL_EXPORTER_OTLP_ENDPOINT` 设为 `http://localhost:4318`。

这次实际卡住的问题记在 [docs/questions-and-ux.md](docs/questions-and-ux.md)。
