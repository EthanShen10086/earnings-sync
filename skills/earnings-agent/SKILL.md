---
name: earnings-agent
description: 用 earnings-sync 的 MCP 工具让模型自己更新或新建 Google 财务报表，而不是只写摘要。当用户要导入财报、改表格、新建三张表、发飞书 takeaway，或问 DeepSeek 能不能操作表格时使用。
---

# 财报代理

DeepSeek 通过工具改表格。不要只回复「建议你去改表格」。

## 工具

MCP 服务名 `earnings-sync`，命令是仓库里的 `go run ./cmd/mcp`。

- `list_companies`：看哪家公司已经有表。
- `create_workbook`：没有表时新建利润表、资产负债表、现金流量表。
- `upsert_period`：把一个财季写入黄色录入格。
- `apply_chart_rules`：补柱状图或饼图。
- `send_takeaway`：把 3 到 5 条关注点发到飞书。

## 模型

执行必须用能调用工具的模型：`deepseek-flash`，或更难的判断用 `deepseek-v4-pro`。`deepseek-chat` 和 `deepseek-reasoner` 已退役，不能当作这个代理的模型。

## 不要做的事

不要读取或复述服务账号 JSON、`client_email`、飞书 webhook、API key，也不要读取用户填进 `GOOGLE_SHARE_WITH` 的邮箱。

新建表之前如果 `.env` 里 `GOOGLE_SHARE_WITH` 还是空的，不要只说缺这一项。让用户打开 `docs/connect.md` 第 4 节：先在服务账号所在的 Cloud 项目里启用 Google Drive API，再把登录 Google 表格时右上角的邮箱写到 `GOOGLE_SHARE_WITH=`。也可以让用户运行 `go run ./cmd/doctor`，按「新建表格」下面的步骤做。
