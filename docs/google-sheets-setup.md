# 接通 Google 表格

完整接通顺序，包括飞书、DeepSeek，以及新建表格要用的 Google Drive API 和 `GOOGLE_SHARE_WITH`，看 [connect.md](connect.md)。下面只保留「改一张已经存在的表」这几步。

服务用一个 Google 服务账号往表格里写黄色录入格。账号邮箱和 JSON 密钥只留在你自己的电脑上，不要发到聊天、不要提交到 Git。

完成后运行：

```bash
go run ./cmd/doctor
```

「Google 表格」显示通过，就可以导入已有的 Apple 表。要新建表，还要做 [connect.md](connect.md) 第 4 节，直到「新建表格」也通过。

## 你本人操作的部分

下面几步会看到私钥或账号邮箱。请自己在浏览器里完成，不要把页面内容贴出来。

1. 创建并下载服务账号的 JSON 密钥。
2. 打开 JSON，复制其中的 `client_email`。
3. 把这个邮箱加为表格的编辑者。
4. 把 JSON 文件路径写进项目根目录 `.env` 的 `GOOGLE_APPLICATION_CREDENTIALS`。

`.env` 已在 `.gitignore` 里。

## 页面顺序

1. 选择或新建一个 Google Cloud 项目。不要用含有其他生产密钥的项目，单独建一个即可。
2. 打开 [Google Sheets API](https://console.cloud.google.com/apis/library/sheets.googleapis.com)，确认右上角项目无误，点启用。
3. 打开 [服务账号](https://console.cloud.google.com/iam-admin/serviceaccounts)。
4. 创建服务账号。名称可以用 `earnings-sync`。不需要给它项目级的 Owner 角色。
5. 进入该账号，打开「密钥」，添加 JSON 密钥并下载。下载完成后立刻关掉显示密钥的页面。
6. 把文件放到本机只有你能读的位置，例如 `~/.config/earnings-sync/sheets.json`，然后执行 `chmod 600` 该文件。
7. 在 `.env` 写入：

```bash
GOOGLE_APPLICATION_CREDENTIALS=/绝对路径/sheets.json
```

8. 打开 [Apple 表格](https://docs.google.com/spreadsheets/d/1ePLqJ8emZAZ-cYp90w5TvA8ptAxjLFr2CcrL9zQWEE0/edit)，点共享，把 JSON 里的 `client_email` 加为编辑者。不要把链接改成「知道链接的任何人」。

## 怎样算接上了

`go run ./cmd/doctor` 里 Google 一项为通过，表示服务账号能读到「利润表」的表头。这一步不会把邮箱或密钥打印出来。

随后：

```bash
go run ./cmd/server -conf trpc_go.yaml
curl -F ticker=AAPL -F file=@testdata/aapl_2026q4.csv http://127.0.0.1:8090/v1/ingest
```

成功时，2026 财年第四季度那一行的黄色格子会更新，灰色公式和图表跟着变。
