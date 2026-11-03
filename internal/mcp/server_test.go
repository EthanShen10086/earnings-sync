package mcp

import (
	"bytes"
	"strconv"
	"strings"
	"testing"

	"github.com/EthanShen10086/earnings-sync/internal/agent"
	"github.com/EthanShen10086/earnings-sync/internal/config"
)

func TestToolsListAndCall(t *testing.T) {
	reg, err := config.Load("../../configs/companies.yaml")
	if err != nil {
		t.Fatal(err)
	}
	body := `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`
	req := "Content-Length: " + strconv.Itoa(len(body)) + "\r\n\r\n" + body
	var out bytes.Buffer
	if err := Serve(strings.NewReader(req), &out, agent.Runtime{Companies: reg, DryRun: true}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "upsert_period") || !strings.Contains(out.String(), "create_workbook") {
		t.Fatal(out.String())
	}
}
