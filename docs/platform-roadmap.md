# earnings-sync 平台能力路线图

> 诚实前提：**不是一次全上。** 用户愿景覆盖 Sheets+PG 双写、Redis/MQ/Kafka、monorepo、前端渲染引擎、IAM/SSO、OTel、i18n、深浅色、多端、对象存储/CDN、K8s、蓝绿、湖仓、网关、CI/CD、审计安全搜索等——这些要分阶段落地。本轮只做「可跑的双写底座 + 最小骨架」，其余在表中占位。

评估基线：本机 `~/Projects/earnings-sync`（master）与商用缺口文档 `docs/commercial-readiness.md`。日期：2026-11-21。

## 本轮做 / 不做（一句话）

| | 范围 |
| --- | --- |
| **本轮做（P0 切片）** | Sheets 保留；PostgreSQL Writer；`sheets-only` / `pg-only` / `dual` 可配；幂等键与迁移；docker-compose（PG + Redis 占位）；最小 monorepo/`apps/web`；文档与 `make test`；PR 合并 |
| **本轮不做** | Kafka 全集群生产面、完整 IAM/SSO、数据湖生产面、CDN 边缘、完整渲染引擎、深浅色/i18n 产品化 |

## P0 — 底座（优先，可商用演示）

| 能力 | 本轮 | 说明 |
| --- | --- | --- |
| Google Sheets 写入 | ✅ 保留 | 现有 `sheet.Store` / Google 实现不变 |
| PostgreSQL 存储 | ✅ 本轮 | 实现同一 Writer 接口；事实表 + 幂等键 + 迁移 SQL |
| 集中式存储 / 跨实例幂等 | ✅ 已实现 | 有 `DATABASE_URL`：facts 双写 + jobs/runs/events → PG（PR #5/#6）；无 PG：降级单机文件 / Memory |
| 双写编排 | ✅ 本轮 | `STORAGE_MODE=sheets-only\|pg-only\|dual`；失败策略可测 |
| 本地依赖 | ✅ 本轮 | `deploy/docker-compose.yml` 起 PG；Redis 仅占位服务 |
| 最小 monorepo | ✅ 深化 | `apps/api|web|mobile-ios|mobile-android` + `packages/{api-client,api-types,swift-api-client}`；Go 仍单 module（`docs/monorepo.md`） |
| 可观测（现有） | 部分 ✅ | slog / Prometheus / OTLP；Kafka 总线 + ELK shipper + 可加载告警规则（PR #11） |
| CI | ✅ 本轮 | `make test` + CI 可挂 PG 服务跑集成 |

**下一推荐 P0（本轮之后）：** ~~编排/任务幂等迁入 PG（替换单机 `jobs.json`）~~ ✅（见 `internal/jobs-pg-idempotency-delivery.md`）→ ~~只读 API 稳定契约、告警仪表盘接线~~ ✅（`docs/api-contract.md` + `docs/observability.md` + `/v1/ops/alerts` stub）。

## P1 — 产品与协作面

| 能力 | 本轮 | 占位说明 |
| --- | --- | --- |
| 前端渲染引擎 | 部分 ✅ | `apps/web` + `packages/api-client` 对齐契约；平台条展示 storage 并存；图表/布局引擎后续 |
| i18n / 深浅色 | 部分 ✅ | web 中/英 + 深浅色骨架；未产品化文案平台 |
| Redis 缓存 / 会话 | 部分 ✅ | `FEATURE_REDIS=1` 无 URL → **可运行** Memory（TTL/命中）；有 Redis → RESP；ingest 接线 `cache_hit`（PR runnable-stubs） |
| MQ（轻量） | 部分 ✅ | `FEATURE_MQ` + Redis Streams（XADD + 同进程 Subscribe）/ NATS / memory 真投递（PR #9 + runnable-stubs） |
| IAM / SSO | 部分 ✅ | `bypass`/`bearer`/`oidc`；OIDC 为 JWT 形 stub（校验 claims，不验签） |
| 审计日志 / 安全搜索 | 部分 ✅ | flag on → 文件/PG **真写入/查询** + `/v1/audit` `/v1/search`（非 noop） |
| 对象存储（开发桶） | 部分 ✅ | filesystem Put/Get 可跑 + MinIO compose；生产 S3 SDK 后续 |
| CI/CD 深化 | 部分 | 已有 fmt/test；发布流水线、环境晋升后续 |

## P2 — 平台与规模化

| 能力 | 本轮 | 占位说明 |
| --- | --- | --- |
| Kafka 全集群 | 部分 ✅ | `FEATURE_KAFKA=1` → `internal/adapters/kafka` memory/file/broker-stub（无 broker 可跑）；全集群/回放后续（PR #11） |
| API 网关 | 部分 ✅ | token-bucket + **路由表/反代 stub** + `/v1/platform/status`；非完整多服务入口（PR #11） |
| 服务发现 | 部分 ✅ | `internal/discovery` Nacos/Consul/Eureka **可跑 stub**；无集群 → 内存注册表自注册（PR #11） |
| ELK | 部分 ✅ | compose profile `elk` + Filebeat；`Shipper` → JSONL / ES stub（PR #11） |
| K8s / 蓝绿 / 金丝雀 | 部分 ✅ | `deploy/k8s` Deployment/Service/HPA + 蓝绿注释；非生产拓扑（PR #9） |
| CDN + 生产对象存储 | 部分 ✅ | `CDN_BASE_URL` URL 改写占位 + `deploy/cdn.env.example`；无边缘缓存（PR #9） |
| 湖仓 / 数仓 | ❌ | 分析副本、分区、批流一体 |
| 多端原生（iOS/Android） | 部分 ✅ | iOS Simulator **可构建**；Android Gradle 骨架 + 构建说明（需 SDK）；共用只读 API + storage 快照（`docs/mobile.md`） |
| 完整 OTel 采样与 SLO | 部分 ✅ | Jaeger/Prometheus/Grafana compose；**alert-rules 已挂进 Prometheus** + Grafana provisioning（PR #11）；非生产 SLO |

## 商用缺口（对照，非本轮交付）

仍缺真实密钥托管、生产级多租户/SLA、渗透与备份演练等——见 `docs/commercial-readiness.md`。「集中式存储 / 跨实例幂等」✅ **已实现**（有 `DATABASE_URL` 时：Sheets+PG 双写与 jobs/runs/events 写 PostgreSQL，跨实例按幂等键恢复；无 PG 时降级单机文件 / Memory）。双写与 PG **不等于可对外商用**。

## 验收指向（本轮代码）

1. 无 Google 密钥：`STORAGE_MODE=dual` 时用 Memory + PG（或 CI 中的 Postgres）双写仍可通过测试；无 PG 时软降级 Memory，写入可查不丢。
2. `make test` 绿；文档说明如何 `docker compose` 起 PG、切模式、打开 `apps/web`。
3. 明确失败策略：默认双写任一侧失败则整次写入失败；可测 `partial` 策略（一侧失败仍提交另一侧并记错；并行 fan-out）。
4. Feature flags：关 → noop；**开且依赖缺失 → 可运行 Memory/文件 stub**（非空壳）；开且依赖可用 → 远程/同一套流程（见 `docs/stub-credentials.md`）。
5. 数据面 adapters：`internal/infra.Open` 缺 Redis/NATS/PG 时降级到可跑 stub，不挡最小链路；ingest 接线审计/搜索/缓存/MQ/对象存储。
6. Monorepo / 多端：`make test` 含 `internal/monorepo` 布局守卫；Sheets 旧链路与 PG/API 展示并存验收见仓库 `docs/coexistence-acceptance.md`。

## 演进原则

1. **接口先于基建**：Writer/Store 抽象稳定后，再换 Kafka/网关。
2. **单写可回退**：任何时候可切回 `sheets-only` 或 `pg-only`。
3. **骨架不爆炸**：monorepo 加深 `apps/`+`packages/`，Go 仍单 module，不搬迁全部包。
4. **路线图可砍**：P2 项可无限期停留在占位，直到有明确租户与 SLA。
