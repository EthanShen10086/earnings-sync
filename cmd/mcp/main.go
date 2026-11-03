package main

import (
	"context"
	"os"

	"github.com/EthanShen10086/earnings-sync/internal/agent"
	"github.com/EthanShen10086/earnings-sync/internal/config"
	"github.com/EthanShen10086/earnings-sync/internal/mcp"
	"github.com/EthanShen10086/earnings-sync/internal/notify"
	"github.com/EthanShen10086/earnings-sync/internal/sheet"
)

func main() {
	settings := config.LoadSettings()
	reg, err := config.Load(settings.CompaniesFile)
	if err != nil {
		os.Exit(1)
	}
	rt := agent.Runtime{Companies: reg, CredFile: settings.GoogleCreds, ShareWith: settings.GoogleShareWith}
	if settings.GoogleCreds != "" {
		if g, err := sheet.NewGoogle(context.Background(), settings.GoogleCreds); err == nil {
			rt.Sheets = g
		}
	}
	if settings.FeishuWebhook != "" {
		rt.Notifier = notify.Feishu{Webhook: settings.FeishuWebhook, Secret: settings.FeishuSecret}
	}
	if err := mcp.Serve(os.Stdin, os.Stdout, rt); err != nil {
		os.Exit(1)
	}
}
