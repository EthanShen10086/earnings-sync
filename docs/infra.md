# 基础设施与数据面占位

诚实前提：**不是生产基建。** 本轮提供可插拔接口、本地 stub、compose/k8s 骨架；高级依赖缺失时自动降级，**不影响** ingest → extract → Sheets/Memory/PG → query 最小链路。

与 [feature-flags.md](feature-flags.md) 的关系：`FEATURE_*` 控制副作用是否执行（关=noop，开=可运行 stub）；`internal/adapters` + `internal/infra` + `internal/discovery` + `internal/observability` 负责可拨号客户端与骨架。拨号失败且 `INFRA_STRICT=false` 时降级到 Memory/文件，**仍执行**同一套接口。占位凭证跑法见 [stub-credentials.md](stub-credentials.md)。

## 组件

| 能力 | 包 | 默认 / 降级 | 真实后端触发 |
| --- | --- | --- | --- |
| 缓存 | `internal/adapters/cache` | memory | `REDIS_URL` 可连 |
| MQ | `internal/adapters/mq` | memory | `REDIS_URL`（Streams）或 `NATS_URL`（TCP stub） |
| Kafka 总线 | `internal/adapters/kafka` | memory / `KAFKA_EVENT_LOG` 文件 | `KAFKA_BROKERS` 可连 → broker-stub（可换 SDK） |
| 对象存储 | `internal/adapters/objectstore` | 本地目录 | `OBJECT_STORE_DIR`；`CDN_BASE_URL` 仅改写 URL |
| 审计 | `internal/adapters/audit` | `data/audit.jsonl` | 可连 `DATABASE_URL` → `audit_events` |
| 搜索 | `internal/adapters/search` | `data/search/*.json` | 可连 `DATABASE_URL` → `search_documents` |
| 服务发现 | `internal/discovery` | 内存注册表 | `DISCOVERY_BACKEND=nacos\|consul\|eureka` + URL；不可达仍内存 |
| 网关 | `internal/adapters/gateway` | 路由表 + 可选限流 | `RATE_LIMIT_RPS` > 0；可 `RegisterRoute` 反代 |
| 日志 shipper | `internal/observability` | `LOG_SHIPPER_PATH` JSONL | `ELASTICSEARCH_URL` → file+ES |
| 接线 | `internal/infra` | `infra.Open` | `INFRA_STRICT=true` 时拨号失败即退出 |

HTTP：`GET /healthz` 在挂上 `Infra` 时附带各后端 `backend` / `degraded` 状态；网关包在整个 mux 外。`GET /v1/platform/status` 暴露 discovery / kafka / shipper / alert_rules。

## Compose profiles

```bash
# 最小：仅 Postgres（双写 / jobs）
docker compose -f deploy/docker-compose.yml up -d postgres

# 缓存 / MQ / 对象 / 可观测 / ELK（按需）
docker compose -f deploy/docker-compose.yml --profile cache up -d
docker compose -f deploy/docker-compose.yml --profile mq up -d      # redis + nats
docker compose -f deploy/docker-compose.yml --profile objects up -d # MinIO（尚未接 S3 SDK）
make observe-up   # prometheus（含 alert-rules）+ grafana + jaeger
make elk-up       # elasticsearch + kibana + filebeat
```

示例环境变量：

```bash
REDIS_URL=redis://127.0.0.1:6379/0
NATS_URL=nats://127.0.0.1:4222
KAFKA_BROKERS=          # 空 → 文件/内存总线
KAFKA_EVENT_LOG=data/kafka-events.jsonl
DISCOVERY_BACKEND=nacos
DISCOVERY_URL=          # 空 → nacos-memory 注册表
FEATURE_DISCOVERY=1
FEATURE_KAFKA=1
FEATURE_ELK=1
LOG_SHIPPER_PATH=data/logs/app.jsonl
ELASTICSEARCH_URL=http://127.0.0.1:9200
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
- 不引入 Nacos/Consul/Eureka/Kafka 重量级 SDK；接口 + 可跑 stub + 拨号探测足够演示
- 不宣称生产 ELK / 完整边缘 API Gateway 已就绪
