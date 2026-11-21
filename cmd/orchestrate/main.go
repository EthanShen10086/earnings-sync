// Command orchestrate runs a YAML AI flow with the local orchestration engine.
//
// Examples:
//
//	# 健康检查（flow 可加载、工具齐全、store 可写）
//	go run ./cmd/orchestrate -check
//
//	# dry-run（只抽取，不写表）
//	go run ./cmd/orchestrate -flow configs/flows/earnings-ingest.flow.yaml \
//	  -input ticker=AAPL -input dry_run=true -input body_path=testdata/aapl_2026q4.csv
//
//	# 真实写入本地 Memory sheet，并导出证据 JSON
//	go run ./cmd/orchestrate -flow configs/flows/earnings-ingest.flow.yaml \
//	  -input ticker=AAPL -input dry_run=false -input body_path=testdata/aapl_2026q4.csv \
//	  -input force=true -dump-writes data/orchestrator-e2e-writes.json
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/EthanShen10086/earnings-sync/internal/extract"
	"github.com/EthanShen10086/earnings-sync/internal/notify"
	"github.com/EthanShen10086/earnings-sync/internal/observe"
	"github.com/EthanShen10086/earnings-sync/internal/orchestrator"
)

func main() {
	flowPath := flag.String("flow", "configs/flows/earnings-ingest.flow.yaml", "flow YAML 路径")
	mock := flag.Bool("mock-model", true, "用 MockChatter，不调用 DeepSeek（默认 true）")
	verbose := flag.Bool("v", true, "打印节点事件")
	check := flag.Bool("check", false, "只做健康检查后退出")
	clientReq := flag.String("client-request-id", "", "幂等键（同 ftp3_runtime_hub client_request_id）；重复调用复用结果")
	requestID := flag.String("request-id", "", "请求关联 id（空则自动生成）")
	traceID := flag.String("trace-id", "", "trace 关联 id（空则自动生成）")
	timeout := flag.Duration("timeout", 2*time.Minute, "整次 flow 超时（调用方无 deadline 时生效）")
	companies := flag.String("companies", "configs/companies.yaml", "公司配置")
	macro := flag.String("macro", "configs/macro.yaml", "宏观配置")
	jobsPath := flag.String("jobs", "data/orchestrator-jobs.json", "job store 路径")
	runsPath := flag.String("runs", "data/orchestrator-runs.json", "flow run 落盘路径（可查询）")
	eventsPath := flag.String("events", "data/orchestrator-events.json", "产品闭环事件落盘路径")
	dumpWrites := flag.String("dump-writes", "", "若使用 Memory sheet，把写入单元格导出到此 JSON")
	notifyCapture := flag.Bool("capture-notify", true, "用内存 notifier 捕获通知（不发飞书）")
	var inputs inputFlags
	flag.Var(&inputs, "input", "流程输入，可重复，形如 key=value（value 支持 true/false/数字/JSON）")
	flag.Parse()

	fileStore, err := orchestrator.OpenFileRunStore(*runsPath, *eventsPath)
	if err != nil {
		fatal(err)
	}

	var note notify.Sender
	if *notifyCapture {
		note = &captureNotify{}
	}
	bridge, err := orchestrator.NewLocalPipeline(orchestrator.LocalPipelineOpts{
		CompaniesFile: *companies,
		MacroFile:     *macro,
		JobStorePath:  *jobsPath,
		Notifier:      note,
		// Opt-in dual/pg for CLI when env is set; tests leave these empty → Memory only.
		StorageMode:   os.Getenv("STORAGE_MODE"),
		DatabaseURL:   os.Getenv("DATABASE_URL"),
		MigrationsDir: os.Getenv("MIGRATIONS_DIR"),
	})
	if err != nil {
		fatal(err)
	}

	reg := orchestrator.Registry{
		Tools: orchestrator.MergeTools(orchestrator.BuiltinTools(), bridge.Tools()),
	}

	if *check {
		rep := orchestrator.CheckHealth(orchestrator.HealthOpts{
			FlowPath:   *flowPath,
			Tools:      reg.Tools,
			RunsPath:   *runsPath,
			EventsPath: *eventsPath,
		})
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(rep)
		if rep.Status == "fail" {
			os.Exit(1)
		}
		return
	}

	def, err := orchestrator.LoadFile(*flowPath)
	if err != nil {
		fatal(err)
	}

	if *mock {
		reg.Chatter = orchestrator.MockChatter{Content: "样例：已完成 pipeline ingest（mock 模型）"}
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

	obs := observe.New()
	observer := &orchestrator.MetricsObserver{
		Inner: orchestrator.NewSlogObserver(fileStore),
		Rec:   obs,
	}

	events := make(chan orchestrator.Event, 64)
	eng := &orchestrator.Engine{
		Registry:       reg,
		Events:         events,
		Runs:           fileStore.Memory(),
		Observer:       observer,
		DefaultTimeout: *timeout,
	}
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
					fmt.Fprintf(os.Stderr, "✓ %s %s (%dms)\n", ev.NodeID, ev.Status, ev.DurationMs)
				}
			case "run_end":
				fmt.Fprintf(os.Stderr, "flow %s (%dms) request_id=%s trace_id=%s\n", ev.Status, ev.DurationMs, ev.RequestID, ev.TraceID)
			}
		}
	}()

	state, err := eng.Run(ctx, def, orchestrator.RunOptions{
		ClientRequestID: *clientReq,
		RequestID:       *requestID,
		TraceID:         *traceID,
		Inputs:          inputs.Map(),
		Timeout:         *timeout,
	})
	close(events)
	<-done

	if *dumpWrites != "" {
		if dumpErr := bridge.DumpWrites(*dumpWrites); dumpErr != nil {
			fmt.Fprintf(os.Stderr, "dump-writes: %v\n", dumpErr)
		} else {
			abs, _ := filepath.Abs(*dumpWrites)
			fmt.Fprintf(os.Stderr, "wrote evidence %s (%d cells)\n", abs, len(bridge.Memory.Writes))
		}
	}

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

type captureNotify struct{ n int }

func (c *captureNotify) Send(context.Context, notify.Message) error {
	c.n++
	return nil
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
