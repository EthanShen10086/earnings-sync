# packages/api-types

稳定 HTTP 契约的机器可读快照。权威说明仍是仓库 [`docs/api-contract.md`](../../docs/api-contract.md)。

- `openapi-lite.json` — 只读面最小 OpenAPI 3 片段（meta / facts / platform status / auth）
- 契约版本字段：`info.version` 与响应头 `X-API-Version` / `GET /v1/meta.api_version` 对齐
