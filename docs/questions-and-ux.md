# 这次对话里卡住的地方

给以后自己跑、也给别人第一次跑。每一条都是实际发生过的疑问，后面是当时缺的说明，以及还能怎么把体验做顺。

## 数据该怎么存

问：为什么只按第四季度整理，之后怎么补数据、怎么筛选？

三张图各自是一个切片，不能当账本。正确的粒度是一行一个财季。利润表、资产负债表、现金流量表是会计三张表，不是三张图。空白表示还没录入。筛选用表头下拉，按财年或财季过滤。新季度从第 46 行接着写财年和财季。

体验上还缺：第一次打开会看到很多空行，没有一句写在表格里的「从这里追加」。

## 什么叫真正跑通

问：不写表、不发飞书的导入，不能算测完。

`dry_run=true` 只证明抽取和勾稽。写进 Google Sheet、发到飞书，要另做真实调用。`go run ./cmd/doctor` 会发一条飞书测试消息，并读一次表头。`EARNINGS_LIVE=1 go test ./internal/live -count=1` 是同一件事的自动测试。

体验上还缺：doctor 失败时直接打开该去的网页，而不是只给链接。

## 密钥和权限

问：`chmod 600` 怎么执行？为什么不能把密钥发到对话里？

`chmod 600 文件路径` 表示只有你能读写这个文件。JSON 私钥、`client_email`、飞书 webhook、模型 key 都只留在本机 `.env` 或密钥文件里。对话里不贴内容。

体验上还缺：doctor 发现下载目录里的新 JSON 时，可以问一句要不要帮你改权限和写入路径，而不是让人先记住命令。

## 用哪一个浏览器

问：不要另开浏览器，用已经打开的 Chrome。

自动化若开了自己的浏览器，里面没有你的登录态。要操作已经打开的窗口，还得先允许辅助功能和屏幕录制。登录、验证码、下载密钥仍由你本人完成。

## 模型

问：`deepseek-chat` 够不够？DeepSeek 能不能自己改表、建表？

不够。`deepseek-chat` 和 `deepseek-reasoner` 已在 2026-07-24 退役。现在的执行模型是 `deepseek-flash`，它通过工具调用 `upsert_period`、`create_workbook`、`apply_chart_rules` 和 `send_takeaway`。更难的财报判断把 `DEEPSEEK_MODEL` 改成 `deepseek-v4-pro`。同一套工具也通过 MCP `earnings-sync`（`go run ./cmd/mcp`）暴露给 Cursor。DeepSeek 不靠 Cursor 的 Google MCP 建表。

## 谁有权建表

问：现在要我把表格共享给服务账号。程序像 agent，授权之后应该能自己创建表格。我不知道 `GOOGLE_SHARE_WITH` 和 Google Drive API 具体怎么补。

按 [connect.md](connect.md) 第 4 节做，或直接运行 `go run ./cmd/doctor`，看「新建表格」下面的两步：

1. 用创建服务账号的 Google 账号打开 Drive API 页面，确认顶部项目正确，点「启用」。不要再下载一把新密钥。
2. 打开项目根目录 `.env`，在 `GOOGLE_SHARE_WITH=` 等号右边写你登录 Google 表格时右上角的邮箱。不要填 JSON 里的 `client_email`。保存后不要把这一行发出来。

DeepSeek 导入时可以自己调用 `create_workbook`。Cursor 里同一套工具走 MCP `earnings-sync`。程序运行时走 Google API，不走 Cursor 自带的 Google Sheets MCP。

服务账号自己的云端硬盘没有容量。如果创建失败并提到 quota，就不要继续用服务账号建表，改为用户授权（OAuth）：浏览器弹出同意页，你本人点允许，文件直接建在你的云端硬盘里。这一步还没做。
