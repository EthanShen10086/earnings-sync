# HTTP API 契约（稳定读写面）

契约版本：`2026-11-22.1`（响应头 `X-API-Version`，以及 `GET /v1/meta`）。商用控制面见 [commercial-ops.md](commercial-ops.md)。

服务入口仍是仓库根目录 `cmd/server`（monorepo 边界说明见 `apps/api`）。无真实 Google/LLM/IdP 密钥时可演示：`AUTH_MODE=bypass` + Memory/PG 读回。

## 鉴权（IAM / SSO）

| `AUTH_MODE` | 行为 |
| --- | --- |
| `bypass`（默认） | 每个受保护请求注入本地 `Principal`（`AUTH_DEV_*`） |
| `bearer` | 需要 `Authorization: Bearer <AUTH_BEARER_TOKEN>` |
| `oidc` | JWT 形 stub：校验 `sub`/`iss`/`aud`（不验签）；无真实 JWKS 也可跑通，见 [stub-credentials.md](stub-credentials.md) |

公开路径（不鉴权）：`/healthz`、`/metrics`、`/v1/meta`、`/v1/auth/providers`、所有 `OPTIONS`。

相关端点：

- `GET /v1/auth/providers` → `{ mode, providers[] }`
- `GET /v1/auth/me` → `{ principal: { sub, email, name, roles, tenant, source } }`
- `GET /v1/platform/status` → feature flags + infra + commercial 摘要
- `GET /v1/search?q=` / `GET /v1/audit` — 需 `FEATURE_SEARCH` / `FEATURE_AUDIT` 开启
- `GET /v1/commercial/status` · tenants / canary / hitl / lakehouse / sla — 见 [commercial-ops.md](commercial-ops.md)

## 读写契约

### 读事实

`GET /v1/facts?ticker=AAPL`

```json
{
  "facts": [
    {
      "idempotency_key": "...",
      "spreadsheet_id": "...",
      "ticker": "AAPL",
      "fiscal_year": 2026,
      "fiscal_quarter": 4,
      "statement": "",
      "line_item": "total_revenue",
      "value": 119.6,
      "unit": ""
    }
  ],
  "meta": { "api_version": "2026-11-21.1", "count": 1, "ticker": "AAPL", "cache_hit": false }
}
```

`FEATURE_REDIS=1` 时 ingest 后再次查询可出现 `"cache_hit": true`（内存或 Redis）。

### 写 / 任务

- `POST /v1/ingest` — multipart 或 JSON；响应 `{ job, error }`
- `GET /v1/jobs/{id}` — `{ job }`
- `POST /v1/watch`、`POST /v1/charts/recommend`、`POST /v1/charts/apply` — 既有形状保持不变

### 运维占位

- `GET /v1/ops/alerts` — 进程内 stub 告警板（非生产 pager）
- `GET /metrics` — Prometheus
- OTLP：`OTEL_EXPORTER_OTLP_ENDPOINT` 可空（noop）；接线见 `docs/observability.md`

### 错误

失败响应统一：`{ "error": "<message>" }`，HTTP 状态码表达类别（400/401/404/422/502/503）。

## 环境变量（鉴权）

```bash
AUTH_MODE=bypass          # bypass | bearer | oidc
AUTH_DEV_SUBJECT=dev-user
AUTH_DEV_EMAIL=dev@localhost
AUTH_DEV_NAME=Dev Bypass
AUTH_DEV_ROLES=admin,reader
AUTH_DEV_TENANT=local
AUTH_BEARER_TOKEN=        # bearer 模式必填
AUTH_OIDC_ISSUER=         # oidc stub：claims.iss 须匹配（可空则不校验）
AUTH_OIDC_CLIENT_ID=
AUTH_OIDC_AUDIENCE=       # claims.aud 须匹配（可空则不校验）
```

## 前端

`apps/web` 调用上述只读契约；默认 bypass，无需填 Bearer。
