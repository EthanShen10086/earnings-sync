# 可观测与告警仪表盘（接线占位）

目标：本地可演示 metrics / traces / alerts **形状**；无外部密钥、无真实 Grafana 账号也能跑最小路径。

## 已有

| 组件 | 说明 |
| --- | --- |
| slog JSON | `internal/observe`；带 `trace_id` / `span_id` 时自动注入 |
| Prometheus | `GET /metrics`；`deploy/prometheus.yml` 抓取 `host.docker.internal:8090` |
| OTLP HTTP | `OTEL_EXPORTER_OTLP_ENDPOINT`；空则 noop tracer，服务仍可启动 |
| 告警 stub | `GET /v1/ops/alerts` 进程内板；规则草稿 `deploy/alert-rules.yml` |

## 本地 stub 一键

```bash
# API（可不设 OTEL）
AUTH_MODE=bypass STORAGE_MODE=dual \
  DATABASE_URL=postgres://earnings:earnings@127.0.0.1:5432/earnings?sslmode=disable \
  go run ./cmd/server -conf trpc_go.yaml

# 可选：Jaeger + Prometheus（compose profile observe）
docker compose -f deploy/docker-compose.yml --profile observe up -d

# 可选：把 OTLP 指到 Jaeger
OTEL_EXPORTER_OTLP_ENDPOINT=http://127.0.0.1:4318 go run ./cmd/server -conf trpc_go.yaml
```

- Jaeger UI：http://127.0.0.1:16686  
- Prometheus：http://127.0.0.1:9090  
- Grafana 仪表盘 JSON 占位：`deploy/grafana/dashboards/earnings-sync.json`（导入即可；未强制 compose 起 Grafana）

## 告警规则草稿

`deploy/alert-rules.yml` 面向 Prometheus；当前 **未** 挂进运行中的 Prometheus 容器（避免假装生产 on-call）。`/v1/ops/alerts` 返回同主题的 stub 行，供前端/运维面联调。

## 不是什么

这不是完整 SLO / 采样策略 / 多租户审计。生产接入真实 IdP、告警通道与保留策略前，保持 stub + feature-flag 心态。
