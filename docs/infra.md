# 基础设施与数据面占位

诚实前提：**不是生产基建。** 本轮提供可插拔接口、本地 stub、compose/k8s 骨架；高级依赖缺失时自动降级，**不影响** ingest → extract → Sheets/Memory/PG → query 最小链路。

与 [feature-flags.md](feature-flags.md) 的关系：`FEATURE_*` 控制进程内 noop/memory 插件开关；本目录的 `internal/adapters` + `internal/infra` 负责可拨号客户端（Redis RESP / NATS TCP / 文件系统 / 审计文件|PG）、网关限流与 deploy 骨架。两者并存，默认都不阻断最小链路。

## 组件

| 能力 | 包 | 默认 / 降级 | 真实后端触发 |
| --- | --- | --- | --- |
| 缓存 | `internal/adapters/cache` | memory | `REDIS_URL` 可连 |
| MQ | `internal/adapters/mq` | memory | `REDIS_URL`（Streams）或 `NATS_URL`（TCP stub） |
| 对象存储 | `internal/adapters/objectstore` | 本地目录 | `OBJECT_STORE_DIR`；`CDN_BASE_URL` 仅改写 URL |
| 审计 | `internal/adapters/audit` | `data/audit.jsonl` | 可连 `DATABASE_URL` → `audit_events` |
| 搜索 | `internal/adapters/search` | `data/search/*.json` | 可连 `DATABASE_URL` → `search_documents` |
| 网关限流 | `internal/adapters/gateway` | 关闭 | `RATE_LIMIT_RPS` > 0 |
| 接线 | `internal/infra` | `infra.Open` | `INFRA_STRICT=true` 时拨号失败即退出 |

HTTP：`GET /healthz` 在挂上 `Infra` 时附带各后端 `backend` / `degraded` 状态；限流中间件包在整个 mux 外。

## Compose profiles

```bash
# 最小：仅 Postgres（双写 / jobs）
docker compose -f deploy/docker-compose.yml up -d postgres

# 缓存 / MQ / 对象 / 可观测（按需）
docker compose -f deploy/docker-compose.yml --profile cache up -d
docker compose -f deploy/docker-compose.yml --profile mq up -d      # redis + nats
docker compose -f deploy/docker-compose.yml --profile objects up -d # MinIO（尚未接 S3 SDK）
docker compose -f deploy/docker-compose.yml --profile observe up -d
```

示例环境变量：

```bash
REDIS_URL=redis://127.0.0.1:6379/0
NATS_URL=nats://127.0.0.1:4222
OBJECT_STORE_DIR=data/objects
CDN_BASE_URL=https://cdn.example.com
RATE_LIMIT_RPS=0
INFRA_STRICT=false
```

CDN 样例见 `deploy/cdn.env.example`。

## Kubernetes 骨架

`deploy/k8s/` 含 Deployment / Service / HPA，以及蓝绿切换注释。不要当成已上线的编排；本机仍以 compose 为准。

## 不做

- 不改 pipeline 写入核心路径（本轮只接线 HTTP / 启动日志）
- 不宣称 Kafka / 生产 CDN / 完整 API Gateway 已就绪
- 不引入 redis/nats 重量级 SDK；RESP / TCP 探测 + memory 降级足够演示
