# Sheets + PostgreSQL 双写

## 模式

| `STORAGE_MODE` | 行为 |
| --- | --- |
| `sheets-only`（默认） | 只写 Google Sheets；无密钥时用 `sheet.Memory` |
| `pg-only` | 只写 PostgreSQL（需 `DATABASE_URL`） |
| `dual` | 先 Sheets 再 PG；需两侧都可用（无 Google 密钥时 Sheets=`Memory`） |

失败策略 `STORAGE_FAIL_POLICY`：

- `all`（默认）：任一侧失败立即返回错误（dual 下若 Sheets 先失败则不会写 PG）
- `partial`：两侧都尝试；任一侧失败仍返回聚合错误，但不中断另一侧

**事务边界：** PG 的 `UpsertAll` / `UpsertMacro` / `ApplyCharts` 各自在一个 SQL 事务内。Sheets 与 PG 之间没有分布式事务。

**幂等键：** `fact|{spreadsheet_id}|{TICKER}|{FY}|{FQ}|{line_item}`（见 `sheet.FactKey`），PG `ON CONFLICT` 更新。

## 本地起 PostgreSQL

```bash
cd deploy
docker compose up -d postgres
# 可选 Redis 占位：docker compose --profile cache up -d redis
```

连接串：

```bash
export DATABASE_URL='postgres://earnings:earnings@127.0.0.1:5432/earnings?sslmode=disable'
export MIGRATIONS_DIR=migrations   # 可选；默认从工作目录向上找 migrations/
export STORAGE_MODE=dual
export STORAGE_FAIL_POLICY=all
```

迁移在 `postgres.Open` 时自动执行 `migrations/*.sql`。

## 验收（无真实 Google 密钥）

```bash
# 单测：双写模式 / 失败策略（Memory+Memory，不需要 PG）
go test ./internal/sheet -count=1

# 有 PG 时：
docker compose -f deploy/docker-compose.yml up -d postgres
export DATABASE_URL='postgres://earnings:earnings@127.0.0.1:5432/earnings?sslmode=disable'
export STORAGE_MODE=dual
go test ./internal/store/postgres -count=1

# 跑服务 + fixture 写入 + 读回
go run ./cmd/server -conf trpc_go.yaml
curl -F ticker=AAPL -F force=true -F file=@testdata/aapl_2026q4.csv http://127.0.0.1:8090/v1/ingest
curl 'http://127.0.0.1:8090/v1/facts?ticker=AAPL'
```

前端：见 [apps/web/README.md](../apps/web/README.md)。Sheets + PG/API **并存验收清单**：[coexistence-acceptance.md](./coexistence-acceptance.md)。

编排 CLI 同样读 `STORAGE_MODE` / `DATABASE_URL`；默认仍是 Memory `sheets-only`。

## 任务 / 编排幂等

`DATABASE_URL` 同时启用 `jobs` / `flow_runs` / `product_events`（见 [jobs-runs-pg.md](./jobs-runs-pg.md)）。与 Sheets 事实双写独立：即使 `STORAGE_MODE=sheets-only`，只要配置了 `DATABASE_URL`，HTTP 服务与 `cmd/orchestrate` 仍会把 job/run 幂等写入 PG。

## 本轮不做

Kafka、K8s、完整 IAM、湖仓、CDN、蓝绿等——见 [platform-roadmap.md](platform-roadmap.md)。「集中式存储 / 跨实例幂等」✅ 已实现（有 `DATABASE_URL`；无 PG 降级单机文件 / Memory）。
