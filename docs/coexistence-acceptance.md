# Sheets 旧链路与 PG/API 展示并存 — 验收

目标：保留 Google Sheets（或 Memory）写入路径的同时，PostgreSQL / HTTP API / Web·移动端只读展示可同时验收，互不互斥。

## 前置

```bash
make pg-up   # 可选但推荐；无 PG 时 dual 软降级 sheets-only(Memory)，API 仍可读
make test    # 必须绿
```

## 矩阵

| 场景 | 环境 | 写入 | 展示验收 |
| --- | --- | --- | --- |
| A. 仅旧 Sheets 路径 | `STORAGE_MODE=sheets-only`，无 Google → Memory | `POST /v1/ingest` | `GET /v1/facts` + `apps/web` 有行；`/v1/meta.storage.sheets_kind=memory` |
| B. 仅 PG | `STORAGE_MODE=pg-only` + `DATABASE_URL` | ingest | facts 来自 PG；`storage.pg_kind=postgres`，`mode=pg-only` |
| C. 双写并存 | `STORAGE_MODE=dual` + `DATABASE_URL`，无 Google | Sheets=Memory + PG 同写 | facts 可读；`want_mode=dual` 且 `mode=dual`（有 PG 时） |
| D. dual 无 PG 软降级 | `STORAGE_MODE=dual`，无 `DATABASE_URL` | Memory | 服务不挂；`mode=sheets-only`，`notes` 含 degrade；web 平台条可见 |

## 手跑（C：推荐演示）

```bash
export AUTH_MODE=bypass
export STORAGE_MODE=dual
export DATABASE_URL='postgres://earnings:earnings@127.0.0.1:5432/earnings?sslmode=disable'

go run ./cmd/server -conf trpc_go.yaml
# 另一终端：
curl -sF ticker=AAPL -F force=true -F file=@testdata/aapl_2026q4.csv http://127.0.0.1:8090/v1/ingest
curl -s 'http://127.0.0.1:8090/v1/meta' | python3 -m json.tool
curl -s 'http://127.0.0.1:8090/v1/facts?ticker=AAPL' | python3 -m json.tool
curl -s 'http://127.0.0.1:8090/v1/platform/status' | python3 -m json.tool

make web
# 打开 http://127.0.0.1:5173/apps/web/ → 平台条显示 storage；加载 AAPL 有行
```

**通过标准**

1. ingest 返回成功 job；同一 ticker 再 force 仍可写（幂等键见 [dual-write.md](./dual-write.md)）。
2. `/v1/meta` 与 `/v1/platform/status` 均含非空 `storage`。
3. `apps/web` 用 `packages/api-client` 拉取事实，不依赖 Google 密钥。
4. 切回 `STORAGE_MODE=sheets-only` 后服务仍可启动；旧 Memory/Google 路径不被删除。
5. iOS / Android 骨架能展示同一 `storage` 行（构建见 [mobile.md](./mobile.md)）；不要求本机一次装齐全部 SDK。

## 自动化覆盖

- `go test ./internal/sheet` — dual / fail policy（Memory）
- `go test ./internal/app` — dual 无 PG 软降级
- `go test ./internal/httpapi` — facts / meta / platform
- `go test ./internal/monorepo` — apps/packages 目录守卫
- `make mobile-swift-test` — Swift 解码 meta/facts（可选，非 CI 默认）

## 非目标

不要求本验收替换 Google Sheets UI；不要求生产 IAM/密钥；不要求 Android CI 出 APK。
