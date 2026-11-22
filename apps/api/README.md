# apps/api

HTTP/tRPC 服务的 monorepo 应用边界。**可执行入口仍在仓库根** `cmd/server`（以及 `cmd/orchestrate`、`cmd/doctor`），本目录不另立 Go module，避免拆模块爆炸。

## 跑起来

```bash
# 仓库根
make run
# 等价：
go run ./cmd/server -conf trpc_go.yaml
```

建议演示环境：

```bash
AUTH_MODE=bypass STORAGE_MODE=dual \
  DATABASE_URL=postgres://earnings:earnings@127.0.0.1:5432/earnings?sslmode=disable \
  go run ./cmd/server -conf trpc_go.yaml
```

## 契约与客户端

| 面 | 路径 |
| --- | --- |
| 散文契约 | [`docs/api-contract.md`](../../docs/api-contract.md) |
| OpenAPI lite | [`packages/api-types/openapi-lite.json`](../../packages/api-types/openapi-lite.json) |
| JS 客户端 | [`packages/api-client`](../../packages/api-client) |
| Swift 客户端 | [`packages/swift-api-client`](../../packages/swift-api-client) |
| Web | [`apps/web`](../web) |
| iOS / Android | [`apps/mobile-ios`](../mobile-ios) · [`apps/mobile-android`](../mobile-android) |

`GET /v1/meta` 与 `GET /v1/platform/status` 返回 `storage`：`want_mode` / `mode` / `sheets_kind` / `pg_kind`，用于 Sheets 旧链路与 PG/API 展示并存验收。

## Makefile 捷径

本目录提供薄封装（转调仓库根）：

```bash
make -C apps/api run
make -C apps/api test
```
