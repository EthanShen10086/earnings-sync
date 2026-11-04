package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/EthanShen10086/earnings-sync/internal/config"
	"github.com/EthanShen10086/earnings-sync/internal/sheet"
)

func main() {
	title := flag.String("title", "", "新表格标题。空着时用公司名")
	ticker := flag.String("ticker", "", "公司代码。给出后用它的分部做表头，并把 spreadsheet id 写回配置")
	flag.Parse()
	settings := config.LoadSettings()
	if settings.GoogleCreds == "" {
		fmt.Fprintln(os.Stderr, "还没有服务账号。先看 docs/google-sheets-setup.md，再跑 go run ./cmd/doctor")
		os.Exit(1)
	}
	if settings.GoogleShareWith == "" {
		fmt.Fprintln(os.Stderr, `还不能建表。请按顺序做这两步，完整说明在 docs/connect.md 第 4 节。

1. 用创建服务账号的 Google 账号打开 Google Drive API 页面并点「启用」：
   https://console.cloud.google.com/apis/library/drive.googleapis.com
   页面最上方要是服务账号所在的项目。不要再下载新的 JSON。
2. 打开项目根目录的 .env，找到 GOOGLE_SHARE_WITH=
   等号右边写你登录 https://docs.google.com 时右上角的邮箱，例如 GOOGLE_SHARE_WITH=name@gmail.com
   不要填 JSON 里的 client_email。保存后不要把这一行发出来。

然后运行 go run ./cmd/doctor，看到「新建表格」通过后再执行本命令。`)
		os.Exit(1)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	g, err := sheet.NewGoogle(ctx, settings.GoogleCreds)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	var segments []config.Segment
	reg, err := config.Load(settings.CompaniesFile)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if *ticker != "" {
		company, ok := reg.Get(*ticker)
		if !ok {
			fmt.Fprintf(os.Stderr, "配置里没有 %s。先写进 configs/companies.yaml。\n", *ticker)
			os.Exit(1)
		}
		segments = company.Segments
		if *title == "" {
			*title = company.Name + " 财务报表"
		}
	}
	if *title == "" {
		*title = "财务报表"
	}
	id, err := g.CreateWorkbook(ctx, settings.GoogleCreds, *title, settings.GoogleShareWith, segments)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		if id != "" {
			fmt.Println("https://docs.google.com/spreadsheets/d/" + id)
		}
		os.Exit(1)
	}
	if *ticker != "" {
		if err := reg.SetSpreadsheet(*ticker, id); err != nil {
			fmt.Fprintln(os.Stderr, err)
			fmt.Println("https://docs.google.com/spreadsheets/d/" + id)
			os.Exit(1)
		}
		fmt.Printf("已创建，并写回 %s 的 spreadsheet_id。\n", strings.ToUpper(*ticker))
	} else {
		fmt.Println("已创建。打开后确认你能编辑，再把 spreadsheet id 写进 configs/companies.yaml。")
	}
	fmt.Println("https://docs.google.com/spreadsheets/d/" + id)
}
