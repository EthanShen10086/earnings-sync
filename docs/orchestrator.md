# AI 编排引擎

`internal/orchestrator` 是 earnings-sync 的本地 AI 驱动程序：用 YAML 描述步骤图，按依赖调度 tool / agent / condition，支持分支、重试与错误恢复。生产导入仍走 `cmd/server` → `pipeline.Ingest`；编排引擎是独立可配置入口， stub 工具可换成真实 sheet/notify/agent。

## 怎么跑

```bash
go run ./cmd/orchestrate \
  -flow configs/flows/earnings-ingest.flow.yaml \
  -input ticker=AAPL \
  -input dry_run=true

make orchestrate
go test ./internal/orchestrator -count=1
```

接真实 DeepSeek：`-mock-model=false` 并设置 `DEEPSEEK_API_KEY`。

## 参考了什么、如何取舍

对照本机 `/Users/ethanshen/Desktop/code/test` 里两套实现：

| 来源 | 角色 | 对本仓库的结论 |
| --- | --- | --- |
| **fit-ai-plugin Flow Runtime**（`SimpleFlowEngine`） | 进程内 DAG 泵：`needs` / 并发 / agent·tool 节点 / `FlowValue` / retry·stop | **主骨架采纳**。earnings-sync 要的是「可配置地驱动模型与工具」，不是机器调度。 |
| **ftp3_runtime_hub** | Pipeline Hub 控制面：版本化 `pipeline:` YAML、线性 `nextStep`、机器分配、Runner WS、人工确认 | **不整仓搬**。它明确废弃了 Hub 内图编排（拒绝顶层 `nodes:`），图引擎已交给插件。 |

### 从 Flow Runtime 借鉴

- YAML/JSON `FlowDefinition` + 节点 `type: agent|tool|condition`
- 依赖 `needs` / `needsMode: all|any`，并发 ready-queue
- `FlowValue`：`{from:input|node|nodes}`
- 节点 `retry`、`onFailure: stop|continue`、`when` 条件跳过
- 执行器注入（`Chatter` / `ToolRegistry`），引擎不直连模型厂商 API

### 从 ftp3_runtime_hub 借鉴（局部）

- **`client_request_id` 幂等开跑**：同一请求键复用已有 `RunState`（见 `RunStore`）
- **定义 / 运行态 / 执行器分离**：YAML 只描述图；`RunState` 记状态；工具与模型可替换
- **步骤声明式输入**：flow 级 `inputs` + 节点 `input` 引用（对应 Hub 的 `input_parameters` / step inputs 思路，去掉 artifact/env 舰队）
- **失败策略显式化**：对齐 Hub `pre_hooks.on_failure` / step 终态语义，用 `onFailure` 表达

### 明确不搬

- Runner / envproxy / 机器分配 / Workbench ACL 与回调
- 线性 `stages → steps` 作为唯一推进方式（财报导入需要 dry_run 分支与并行校验，DAG 更合适）
- `waiting_confirm` / intervention 人工环（本仓库无审批台）
- MCP/Skill 目录与 AG-UI（已有独立 `cmd/mcp` + `internal/agent`）

## 节点类型

| type | 作用 |
| --- | --- |
| `tool` | 调用 `ToolRegistry` 中的 Go 函数 |
| `agent` | 调用 `Chatter`（DeepSeek 或 `MockChatter`） |
| `condition` | 输出 `{branch, matched, value}`，配合 `when` 做分支 |

示例：`configs/flows/earnings-ingest.flow.yaml`。

## 和现有代码的关系

- `internal/agent`：模型工具循环（写表/飞书）——可挂进编排的 `tool` / `AgentToolHandler`
- `internal/pipeline`：线上导入编排（脚本化）——编排引擎是配置化的并行路径，不替换 HTTP ingest
