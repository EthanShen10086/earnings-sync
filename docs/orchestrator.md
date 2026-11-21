# AI 编排引擎

`internal/orchestrator` 是 earnings-sync 的本地 AI 驱动程序：用 YAML 描述步骤图，按依赖调度 tool / agent / condition，支持分支、重试、超时、错误恢复、幂等开跑、结构化日志与产品闭环事件落盘。

财报导入 flow 通过工具 `pipeline_ingest` **直接调用** `pipeline.Ingest`（与 HTTP `/ingest` 同一条能力）。

## 架构

```
cmd/orchestrate
    │  解析 flags / 健康检查 / 打开 FileRunStore
    ▼
Engine  ── Observer（JSON 日志 + product events + Prometheus）
    │
    ├─ RunStore / FileRunStore   运行态 + 幂等键 + 落盘
    ├─ Registry.Tools            tool 节点
    ├─ Registry.Chatter          agent 节点（DeepSeek 或 Mock）
    └─ FlowDefinition (YAML DAG)
           needs / when / retry / timeoutMs / onFailure
```

| 层 | 职责 | 主要文件 |
| --- | --- | --- |
| 定义 | YAML DAG、输入输出声明 | `configs/flows/*.flow.yaml`, `load.go` |
| 调度 | ready-queue、并发、跳过/失败策略 | `engine.go`, `graph.go` |
| 执行 | tool / agent / condition runner | `runners.go`, `builtins.go`, `pipeline_bridge.go` |
| 运行态 | RunState、幂等、落盘查询 | `runstore.go`, `filestore.go` |
| 可观测 | request/trace id、事件、指标 | `observer.go`, `context.go`, `internal/observe` |
| 稳定性 | 超时、重试、健康检查 | `engine.go` DefaultTimeout、`health.go` |

## 怎么跑 / 验收

```bash
# 1) 健康检查：flow 可加载、工具齐全、store 可写
make orchestrate-check

# 2) dry-run：真实解析 fixture，不写表
make orchestrate
# 或
go run ./cmd/orchestrate \
  -flow configs/flows/earnings-ingest.flow.yaml \
  -input ticker=AAPL -input dry_run=true \
  -input body_path=testdata/aapl_2026q4.csv -input force=true

# 3) 非 dry：写入 Memory sheet，导出证据
make orchestrate-live
# 证据：data/orchestrator-e2e-writes.json
# 运行态：data/orchestrator-runs.json
# 闭环事件：data/orchestrator-events.json

# 4) 单测
go test ./internal/orchestrator -count=1
```

验收清单：

- `-check` 输出 `"status":"ok"`
- dry-run 退出码 0，stdout 中 `status=succeeded`，`ingest.dry_run=true`
- live 写入 `orchestrator-e2e-writes.json` 含 cells；`orchestrator-events.json` 含 `flow.start` / `flow.success`
- 同一 `-client-request-id` 再跑一次，复用同一 `run_id`

接真实 DeepSeek：`-mock-model=false` 并设置 `DEEPSEEK_API_KEY`。

## 如何加新 tool

1. 在 Go 里实现 `ToolFunc`，注册进 `ToolRegistry`（`BuiltinTools`、`PipelineBridge.Tools`，或 `MergeTools`）。
2. 在 flow YAML 里加 `type: tool` 节点，设 `tool: <name>`、`input`、`needs`、可选 `retry` / `timeoutMs` / `onFailure`。
3. 单测：直接 `Engine.Run` 或走 `cmd/orchestrate`。
4. 若改了默认财报 flow，更新 `make orchestrate-check` 所需工具集合。

示例：

```go
tools := orchestrator.BuiltinTools()
tools["my_tool"] = func(ctx context.Context, input any) (any, error) {
    return map[string]any{"ok": true}, nil
}
```

```yaml
- id: step
  type: tool
  tool: my_tool
  needs: [validate]
  timeoutMs: 10000
  retry: { maxAttempts: 2, backoffMs: 100 }
  input:
    ticker: { from: input, key: ticker }
```

## 如何加新 agent

1. 注入实现 `Chatter` 的模型（`extract.HTTPProvider` 或 `MockChatter`）。
2. 可选：配置 `Registry.AgentTools` + `AgentToolHandler`，允许模型在节点内调工具。
3. YAML：`type: agent`，必填 `prompt`；`input` 引用上游节点输出。

## 如何加新 flow

1. 新建 `configs/flows/<name>.flow.yaml`。
2. 声明 `inputs`（必填项标 `required: true`）、`nodes`、可选 `output` / `concurrency`。
3. 用 `go run ./cmd/orchestrate -flow ... -check` 与带 `-input` 的试跑验收。
4. 在 README / Makefile 加快捷目标（若值得一键跑）。

## 如何维护

| 场景 | 做法 |
| --- | --- |
| 查一次失败 | 读 stdout 的 `RunState`，或 `data/orchestrator-runs.json` 按 `run_id` |
| 查产品闭环 | `data/orchestrator-events.json`：`flow.start` / `flow.success` / `flow.fail` / `step.*` |
| 关联日志 | 每条 JSON 日志含 `request_id`、`trace_id`、`run_id` |
| 指标 | HTTP 服务 `/metrics` 含 `earnings_flow_*`；CLI 进程内也会记 histogram（未暴露 HTTP 时仅内存） |
| 幂等重放 | 传相同 `-client-request-id`；pipeline 层另有 job hash 幂等 |
| 改公司/宏观 | 只改 `configs/companies.yaml` / `configs/macro.yaml`，不必改引擎 |

## 故障排查

| 现象 | 排查 |
| --- | --- |
| `缺少必填 Flow 输入` | 补 `-input key=value` |
| `未注册工具` | `MergeTools` 是否包含该 tool；`-check` 会列出缺失 |
| `flow 失败` + 节点 Error | 看该节点 `attempts`；检查 `retry` / `timeoutMs` / 上游输出 |
| 超时取消 | 调大 `-timeout` 或节点 `timeoutMs`；确认 fixture/网络 |
| dry_run 却想写表 | 设 `-input dry_run=false`；确认 dump-writes 路径 |
| 飞书没消息 | CLI 默认 `-capture-notify=true`（不发飞书）；真发需关掉并配置 webhook |
| 幂等未生效 | 是否传了 `-client-request-id`；`runs` 文件是否被清空 |
| 健康检查 fail | 先 `make orchestrate-check`，按 `checks` 字段修 |

## 稳定性约定

- **超时**：节点 `timeoutMs`；整次 run 用 `-timeout` / `Engine.DefaultTimeout`（调用方无 deadline 时生效）。
- **重试**：节点 `retry.maxAttempts` + `backoffMs`。
- **失败策略**：`onFailure: stop`（默认，取消其余）或 `continue`。
- **幂等**：`client_request_id` → 复用 `RunState`；pipeline job store 防重复写表。
- **健康检查**：`GET` 语义的 CLI `-check` / `CheckHealth`（flow + tools + store 可写）。HTTP 服务另有 `/healthz`。

## 设计取舍（摘要）

对照本机 Flow Runtime 与 ftp3_runtime_hub：

- **采纳**：YAML DAG、needs/并发、retry/onFailure、执行器注入、`client_request_id` 幂等。
- **不搬**：机器舰队、Workbench ACL、线性 nextStep 唯一推进、人工确认环。

## 节点类型

| type | 作用 |
| --- | --- |
| `tool` | 调用 `ToolRegistry` 中的 Go 函数（含 `pipeline_ingest`） |
| `agent` | 调用 `Chatter`（DeepSeek 或 `MockChatter`） |
| `condition` | 输出 `{branch, matched, value}`，配合 `when` 做分支 |

示例：`configs/flows/earnings-ingest.flow.yaml`。

## 和现有代码的关系

- `pipeline_ingest` → `pipeline.Pipeline.Ingest`（与 `cmd/server` HTTP ingest 同源）
- `internal/agent`：模型工具循环——可挂进编排的 `AgentToolHandler`
- `BuiltinTools`：离线 stub，仅用于引擎单测与无 pipeline 的演示
- HTTP `/healthz`、`/metrics`、OTLP：属于 `cmd/server` 进程；编排 CLI 复用同一套 observe 指标类型与日志字段约定
