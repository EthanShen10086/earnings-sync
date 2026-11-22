# 可观测与告警仪表盘（接线占位）

目标：本地可演示 metrics / traces / alerts / ELK **形状**；无外部密钥、无真实 Grafana 账号也能跑最小路径。

## 已有

| 组件 | 说明 |
| --- | --- |
| slog JSON | `internal/observe`；带 `trace_id` / `span_id` 时自动注入 |
| Prometheus | `GET /metrics`；`deploy/prometheus.yml` 抓取 `host.docker.internal:8090` |
| Alert rules | `deploy/alert-rules.yml` **已挂进** Prometheus（compose profile `observe`）；进程内 `LoadAlertRules` → `/v1/ops/alerts` |
| OTLP HTTP | `OTEL_EXPORTER_OTLP_ENDPOINT`；空则 noop tracer，服务仍可启动 |
| 告警 stub | `GET /v1/ops/alerts` 进程内板 + 已加载的 Prometheus 规则名 |
| Log shipper | `internal/observability.Shipper` → `LOG_SHIPPER_PATH` JSONL；可选 `ELASTICSEARCH_URL` |
| ELK compose | profile `elk`：Elasticsearch + Kibana + Filebeat |

## 本地 stub 一键

```bash
# API（可不设 OTEL）
AUTH_MODE=bypass STORAGE_MODE=dual \
  DATABASE_URL=postgres://earnings:earnings@127.0.0.1:5432/earnings?sslmode=disable \
  go run ./cmd/server -conf trpc_go.yaml

# Jaeger + Prometheus（加载 alert-rules）+ Grafana provisioning
make observe-up
# 或
docker compose -f deploy/docker-compose.yml --profile observe up -d

# ELK（可选；无集群时进程仍写 data/logs/app.jsonl）
FEATURE_ELK=1 make elk-up
# 指向 compose ES：
ELASTICSEARCH_URL=http://127.0.0.1:9200

# 可选：把 OTLP 指到 Jaeger
OTEL_EXPORTER_OTLP_ENDPOINT=http://127.0.0.1:4318 go run ./cmd/server -conf trpc_go.yaml
```

- Jaeger UI：http://127.0.0.1:16686  
- Prometheus：http://127.0.0.1:9090（Status → Rules 可见 `earnings-sync-stubs`）  
- Grafana：http://127.0.0.1:3000（Prometheus datasource + dashboard 自动 provision）  
- Kibana：http://127.0.0.1:5601（profile `elk`）  
- ES stub（无 Docker）：进程内 `observability.ESStubHandler` 供单测；生产前换真集群

## 告警规则

`deploy/alert-rules.yml` 由 `deploy/prometheus.yml` 的 `rule_files` 加载。`ALERT_RULES_PATH`（默认同路径）在启动时解析进 `/v1/ops/alerts` 的 `prometheus_rules` 与 `/v1/platform/status`。

## 不是什么

这不是完整 SLO / 采样策略 / 多租户审计。生产接入真实 IdP、告警通道与保留策略前，保持 stub + feature-flag 心态。
