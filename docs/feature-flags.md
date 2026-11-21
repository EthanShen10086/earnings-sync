# Feature flags 与降级链路

高级能力默认关或 auto；**无密钥时最小路径** `ingest → extract(fixture) → Memory/PG 存储 → 查询` 必须可跑且不丢数据。

## 原则

| 配置 | 依赖 | 结果 |
| --- | --- | --- |
| flag **off** | 任意 | 使用 noop / 跳过能力 |
| flag **on** | 可用 | **启用**（当前多为进程内适配器；远程客户端可后续替换） |
| flag **on** | 缺失 | **降级**到 noop，进程不退出 |
| flag **auto**（默认用于 LLM/Sheets/Notify/OTel） | 有密钥/端点则启用，否则保持关闭 | |

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

# 凭证类（默认 auto）
FEATURE_NOTIFY=auto
FEATURE_LLM=auto
FEATURE_SHEETS_GOOGLE=auto
FEATURE_OTEL=auto

# 依赖占位（可空）
REDIS_URL=
MQ_URL=
KAFKA_BROKERS=
OBJECT_STORAGE_ENDPOINT=
OBJECT_STORAGE_BUCKET=
```

存储相关仍见 [dual-write.md](dual-write.md)：`STORAGE_MODE` / `STORAGE_FAIL_POLICY` / `DATABASE_URL`。

## 并发与幂等

- `sheet.Dual`：`FailPartial` 下双后端**并行**写入；`FailAll` 仍顺序（Sheets → PG）。
- Memory / Postgres 对 idempotency key 做 upsert；并发重复 ingest 不丢、不胀行。
- 流水线仍用 `singleflight` + job hash 幂等。

## 代码入口

- `internal/features` — flag 解析与插件 stub
- `internal/app.BuildStoreDetailed` — 存储软降级
- `internal/sheet.Dual` — 双写 / 并发
