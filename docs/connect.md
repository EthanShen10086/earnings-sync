# 接通并开始用

在项目根目录打开 `.env`。每一项都是一行 `名字=值`，等号两边不要空格，值不要加引号。改完保存，再运行：

```bash
go run ./cmd/doctor
```

没通过的那一项下面就是下一步该点哪里、该改哪一行。不要把填好的 `.env`、JSON 或邮箱发到对话里。

飞书、DeepSeek、已有表格、新建表格，这四项都显示「通过」之后，再做文末的导入。

## 1. 飞书

程序把每季要点发到一个飞书群。

1. 打开要接收消息的飞书群。
2. 点右上角设置 → 群机器人 → 添加机器人 → 自定义机器人。
3. 安全设置打开「签名校验」，然后完成添加。
4. 页面上会同时给出两样东西，必须来自这同一个机器人：
   - Webhook 地址，写到 `.env` 的 `FEISHU_WEBHOOK_URL=`
   - 签名密钥，写到 `.env` 的 `FEISHU_WEBHOOK_SECRET=`
5. 保存 `.env`，运行 `go run ./cmd/doctor`。目标群里会出现一条「连通性测试」。看到那条消息，飞书就接上了。

签名校验开着、但密钥空着或配错机器人时，doctor 会写明发送失败。回到同一个机器人的设置里，重新复制这两个值。

## 2. DeepSeek

程序用 DeepSeek 读财报，并自己决定写表、建表、补图、发飞书。这不是普通对话，模型必须能调用工具。

1. 打开 https://platform.deepseek.com/api_keys ，创建一个 API key。
2. 写到 `.env` 的 `DEEPSEEK_API_KEY=`。
3. 确认这两行是：

```bash
DEEPSEEK_BASE_URL=https://api.deepseek.com
DEEPSEEK_MODEL=deepseek-flash
```

4. 保存后再跑 doctor。「模型」通过时，会写明实际请求的是 `deepseek-flash`。

`deepseek-chat` 和 `deepseek-reasoner` 已经退役。更难的财报判断可以把 `DEEPSEEK_MODEL` 改成 `deepseek-v4-pro`。换成别的 OpenAI 兼容服务时，只改 `DEEPSEEK_BASE_URL` 和 `DEEPSEEK_MODEL`。

## 3. 改已经存在的 Google 表格

Apple 那张表已经在用。这一节是以后换电脑、或别人第一次接时要做的事。

1. 打开 https://console.cloud.google.com/apis/library/sheets.googleapis.com ，确认左上角是创建服务账号的那个项目，点「启用」。已经是「管理」就不用再点。
2. 打开 https://console.cloud.google.com/iam-admin/serviceaccounts ，进入服务账号，打开「密钥」，添加 JSON 密钥并下载。下载后关掉页面，不要把文件内容贴出来。
3. 把 JSON 放到只有你能读的位置，然后在终端执行 `chmod 600 那个文件的绝对路径`。
4. `.env` 写绝对路径：

```bash
GOOGLE_APPLICATION_CREDENTIALS=/你的路径/服务账号.json
```

5. 用编辑器打开 JSON，只复制 `client_email` 的值。它以 `.gserviceaccount.com` 结尾。
6. 打开表格 → 共享 → 把这个邮箱加为编辑者。不要把链接改成「知道链接的任何人」。
7. 再跑 doctor。「Google 表格」通过，表示服务账号能读到「利润表」表头。

当前这台机器上，Sheets API、服务账号 JSON 和 Apple 表的共享已经做过。doctor 里这一项如果是通过，就不要重做。

## 4. 让程序能新建表格

已有的 Apple 表不依赖这一节。新建 Alphabet、Tesla 或任何新公司的三张表时，需要下面两步。服务账号自己的云端硬盘没有容量，所以新表必须共享到你本人的 Google 账号，你才能在自己的网盘里打开。

### 4.1 启用 Google Drive API

用创建服务账号时的同一个 Google 账号打开：

https://console.cloud.google.com/apis/library/drive.googleapis.com?project=plucky-environs-464307-a8

1. 看页面最上方的项目名。它必须是服务账号所在的项目。现在是「My First Project」。如果不是，先在顶部换成这个项目，不要新建项目。
2. 点蓝色的「启用」。
3. 按钮变成「管理」，或页面显示 API 已启用，这一步就完成了。刚启用的话等一两分钟再继续。
4. 这里不要再点「创建凭据」，也不要再下载一把新密钥。程序继续用你已经放好的那个 JSON。

### 4.2 在 .env 写下你自己的邮箱

1. 打开项目根目录的 `.env`。Cursor 里可以直接打开这个文件。
2. 找到这一行：

```bash
GOOGLE_SHARE_WITH=
```

3. 在等号右边写你登录 https://docs.google.com 时、右上角头像里的那个邮箱。保存。形式如下，把示例换成你的邮箱：

```bash
GOOGLE_SHARE_WITH=name@gmail.com
```

填的时候注意：

- 填你自己的 Google 登录邮箱。新表会以编辑者身份出现在这个账号里。
- 不要填 JSON 里的 `client_email`。那是服务账号，以 `.gserviceaccount.com` 结尾，填了也看不到新表。
- 不要加引号，等号两边不要空格。
- 不要把这一行发到对话里。

4. 保存后运行：

```bash
go run ./cmd/doctor
```

「新建表格」显示通过，就可以建第一张新表：

```bash
go run ./cmd/sheetcreate -title "Alphabet 财务报表"
```

终端只打印一个 `https://docs.google.com/spreadsheets/d/...` 链接。用你刚填的邮箱打开，能编辑就对了。然后把链接里 `/d/` 和后面 `/` 之间的那段 id，写进 `configs/companies.yaml` 里对应公司的 `spreadsheet_id`。

如果 doctor 或建表报错里出现 quota，就是服务账号的网盘没有容量。不要换一个服务账号重试。等用户授权（浏览器里你本人点允许，文件直接建在你的网盘）做完之后再用那条路。这一步程序还没做。

## 5. 导入一次财报

四项都通过后，在项目根目录：

```bash
go run ./cmd/server -conf trpc_go.yaml
```

另开一个终端：

```bash
curl -F ticker=AAPL -F file=@testdata/aapl_2026q4.csv http://127.0.0.1:8090/v1/ingest
```

不要加 `dry_run`。成功时：

- Apple 表里这一季的黄色格子被写上数，包括资产负债表，不只是利润表。灰色公式列自己重算，旁边的图跟着变。以后再导入新的财季，同一批图会把新行画进去。
- 飞书群收到中文解读：哪些数异常好、哪些异常坏、业务结构、靠什么赚钱、资产负债表健康度、公司质地。
- 英文财报原文会先收成表格文本，再由 DeepSeek 解析一次。已经排好的 CSV 不会重复送给模型。现金流量如果原文只有全年数，单季数用 SEC 的累计数相减得到。
- 同一份文件再传一次不会重复写，也不会重复发。先前如果只做过 dry run，正式导入仍会执行。

结构化 CSV 里可以放多个财季。程序一次解析，只读一次期间列，再用一次 Google 批量写入把所有黄色格子写完。这些数字不会再送给模型逐格抄写。模型只根据最新一季写一段短 takeaway。PDF 或网页才走 DeepSeek 的工具循环。

只想看抽取结果、不改表、不发飞书时，才加 `-F dry_run=true`。

其他入口：

```bash
curl -H 'content-type: application/json' -d '{"ticker":"AAPL","url":"https://example.com/q4.html"}' http://127.0.0.1:8090/v1/ingest
curl -H 'content-type: application/json' -d '{"ticker":"AAPL"}' http://127.0.0.1:8090/v1/watch
```

DeepSeek 接到导入后，可以自己调用这些操作，而不是只写一段摘要：查看已登记公司、把一季数字写入黄色格子、没有表时新建三张表、按规则补图、把要点发到飞书。Cursor 里用同一套操作时，启用 `.cursor/mcp.json` 的 `earnings-sync`，说明在 `skills/earnings-agent/SKILL.md`。

公司名单在 `configs/companies.yaml`。Apple 用现有表格。Alphabet 和 Tesla 走 SEC，要先按第 4 节建表并填上 `spreadsheet_id`。SpaceX 没有公开 10-Q，只能手动传文件。
