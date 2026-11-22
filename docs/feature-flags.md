# Feature flags 与降级链路

高级能力默认关或 auto；**无密钥时最小路径** `ingest → extract(fixture/stub) → Memory/PG 存储 → 查询` 必须可跑且不丢数据。

> 占位凭证端到端步骤见 [stub-credentials.md](stub-credentials.md)。

## 原则

| 配置 | 依赖 | 结果 |
| --- | --- | --- |
| flag **off** | 任意 | **noop** / 跳过副作用 |
| flag **on** | 可用 | **启用**（进程内 stub + `internal/infra` 可拨号时走远程） |
| flag **on** | 缺失 | **降级到可运行 stub**（Memory/文件），**不是**空 noop |
| flag **auto**（LLM/Sheets/Notify/OTel） | 有密钥/端点则 live；否则 auto-off（LLM/Notify 若强制 `on` 则 stub） |

Google Sheets：**有** `GOOGLE_APPLICATION_CREDENTIALS` 且未强制 off → 走 Google；否则 Sheets 侧为 **Memory**。`STORAGE_MODE=dual` 且无可用 PG 时，**软降级**为 `sheets-only`（Memory），写入与查询仍可用。

## 环境变量

```bash
# 高级插件（默认 off）
FEATURE_REDIS=0|1|auto
FEATURE_MQ=0|1|auto
FEATURE_KAFKA=0|1|auto
FEATURE_OBJECT_STORAGE=0|1|auto
FEATURE_AUDIT=0|1|auto
FEATURE_SEARCH=0|1|auto
FEATURE_DISCOVERY=0|1|auto
FEATURE_ELK=0|1|auto

# 凭证类（默认 auto；=1 且无密钥 → 本地 stub provider）
FEATURE_NOTIFY=auto
FEATURE_LLM=auto
FEATURE_SHEETS_GOOGLE=auto
FEATURE_OTEL=auto

# 依赖占位（可空；空则 stub）
REDIS_URL=
MQ_URL=
KAFKA_BROKERS=
KAFKA_EVENT_LOG=data/kafka-events.jsonl
DISCOVERY_BACKEND=memory
DISCOVERY_URL=
ELASTICSEARCH_URL=
LOG_SHIPPER_PATH=data/logs/app.jsonl
OBJECT_STORAGE_ENDPOINT=
OBJECT_STORAGE_BUCKET=
```

存储相关仍见 [dual-write.md](dual-write.md)：`STORAGE_MODE` / `STORAGE_FAIL_POLICY` / `DATABASE_URL`。

## 并发与幂等

- `sheet.Dual`：`FailPartial` 下双后端**并行**写入；`FailAll` 仍顺序（Sheets → PG）。
- Memory / Postgres 对 idempotency key 做 upsert；并发重复 ingest 不丢、不胀行。
- 流水线仍用 `singleflight` + job hash 幂等。

## HTTP 接线

开启对应 `FEATURE_*` 后，`POST /v1/ingest` 会真正执行：

- audit Append → `GET /v1/audit`
- search Upsert → `GET /v1/search?q=`
- cache Set → `GET /v1/facts` 返回 `meta.cache_hit`
- mq Publish（内存订阅可收到）
- object Put（`OBJECT_STORE_DIR`）
- kafka `internal/adapters/kafka`（memory/file/broker-stub）
- discovery Discover（`internal/discovery` 内存或 Nacos/Consul/Eureka stub）
- elk Shipper 写 JSONL（可选 POST ES）

总览：`GET /v1/platform/status`。

## 代码入口

- `internal/features` — flag 解析与可运行 stub（flag on 无依赖 → Memory）
- `internal/infra` + `internal/adapters` — 可拨号客户端，失败降级 Memory/文件
- `internal/discovery` — Nacos/Consul/Eureka 接口 + 内存注册表
- `internal/observability` — log shipper + Prometheus rule 加载
- `internal/httpapi` — ingest 副作用 + search/audit/platform API
- `internal/extract.StubProvider` / `internal/notify.Memory` / `internal/auth` OIDC stub
- `internal/app.BuildStoreDetailed` — 存储软降级
- `internal/sheet.Dual` — 双写 / 并发
