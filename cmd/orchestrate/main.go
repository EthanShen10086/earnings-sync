// Command orchestrate runs a YAML AI flow with the local orchestration engine.
//
// Example:
//
//	go run ./cmd/orchestrate -flow configs/flows/earnings-ingest.flow.yaml -input ticker=AAPL -input dry_run=true
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"

	"github.com/EthanShen10086/earnings-sync/internal/extract"
	"github.com/EthanShen10086/earnings-sync/internal/orchestrator"
)

func main() {
	flowPath := flag.String("flow", "configs/flows/earnings-ingest.flow.yaml", "flow YAML 路径")
	mock := flag.Bool("mock-model", true, "用 MockChatter，不调用 DeepSeek（默认 true）")
	verbose := flag.Bool("v", true, "打印节点事件")
	clientReq := flag.String("client-request-id", "", "幂等键（同 ftp3_runtime_hub client_request_id）；重复调用复用结果")
	var inputs inputFlags
	flag.Var(&inputs, "input", "流程输入，可重复，形如 key=value（value 支持 true/false/数字/JSON）")
	flag.Parse()

	def, err := orchestrator.LoadFile(*flowPath)
	if err != nil {
		fatal(err)
	}
	reg := orchestrator.Registry{Tools: orchestrator.BuiltinTools()}
	if *mock {
		reg.Chatter = orchestrator.MockChatter{Content: "样例：已完成抽取与分支判断（mock 模型）"}
	} else {
		apiKey := os.Getenv("DEEPSEEK_API_KEY")
		base := os.Getenv("DEEPSEEK_BASE_URL")
		model := os.Getenv("DEEPSEEK_MODEL")
		if apiKey == "" {
			fatal(fmt.Errorf("未设置 DEEPSEEK_API_KEY；或改用 -mock-model=true"))
		}
		if base == "" {
			base = "https://api.deepseek.com"
		}
		reg.Chatter = extract.HTTPProvider{APIKey: apiKey, BaseURL: base, Model: model}
	}

	events := make(chan orchestrator.Event, 64)
	eng := &orchestrator.Engine{Registry: reg, Events: events}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	done := make(chan struct{})
	go func() {
		defer close(done)
		for ev := range events {
			if !*verbose {
				continue
			}
			switch ev.Kind {
			case "node_start":
				fmt.Fprintf(os.Stderr, "→ %s 开始\n", ev.NodeID)
			case "node_end":
				if ev.Error != "" {
					fmt.Fprintf(os.Stderr, "✗ %s %s: %s\n", ev.NodeID, ev.Status, ev.Error)
				} else {
					fmt.Fprintf(os.Stderr, "✓ %s %s\n", ev.NodeID, ev.Status)
				}
			case "run_end":
				fmt.Fprintf(os.Stderr, "flow %s\n", ev.Status)
			}
		}
	}()

	state, err := eng.Run(ctx, def, orchestrator.RunOptions{
		ClientRequestID: *clientReq,
		Inputs:          inputs.Map(),
	})
	close(events)
	<-done
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if state != nil {
		_ = enc.Encode(state)
	}
	if err != nil {
		fatal(err)
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err.Error())
	os.Exit(1)
}

type inputFlags []string

func (f *inputFlags) String() string { return strings.Join(*f, ",") }
func (f *inputFlags) Set(v string) error {
	*f = append(*f, v)
	return nil
}
func (f inputFlags) Map() map[string]any {
	out := make(map[string]any, len(f))
	for _, item := range f {
		k, v, ok := strings.Cut(item, "=")
		if !ok {
			out[item] = true
			continue
		}
		out[k] = parseValue(v)
	}
	return out
}

func parseValue(v string) any {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "true":
		return true
	case "false":
		return false
	}
	if n, err := strconv.Atoi(v); err == nil {
		return n
	}
	if n, err := strconv.ParseFloat(v, 64); err == nil {
		return n
	}
	if strings.HasPrefix(v, "{") || strings.HasPrefix(v, "[") {
		var anyJSON any
		if err := json.Unmarshal([]byte(v), &anyJSON); err == nil {
			return anyJSON
		}
	}
	return v
}
