package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/EthanShen10086/earnings-sync/internal/config"
	"github.com/EthanShen10086/earnings-sync/internal/sheet"
)

func main() {
	tokenOut := flag.String("token", "", "写入的用户 token 路径；默认 ~/.config/earnings-sync/google-oauth-token.json")
	flag.Parse()
	settings := config.LoadSettings()
	client := settings.GoogleOAuthClient
	if client == "" {
		fmt.Fprintln(os.Stderr, `还没有 OAuth 客户端 JSON。

在 Google Cloud 控制台（与服务账号同一项目）创建「桌面应用」OAuth 客户端，下载 JSON，
写到 .env：

  GOOGLE_OAUTH_CLIENT=/绝对路径/client_secret.json

然后重新运行：go run ./cmd/googleauth

完成后把 token 路径写到：

  GOOGLE_OAUTH_TOKEN=~/.config/earnings-sync/google-oauth-token.json

之后写入你本人能打开的表格时，不必再手动共享给服务账号邮箱。`)
		os.Exit(1)
	}
	if *tokenOut == "" {
		*tokenOut = settings.GoogleOAuthToken
	}
	if *tokenOut == "" {
		home, _ := os.UserHomeDir()
		*tokenOut = filepath.Join(home, ".config", "earnings-sync", "google-oauth-token.json")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	if err := sheet.LoginOAuth(ctx, client, *tokenOut); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
