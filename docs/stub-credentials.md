# 占位凭证如何跑全流程

真实 Google / LLM / OIDC / S3 / Redis 密钥**不是**启动前置条件。把 `.env.example` 复制为 `.env` 后，密钥可留空；需要联调高级能力时把对应 `FEATURE_*=1`，进程会走**可运行的本地 stub**（内存 / 文件 / compose），而不是空 noop。

## 最小链路（默认）

```bash
cp .env.example .env
# 不填 DEEPSEEK / FEISHU / GOOGLE / DATABASE_URL
make test
# 或
make run   # HTTP :8090
# 另开终端
cd apps/web && python3 -m http.server 5173 --bind 127.0.0.1
```

验收：

1. `POST /v1/ingest` 上传 `testdata/aapl_2026q4.csv`（`dry_run=true` 亦可）→ job 成功。
2. `STORAGE_MODE=sheets-only` 且无 Google 凭证 → Sheets 侧为 **Memory**，写入可 `GET /v1/facts?ticker=AAPL` 读回。
3. `AUTH_MODE=bypass`（默认）→ `GET /v1/auth/me` 返回合成 principal。

## 高级能力全开（仍无真实密钥）

在 `.env` 中：

```bash
FEATURE_REDIS=1
FEATURE_MQ=1
FEATURE_KAFKA=1
FEATURE_OBJECT_STORAGE=1
FEATURE_AUDIT=1
FEATURE_SEARCH=1
FEATURE_DISCOVERY=1
FEATURE_ELK=1
FEATURE_LLM=1          # 无 DEEPSEEK_API_KEY → stub-llm
FEATURE_NOTIFY=1       # 无 FEISHU_WEBHOOK_URL → memory notifier
FEATURE_SHEETS_GOOGLE=auto
AUTH_MODE=oidc
AUTH_OIDC_ISSUER=https://issuer.local
AUTH_OIDC_AUDIENCE=earnings-sync
# 下面仍可留空：REDIS_URL / MQ_URL / KAFKA_BROKERS / OBJECT_STORAGE_* / DATABASE_URL / DISCOVERY_URL / ELASTICSEARCH_URL
OBJECT_STORE_DIR=data/objects
AUDIT_LOG_PATH=data/audit.jsonl
SEARCH_INDEX_PATH=data/search
KAFKA_EVENT_LOG=data/kafka-events.jsonl
LOG_SHIPPER_PATH=data/logs/app.jsonl
DISCOVERY_BACKEND=nacos
RATE_LIMIT_RPS=50
RATE_LIMIT_BURST=20
```

OIDC stub token（JWT 形，不验签，只校验 claims）：

```text
# 用任意脚本或测试里的 auth.MintStubOIDCToken 生成
Authorization: Bearer <header.payload.sig>
# payload 至少含 sub；若设置了 AUTH_OIDC_ISSUER / AUDIENCE 则 iss/aud 必须匹配
```

验收（`GET /v1/platform/status` 可见 active=true）：

| 能力 | stub 行为 |
| --- | --- |
| Redis / cache | 内存 Get/Set + TTL；ingest 后 `facts` 带 `cache_hit` |
| MQ | 内存 Publish/Subscribe 投递 |
| Kafka | memory/file EventBus（`KAFKA_EVENT_LOG`）；有 broker 则 stub 客户端 |
| Discovery | Nacos/Consul/Eureka 接口；无集群 → 内存注册表 + 自注册 |
| ELK | JSONL shipper；可选 ES；compose profile `elk` |
| Object storage | 本地目录 Put/Get |
| Audit | 文件/PG Append + `GET /v1/audit` |
| Search | 文件/PG Upsert + `GET /v1/search?q=` |
| LLM | `extract.StubProvider` 返回可解码 JSON |
| Notify | `notify.Memory` 记录 Send |
| Gateway | token-bucket + 路由表；`allowed/rejected` 计数 |
| OIDC | JWT 形 Bearer → Principal |
| Prometheus rules | `deploy/alert-rules.yml` 可加载进 compose Prometheus + `/v1/ops/alerts` |

关高级能力（`FEATURE_*=0`）→ 对应路径回 noop / 跳过副作用，最小 ingest→store→query 仍可用。

## Sheets 真/假路径

| 条件 | 行为 |
| --- | --- |
| `GOOGLE_APPLICATION_CREDENTIALS` 有效且未 `FEATURE_SHEETS_GOOGLE=0` | Google Sheets |
| 无凭证 / Google 拨号失败 / flag off | **Memory**（数据不丢，可 ListFacts） |
| `STORAGE_MODE=dual` + 无 `DATABASE_URL` | 软降级 `sheets-only`（Memory） |

## 可选本地依赖

```bash
make pg-up          # Postgres
make infra-up       # Redis (+ NATS profile)
# 有 REDIS_URL / DATABASE_URL 时 infra 优先真后端；拨号失败且 INFRA_STRICT=false → 仍降级到可运行 stub
```

更多开关见 [feature-flags.md](feature-flags.md)、[infra.md](infra.md)、[dual-write.md](dual-write.md)。
