# apps/api

Go HTTP/tRPC 服务入口仍是仓库根目录的 `cmd/server`（以及 `cmd/orchestrate`、`cmd/doctor` 等）。

本目录仅作 monorepo 边界说明。稳定读写契约与 IAM 占位：

- 契约文档：[docs/api-contract.md](../../docs/api-contract.md)
- 鉴权实现：`internal/auth`（`AUTH_MODE=bypass|bearer|oidc`）
- HTTP 面：`internal/httpapi`（`/v1/meta`、`/v1/auth/*`、`/v1/facts`、`/v1/ops/alerts` …）
