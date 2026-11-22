# 商用运维面（ORM / 多租户 / 灰度 / 湖仓 / SLA / HITL）

诚实前提：本轮提供**可跑通的状态机与可 apply 的 K8s 清单**；无真实集群 / 数仓时用 Memory stub。Sheets + PostgreSQL 双写路径不变。

## 包一览

| 包 | 作用 |
| --- | --- |
| `internal/orm` | 轻量 ORM：`WithContext` / `Create` / `Where`/`Find`/`Updates`/`Delete` / `Transaction`；底层 SQL 或 Memory |
| `internal/tenant` | 租户注册表 + `X-Tenant-ID` 中间件 + context |
| `internal/canary` | flow 灰度：draft → canary → promoted / rolled_back；按 routing key 分桶 |
| `internal/hitl` | 人工干预环：pending → approved/rejected/timeout/canceled |
| `internal/lakehouse` | 分区事实表 + export job stub（pending → running → succeeded） |
| `internal/sla` | 备份/恢复演练钩子（preflight/snapshot/verify）+ RTO 判定 |

HTTP（鉴权同其它 `/v1/*`；默认租户 `default`）：

- `GET /v1/commercial/status`
- `GET|POST /v1/tenants`
- `POST /v1/canary/releases` … `/canary` `/promote` `/rollback` `/pick`
- `POST /v1/hitl/ask` · `POST /v1/hitl/{id}/decide`
- `POST /v1/lakehouse/facts` · exports `start`/`run`
- `POST /v1/sla/drills` · `POST /v1/sla/drills/{id}/run`

## Docker / K8s

```bash
make docker-build          # Dockerfile → earnings-sync:local
make k8s-validate          # kubectl dry-run 或 YAML 结构校验（无集群可过）
kubectl apply -k deploy/k8s   # 有集群时
```

清单：`deploy/k8s/`（namespace、ConfigMap、Deployment、canary Deployment、Service、HPA）。说明见该目录 README。

## 与双写的关系

- Ingest / Sheets / PG：仍走 `STORAGE_MODE` 与 `sheet.Store`。
- 湖仓 `lakehouse_facts` 是**分析副本骨架**（migration `003_commercial.sql`）；Memory ORM 在无 PG 时同样可测。
- 灰度与 HITL 挂在控制面，不改写既有 pipeline 热路径。

## 验收

```bash
make test
make k8s-validate
go test ./internal/orm ./internal/tenant ./internal/canary ./internal/hitl ./internal/lakehouse ./internal/sla ./internal/httpapi -count=1
```
