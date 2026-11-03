package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/EthanShen10086/earnings-sync/internal/config"
	"github.com/EthanShen10086/earnings-sync/internal/extract"
	"github.com/EthanShen10086/earnings-sync/internal/notify"
	"github.com/EthanShen10086/earnings-sync/internal/sheet"

	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
)

type check struct {
	name string
	ok   bool
	hint string
}

func main() {
	settings := config.LoadSettings()
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	var results []check
	results = append(results, checkFeishu(ctx, settings))
	results = append(results, checkModel(ctx, settings))
	results = append(results, checkSheet(ctx, settings))
	results = append(results, checkCreate(ctx, settings))
	failed := 0
	fmt.Println("earnings-sync 连通性")
	fmt.Println("逐步接通说明在 docs/connect.md。下面没通过的一项，按它写的页面和 .env 那一行去做。不要把填好的内容发到对话里。")
	for _, item := range results {
		mark := "通过"
		if !item.ok {
			mark = "未通过"
			failed++
		}
		fmt.Printf("\n[%s] %s\n%s\n", mark, item.name, item.hint)
	}
	if failed > 0 {
		fmt.Printf("\n还有 %d 项没接上。改的是项目根目录的 .env，保存后再跑一次 go run ./cmd/doctor\n", failed)
		if results[0].ok && results[1].ok && results[2].ok && !results[3].ok {
			fmt.Println("飞书、DeepSeek 和已有的 Apple 表已经能用。新建表格只差上面「新建表格」里的那两步。")
		}
		os.Exit(1)
	}
	fmt.Println("\n四项都通了。先启动服务，再导入，不要加 dry_run：")
	fmt.Println("  go run ./cmd/server -conf trpc_go.yaml")
	fmt.Println("  curl -F ticker=AAPL -F file=@testdata/aapl_2026q4.csv http://127.0.0.1:8090/v1/ingest")
	fmt.Println("新建一张三张报表：")
	fmt.Println("  go run ./cmd/sheetcreate -title \"公司名 财务报表\"")
}

func checkFeishu(ctx context.Context, s config.Settings) check {
	if s.FeishuWebhook == "" {
		return check{name: "飞书", hint: `还没有 FEISHU_WEBHOOK_URL。
1. 打开目标飞书群 → 设置 → 群机器人 → 添加机器人 → 自定义机器人
2. 安全设置打开「签名校验」
3. 把 Webhook 地址写到 .env 的 FEISHU_WEBHOOK_URL
4. 把签名密钥写到 .env 的 FEISHU_WEBHOOK_SECRET
5. 再跑 go run ./cmd/doctor，群里会出现一条连通性测试`}
	}
	err := (notify.Feishu{Webhook: s.FeishuWebhook, Secret: s.FeishuSecret}).Send(ctx, notify.Message{
		Ticker:   "earnings-sync",
		Period:   "连通性测试",
		Takeaway: []string{"飞书机器人已接通。这不是财报，只是确认消息能发出来。"},
	})
	if err != nil {
		return check{name: "飞书", hint: err.Error() + "\n确认 webhook 和签名密钥来自同一个自定义机器人。"}
	}
	extra := "没有签名密钥。"
	if s.FeishuSecret != "" {
		extra = "已使用签名密钥。"
	}
	return check{name: "飞书", ok: true, hint: "测试消息已发到群里。" + extra}
}

func checkModel(ctx context.Context, s config.Settings) check {
	if s.ModelAPIKey == "" {
		return check{name: "模型", hint: fmt.Sprintf(`还没有 DEEPSEEK_API_KEY。
1. 打开 https://platform.deepseek.com/api_keys 创建密钥
2. 写到 .env 的 DEEPSEEK_API_KEY
当前会请求的地址是 %s ，模型名是 %s 。
换别的 OpenAI 兼容模型时，只改 DEEPSEEK_BASE_URL 和 DEEPSEEK_MODEL，代码不用动。`, s.ModelBaseURL, s.ModelName)}
	}
	p := extract.HTTPProvider{NameStr: "deepseek", BaseURL: s.ModelBaseURL, APIKey: s.ModelAPIKey, Model: s.ModelName}
	turn, usage, err := p.Chat(ctx, []extract.ChatMessage{{Role: "user", Content: "调用 ping 工具，不要直接回答。"}}, []extract.ToolDef{{
		Name: "ping", Description: "确认工具通道可用", Parameters: map[string]any{"type": "object", "properties": map[string]any{}},
	}})
	if err != nil {
		return check{name: "模型", hint: fmt.Sprintf("调用 %s 失败：%s\n执行模型请用 deepseek-flash。更难的判断可改 DEEPSEEK_MODEL=deepseek-v4-pro。deepseek-chat 已退役，不能只靠对话。", p.AgentModel(), err.Error())}
	}
	if len(turn.ToolCalls) == 0 || turn.ToolCalls[0].Name != "ping" {
		return check{name: "模型", hint: "模型有响应，但没有调用工具。请把 DEEPSEEK_MODEL 设为 deepseek-flash 或 deepseek-v4-pro，不要用已退役的 deepseek-chat。"}
	}
	note := ""
	if s.ModelName == "deepseek-chat" || s.ModelName == "deepseek-reasoner" || s.ModelName == "" {
		note = "配置里的旧模型名已自动改用 deepseek-flash。"
	}
	return check{name: "模型", ok: true, hint: fmt.Sprintf("工具调用已接通，实际请求的是 %s，本次消耗 %d token。%s", p.AgentModel(), usage.Total, note)}
}

func checkSheet(ctx context.Context, s config.Settings) check {
	const sheetID = "1ePLqJ8emZAZ-cYp90w5TvA8ptAxjLFr2CcrL9zQWEE0"
	if s.GoogleCreds == "" {
		return check{name: "Google 表格", hint: `还没有 GOOGLE_APPLICATION_CREDENTIALS。
1. 打开 https://console.cloud.google.com/apis/library/sheets.googleapis.com 启用 Google Sheets API
2. 打开 https://console.cloud.google.com/iam-admin/serviceaccounts 新建服务账号
3. 在该账号下「密钥 → 添加密钥 → JSON」，把下载的文件路径写到 .env 的 GOOGLE_APPLICATION_CREDENTIALS
4. 用文本编辑器打开那个 JSON，复制 client_email
5. 打开 https://docs.google.com/spreadsheets/d/` + sheetID + `/edit ，点共享，把这个邮箱加为编辑者
6. 再跑 go run ./cmd/doctor`}
	}
	if st, err := os.Stat(s.GoogleCreds); err != nil || st.IsDir() {
		return check{name: "Google 表格", hint: "GOOGLE_APPLICATION_CREDENTIALS 指向的文件不存在：" + s.GoogleCreds}
	}
	g, err := sheet.NewGoogle(ctx, s.GoogleCreds)
	if err != nil {
		return check{name: "Google 表格", hint: "服务账号文件打不开。确认 GOOGLE_APPLICATION_CREDENTIALS 指向你下载的 JSON，且没有把文件内容贴到对话里。"}
	}
	if err := g.Ping(ctx, sheetID); err != nil {
		return check{name: "Google 表格", hint: "服务账号还读不到「利润表」。请你本人打开 JSON 里的 client_email，把表格共享给它并设为编辑者。不要把邮箱发到对话里。\n" + err.Error()}
	}
	return check{name: "Google 表格", ok: true, hint: "已读到「利润表」表头。导入时只改黄色录入格，灰色公式列不用手改。"}
}

func checkCreate(ctx context.Context, s config.Settings) check {
	if s.GoogleCreds == "" {
		return check{name: "新建表格", hint: "先做完上面的「Google 表格」。已有表能读之后，这里会写出启用 Drive API 的页面，以及 .env 里 GOOGLE_SHARE_WITH 该怎么填。"}
	}
	projectID := credProjectID(s.GoogleCreds)
	if strings.TrimSpace(s.GoogleShareWith) == "" {
		return check{name: "新建表格", hint: shareMissingHint(projectID)}
	}
	email := strings.TrimSpace(s.GoogleShareWith)
	if strings.ContainsAny(email, " \t\"'") || !strings.Contains(email, "@") || strings.HasSuffix(strings.ToLower(email), ".gserviceaccount.com") {
		return check{name: "新建表格", hint: `GOOGLE_SHARE_WITH 填错了。
打开项目根目录的 .env，找到 GOOGLE_SHARE_WITH= 这一行。
等号右边换成你登录 https://docs.google.com 时右上角头像里的邮箱，不要加引号，等号两边不要空格。
不要填 JSON 里的 client_email，那是服务账号，以 .gserviceaccount.com 结尾。
保存后再运行 go run ./cmd/doctor。不要把这一行发到对话里。`}
	}
	if err := probeDrive(ctx, s.GoogleCreds); err != nil {
		return check{name: "新建表格", hint: driveDisabledHint(projectID, err)}
	}
	return check{name: "新建表格", ok: true, hint: "Drive API 可用。新表会共享给 GOOGLE_SHARE_WITH 里的邮箱，程序不会把邮箱打印出来。\n创建：go run ./cmd/sheetcreate -title \"公司名 财务报表\""}
}

func shareMissingHint(projectID string) string {
	return `已有的 Apple 表不用做这两步。新建表需要它们。

第一步，启用 Google Drive API。用创建服务账号的那个 Google 账号打开：
` + driveEnableURL(projectID) + `
页面最上方必须是服务账号所在的项目。点「启用」。按钮变成「管理」就好了。不要再创建凭据，也不要再下载新的 JSON。

第二步，打开项目根目录的 .env，找到这一行：
GOOGLE_SHARE_WITH=
等号右边写你登录 https://docs.google.com 时右上角头像里的邮箱，例如：
GOOGLE_SHARE_WITH=name@gmail.com
不要填 JSON 里的 client_email。不要加引号，等号两边不要空格。保存。不要把这一行发到对话里。

然后重新运行 go run ./cmd/doctor。完整说明在 docs/connect.md 第 4 节。`
}

func driveDisabledHint(projectID string, err error) string {
	return "邮箱已经写进 .env，但这个 Cloud 项目还不能调用 Google Drive API。\n1. 打开 " + driveEnableURL(projectID) + "\n2. 确认页面最上方是服务账号所在的项目\n3. 点「启用」，等一两分钟\n4. 再跑 go run ./cmd/doctor\n\n" + err.Error()
}

func driveEnableURL(projectID string) string {
	if projectID == "" {
		return "https://console.cloud.google.com/apis/library/drive.googleapis.com"
	}
	return "https://console.cloud.google.com/apis/library/drive.googleapis.com?project=" + projectID
}

func credProjectID(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var meta struct {
		ProjectID string `json:"project_id"`
	}
	if json.Unmarshal(b, &meta) != nil {
		return ""
	}
	return meta.ProjectID
}

func probeDrive(ctx context.Context, credFile string) error {
	drv, err := drive.NewService(ctx, option.WithCredentialsFile(credFile), option.WithScopes("https://www.googleapis.com/auth/drive.file"))
	if err != nil {
		return err
	}
	_, err = drv.Files.List().PageSize(1).Fields("files(id)").Context(ctx).Do()
	return err
}
