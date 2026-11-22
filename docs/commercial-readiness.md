# earnings-sync 商用就绪评估

评估对象：本机 `~/Projects/earnings-sync`（master 基线）及本轮补齐后的编排底座。日期：2026-11-21。

## 结论（一句话）

引擎代码体量偏精简但**扩展点够用**；补齐文档/可观测/落盘闭环后，达到**可维护、可观测、可扩展的半商用底座**——**尚不可直接对外商用**（缺真实密钥环境、多租户、运维与 SLA 等）。

## 诚实回答

| 问题 | 判断 |
| --- | --- |
| 引擎代码是否太少？ | **偏少但合理**。全仓 Go ~12k LOC；`internal/orchestrator`+CLI 约 2k+。DAG 泵本身不该堆业务；业务在 `pipeline`/`sheet`/`extract`。过少的是「运维面」而非调度内核。 |
| 能否扩展维护？ | **能**。ToolRegistry / Chatter / YAML flow / MergeTools 已分离；加 tool/agent/flow 有文档路径。缺的是版本化 flow 发布与多人协作规范（后续）。 |
| README 是否沉淀实现/维护/扩展？ | **补齐前不足**（仅指向短文档）；**本轮已补** `docs/orchestrator.md`（架构/跑通/加扩展/维护/排障）与 README 落盘说明。 |
| 可观测？ | **HTTP 链路已有** slog/Prometheus/OTLP；**编排曾几乎只有 Events channel**。本轮贯通 request/trace/run id、结构化日志、flow/node 指标、事件落盘。 |
| 稳定性？ | **已有** 节点 retry / onFailure / timeout / client_request_id 幂等；整次 run 超时、健康检查；有 `DATABASE_URL` 时跨实例可恢复，无 PG 时 FileRunStore 单机可查。 |
| 埋点 / 产品闭环？ | 有 `DATABASE_URL` 时 jobs/runs/events 写 PostgreSQL；无 PG 时 `jobs.json` / `orchestrator-runs.json` / `orchestrator-events.json`。无外部分析平台。 |
| 是否可商用？ | **否（半商用底座）**。可内部试用/演示/二次开发；不可承诺生产 SLA、合规与多客户隔离。 |

## 本轮已补齐（代码）

- 文档：`docs/orchestrator.md` 扩展；README 编排章节更新
- 可观测：`Observer`、JSON 日志字段贯通、`earnings_flow_*` 指标
- 稳定性：默认/可配 run timeout、`-check` 健康检查、节点 timeout 写入财报 flow
- 闭环：有 `DATABASE_URL` 时 jobs/runs/events 写 PostgreSQL（跨实例幂等，PR #5/#6）；无 PG 时 FileRunStore / `jobs.json` 单机文件降级
- 测试：产品事件、超时、健康检查、落盘重载

## 距商用仍缺（需真实环境 / 未做）

1. **真实 LLM / Google Sheets / 飞书密钥** 的托管、轮换与生产配置（本环境不落密钥）
2. **编排 HTTP API / 鉴权 / 生产级多租户**（本地 CLI + 租户 stub 已有；非生产隔离）
3. ~~**集中式存储**（Postgres 等）与跨实例幂等~~ ✅ **已实现**（有 `DATABASE_URL` 时；无 PG 时降级单机文件 / Memory——**不是缺口**）
4. **告警与仪表盘生产化**（本地 Prometheus 已加载 `alert-rules.yml` + Grafana provisioning + `/v1/ops/alerts` 规则列表；仍缺真实 pager / SLO / 采样策略）
5. **限流、配额、审计日志合规、密钥保险箱**（进程内网关限流+路由表已有；非边缘生产网关）
6. **CI 中的 live E2E**（真实 Sheets/飞书；现仅 Memory fixture）
7. **flow 版本发布、灰度、回滚** 控制面（canary stub 已有；非生产发布系统）
8. **人工确认 / 干预环**、长任务队列与重放 UX（HITL stub 已有）
9. **安全评审与渗透、备份恢复演练、on-call 手册**（SLA drill stub 已有；非真实演练）
10. **真实服务发现 / Kafka / ELK 集群**（PR #11：Nacos/Consul/Eureka + Kafka memory/file + ELK compose/shipper 为可跑 stub，非生产集群）

## 体量参考（审计时）

- 全仓 `*.go`（排除 worktree/vendor）≈ 76 文件 / ~12k LOC
- 编排相关：`internal/orchestrator/*`、`cmd/orchestrate`、`docs/orchestrator.md`
