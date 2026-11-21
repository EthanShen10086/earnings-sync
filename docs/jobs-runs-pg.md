# Jobs / Flow runs → PostgreSQL

编排与 pipeline 的幂等从单机 JSON（`jobs.json` / `orchestrator-runs.json`）迁到可选的 PostgreSQL，支持跨进程/实例恢复与查询。

## 选择规则

| 条件 | Jobs | Flow runs / product events |
| --- | --- | --- |
| `DATABASE_URL` 已设置（服务 / `cmd/orchestrate` 显式传入） | `job.PGStore` | `orchestrator.PGRunStore` |
| 未设置 | `job.FileStore`（`JOB_STORE_PATH`，默认 `data/jobs.json`） | `FileRunStore`（`-runs` / `-events`） |

单元测试**不会**因 CI 全局 `DATABASE_URL` 自动切到 PG：`job.OpenAuto` / `OpenPersistentRuns` 只看调用方传入的 `DatabaseURL` / `DB`。

## 表（`migrations/002_jobs_runs.sql`）

- `jobs` — 与 `model.Job` 对齐；成功态上 `doc_hash` / `locator` 唯一（部分唯一索引）
- `flow_runs` — 与 `RunState` 对齐；非空 `client_request_id` 唯一
- `product_events` — 产品闭环事件

迁移仍由 `postgres.Open` / `Migrate` 按文件名顺序执行。

## 接口

```go
// job.Store
Create / ByHash / ByLocator / Get / Save

// orchestrator.RunRegistry
Put / Get / ByClientRequestID / List
```

`pipeline.Pipeline.Jobs` 与 `Engine.Runs` 依赖上述接口，文件与 PG 可互换。

## 验收

```bash
# 无 PG：文件路径仍绿
make test

# 有 PG（本地或 CI service）
export DATABASE_URL='postgres://earnings:earnings@127.0.0.1:5432/earnings?sslmode=disable'
export MIGRATIONS_DIR=migrations
make pg-up
make pg-test

# 跨进程幂等：同一 client_request_id 复用 run_id
go run ./cmd/orchestrate -flow configs/flows/earnings-ingest.flow.yaml \
  -client-request-id demo-1 \
  -input ticker=AAPL -input dry_run=true \
  -input body_path=testdata/aapl_2026q4.csv -input force=true
# 再跑一次，stdout 中 run_id 相同
```

HTTP：`GET /v1/jobs/{id}` 在 `DATABASE_URL` 模式下从 PG 读回。
