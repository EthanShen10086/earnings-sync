package orchestrator

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/EthanShen10086/earnings-sync/internal/notify"
)

func TestRunHappyPathWithBranch(t *testing.T) {
	yaml := `
title: demo
concurrency: 2
inputs:
  ticker:
    type: text
    required: true
  dry_run:
    type: json
    required: true
output:
  from: node
  nodeId: summarize
nodes:
  - id: validate
    type: tool
    tool: validate_ingest
    input:
      ticker: { from: input, key: ticker }
      dry_run: { from: input, key: dry_run }
  - id: extract
    type: tool
    tool: extract_facts_stub
    needs: [validate]
    input:
      ticker: { from: node, nodeId: validate, path: ticker }
  - id: decide
    type: condition
    needs: [validate]
    equals: true
    branchTrue: dry
    branchFalse: live
    input:
      value: { from: node, nodeId: validate, path: dry_run }
  - id: write
    type: tool
    tool: upsert_period_stub
    needs: [extract, decide]
    when:
      from: node
      nodeId: decide
      path: branch
      equals: live
    input:
      ticker: { from: node, nodeId: extract, path: ticker }
  - id: summarize
    type: agent
    needs: [extract]
    needsMode: all
    onFailure: continue
    prompt: 用一句话总结抽取结果
    input:
      extraction: { from: node, nodeId: extract }
`
	def, err := Parse([]byte(yaml))
	if err != nil {
		t.Fatal(err)
	}
	eng := &Engine{Registry: Registry{Tools: BuiltinTools(), Chatter: MockChatter{Content: "抽取完成"}}}
	state, err := eng.Run(context.Background(), def, RunOptions{Inputs: map[string]any{
		"ticker":  "AAPL",
		"dry_run": true,
	}})
	if err != nil {
		t.Fatal(err)
	}
	if state.Status != StatusSucceeded {
		t.Fatalf("status=%s err=%s", state.Status, state.Error)
	}
	if state.Nodes["write"].Status != StatusSkipped {
		t.Fatalf("dry_run 时应跳过 write, got %s", state.Nodes["write"].Status)
	}
	if state.Nodes["summarize"].Status != StatusSucceeded {
		t.Fatalf("summarize=%s", state.Nodes["summarize"].Status)
	}
	out, ok := state.Output.(map[string]any)
	if !ok {
		t.Fatalf("output type %T", state.Output)
	}
	if fmt.Sprint(out["content"]) != "抽取完成" {
		t.Fatalf("unexpected output: %#v", state.Output)
	}
}

func TestRetryThenSucceed(t *testing.T) {
	var calls atomic.Int32
	tools := BuiltinTools()
	tools["flaky"] = func(_ context.Context, _ any) (any, error) {
		n := calls.Add(1)
		if n < 3 {
			return nil, fmt.Errorf("try %d", n)
		}
		return map[string]any{"ok": true, "tries": n}, nil
	}
	def := FlowDefinition{
		Title: "retry",
		Nodes: []Node{{
			ID:    "flaky",
			Type:  "tool",
			Tool:  "flaky",
			Retry: &RetryPolicy{MaxAttempts: 3, BackoffMs: 1},
		}},
		Output: map[string]any{"from": "node", "nodeId": "flaky"},
	}
	eng := &Engine{Registry: Registry{Tools: tools}}
	state, err := eng.Run(context.Background(), def, RunOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if state.Status != StatusSucceeded {
		t.Fatalf("status=%s", state.Status)
	}
	if calls.Load() != 3 {
		t.Fatalf("calls=%d", calls.Load())
	}
	if len(state.Nodes["flaky"].Attempts) != 3 {
		t.Fatalf("attempts=%d", len(state.Nodes["flaky"].Attempts))
	}
}

func TestOnFailureContinue(t *testing.T) {
	tools := BuiltinTools()
	tools["boom"] = func(_ context.Context, _ any) (any, error) {
		return nil, fmt.Errorf("boom")
	}
	def := FlowDefinition{
		Concurrency: 2,
		Nodes: []Node{
			{ID: "boom", Type: "tool", Tool: "boom", OnFailure: "continue"},
			{ID: "ok", Type: "tool", Tool: "echo", Input: "hello"},
		},
	}
	eng := &Engine{Registry: Registry{Tools: tools}}
	state, err := eng.Run(context.Background(), def, RunOptions{})
	if err == nil {
		t.Fatal("expected flow failure because a node failed")
	}
	if state.Nodes["ok"].Status != StatusSucceeded {
		t.Fatalf("ok node should succeed despite boom, got %s", state.Nodes["ok"].Status)
	}
	if state.Nodes["boom"].Status != StatusFailed {
		t.Fatalf("boom=%s", state.Nodes["boom"].Status)
	}
}

func TestOnFailureStopSkipsDependents(t *testing.T) {
	tools := BuiltinTools()
	tools["boom"] = func(_ context.Context, _ any) (any, error) {
		return nil, fmt.Errorf("boom")
	}
	def := FlowDefinition{
		Nodes: []Node{
			{ID: "boom", Type: "tool", Tool: "boom", OnFailure: "stop"},
			{ID: "after", Type: "tool", Tool: "echo", Needs: []string{"boom"}, Input: "nope"},
		},
	}
	eng := &Engine{Registry: Registry{Tools: tools}}
	state, err := eng.Run(context.Background(), def, RunOptions{})
	if err == nil {
		t.Fatal("expected failure")
	}
	if state.Nodes["after"].Status != StatusSkipped {
		t.Fatalf("after=%s want skipped", state.Nodes["after"].Status)
	}
}

func TestNeedsModeAny(t *testing.T) {
	tools := BuiltinTools()
	tools["boom"] = func(_ context.Context, _ any) (any, error) {
		return nil, fmt.Errorf("boom")
	}
	def := FlowDefinition{
		Nodes: []Node{
			{ID: "boom", Type: "tool", Tool: "boom", OnFailure: "continue"},
			{ID: "ok", Type: "tool", Tool: "echo", Input: "hi", OnFailure: "continue"},
			{
				ID:        "join",
				Type:      "tool",
				Tool:      "echo",
				Needs:     []string{"boom", "ok"},
				NeedsMode: "any",
				Input:     map[string]any{"from": "node", "nodeId": "ok"},
			},
		},
	}
	eng := &Engine{Registry: Registry{Tools: tools}}
	state, err := eng.Run(context.Background(), def, RunOptions{})
	if err == nil {
		t.Fatal("expected flow failure from boom")
	}
	if state.Nodes["join"].Status != StatusSucceeded {
		t.Fatalf("join=%s want succeeded via needsMode=any", state.Nodes["join"].Status)
	}
}

func TestMissingRequiredInput(t *testing.T) {
	def := FlowDefinition{
		Inputs: map[string]InputSpec{"ticker": {Required: true}},
		Nodes:  []Node{{ID: "echo", Type: "tool", Tool: "echo", Input: "x"}},
	}
	eng := &Engine{Registry: Registry{Tools: BuiltinTools()}}
	if _, err := eng.Run(context.Background(), def, RunOptions{}); err == nil {
		t.Fatal("expected missing input error")
	}
}

func TestLoadDemoFile(t *testing.T) {
	def, err := LoadFile("../../configs/flows/earnings-ingest.flow.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if def.Title == "" || len(def.Nodes) < 3 {
		t.Fatalf("unexpected def: %+v", def)
	}
	found := false
	for _, n := range def.Nodes {
		if n.Tool == "pipeline_ingest" {
			found = true
		}
	}
	if !found {
		t.Fatal("flow must call pipeline_ingest")
	}
}

func TestClientRequestIDIdempotent(t *testing.T) {
	def := FlowDefinition{
		Nodes: []Node{{ID: "echo", Type: "tool", Tool: "echo", Input: "hi"}},
	}
	eng := &Engine{Registry: Registry{Tools: BuiltinTools()}, Runs: NewRunStore()}
	first, err := eng.Run(context.Background(), def, RunOptions{ClientRequestID: "req-1"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := eng.Run(context.Background(), def, RunOptions{ClientRequestID: "req-1"})
	if err != nil {
		t.Fatal(err)
	}
	if first.RunID != second.RunID {
		t.Fatalf("expected same run id, got %s vs %s", first.RunID, second.RunID)
	}
}

func TestRequestTraceIDsAndProductEvents(t *testing.T) {
	dir := t.TempDir()
	fs, err := OpenFileRunStore(filepath.Join(dir, "runs.json"), filepath.Join(dir, "events.json"))
	if err != nil {
		t.Fatal(err)
	}
	obs := NewSlogObserver(fs)
	eng := &Engine{
		Registry: Registry{Tools: BuiltinTools()},
		Runs:     fs.Memory(),
		Observer: obs,
	}
	state, err := eng.Run(context.Background(), FlowDefinition{
		Nodes: []Node{{ID: "echo", Type: "tool", Tool: "echo", Input: "hi"}},
	}, RunOptions{RequestID: "req-abc", TraceID: "trc-xyz"})
	if err != nil {
		t.Fatal(err)
	}
	if state.RequestID != "req-abc" || state.TraceID != "trc-xyz" {
		t.Fatalf("ids=%s/%s", state.RequestID, state.TraceID)
	}
	if state.DurationMs < 0 {
		t.Fatal("missing duration")
	}
	loaded, ok := fs.Get(state.RunID)
	if !ok || loaded.Status != StatusSucceeded {
		t.Fatalf("persisted run missing: ok=%v %#v", ok, loaded)
	}
	evs := fs.Events(20)
	names := map[string]bool{}
	for _, ev := range evs {
		names[ev.Name] = true
		if ev.RequestID != "req-abc" || ev.TraceID != "trc-xyz" {
			t.Fatalf("event ids %#v", ev)
		}
	}
	for _, want := range []string{"flow.start", "step.success", "flow.success"} {
		if !names[want] {
			t.Fatalf("missing product event %s in %#v", want, names)
		}
	}
	// reload from disk
	fs2, err := OpenFileRunStore(filepath.Join(dir, "runs.json"), filepath.Join(dir, "events.json"))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := fs2.Get(state.RunID); !ok {
		t.Fatal("run not reloaded from disk")
	}
	if len(fs2.Events(0)) < 3 {
		t.Fatalf("events not reloaded: %d", len(fs2.Events(0)))
	}
}

func TestFlowTimeout(t *testing.T) {
	tools := BuiltinTools()
	tools["slow"] = func(ctx context.Context, _ any) (any, error) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(2 * time.Second):
			return map[string]any{"ok": true}, nil
		}
	}
	eng := &Engine{
		Registry:       Registry{Tools: tools},
		DefaultTimeout: 30 * time.Millisecond,
	}
	state, err := eng.Run(context.Background(), FlowDefinition{
		Nodes: []Node{{ID: "slow", Type: "tool", Tool: "slow", TimeoutMs: 5000}},
	}, RunOptions{})
	if err == nil {
		t.Fatal("expected timeout failure")
	}
	if state.Status != StatusFailed && state.Status != StatusCanceled {
		t.Fatalf("status=%s", state.Status)
	}
}

func TestHealthCheck(t *testing.T) {
	dir := t.TempDir()
	bridge := testBridge(t)
	tools := MergeTools(BuiltinTools(), bridge.Tools())
	rep := CheckHealth(HealthOpts{
		FlowPath:   "../../configs/flows/earnings-ingest.flow.yaml",
		Tools:      tools,
		RunsPath:   filepath.Join(dir, "runs.json"),
		EventsPath: filepath.Join(dir, "events.json"),
	})
	if rep.Status != "ok" {
		t.Fatalf("health=%#v", rep)
	}
	bad := CheckHealth(HealthOpts{FlowPath: "../../configs/flows/earnings-ingest.flow.yaml", Tools: BuiltinTools()})
	if bad.Status != "fail" {
		t.Fatalf("expected missing pipeline_ingest to fail, got %#v", bad)
	}
}

type captureNotify struct{ n int }

func (c *captureNotify) Send(context.Context, notify.Message) error {
	c.n++
	return nil
}

func testBridge(t *testing.T) *PipelineBridge {
	t.Helper()
	note := &captureNotify{}
	bridge, err := NewLocalPipeline(LocalPipelineOpts{
		CompaniesFile: "../../configs/companies.yaml",
		MacroFile:     "../../configs/macro.yaml",
		JobStorePath:  filepath.Join(t.TempDir(), "jobs.json"),
		Notifier:      note,
	})
	if err != nil {
		t.Fatal(err)
	}
	return bridge
}

func TestEarningsIngestDryRunViaPipeline(t *testing.T) {
	def, err := LoadFile("../../configs/flows/earnings-ingest.flow.yaml")
	if err != nil {
		t.Fatal(err)
	}
	bridge := testBridge(t)
	eng := &Engine{Registry: Registry{
		Tools:   MergeTools(BuiltinTools(), bridge.Tools()),
		Chatter: MockChatter{Content: "ok"},
	}}
	state, err := eng.Run(context.Background(), def, RunOptions{Inputs: map[string]any{
		"ticker":    "AAPL",
		"dry_run":   true,
		"body_path": "../../testdata/aapl_2026q4.csv",
		"force":     true,
	}})
	if err != nil {
		t.Fatal(err)
	}
	if state.Nodes["ingest"].Status != StatusSucceeded {
		t.Fatalf("ingest=%s err=%s", state.Nodes["ingest"].Status, state.Nodes["ingest"].Error)
	}
	out, _ := state.Nodes["ingest"].Output.(map[string]any)
	if fmt.Sprint(out["dry_run"]) != "true" {
		t.Fatalf("expected dry_run job, got %#v", out)
	}
	if len(bridge.Memory.Writes) != 0 {
		t.Fatalf("dry_run must not write, got %d", len(bridge.Memory.Writes))
	}
	if state.Nodes["summarize"].Status != StatusSucceeded {
		t.Fatalf("summarize=%s", state.Nodes["summarize"].Status)
	}
}

func TestEarningsIngestLiveWritesMemory(t *testing.T) {
	def, err := LoadFile("../../configs/flows/earnings-ingest.flow.yaml")
	if err != nil {
		t.Fatal(err)
	}
	bridge := testBridge(t)
	eng := &Engine{Registry: Registry{
		Tools:   MergeTools(BuiltinTools(), bridge.Tools()),
		Chatter: MockChatter{Content: "已写入"},
	}}
	state, err := eng.Run(context.Background(), def, RunOptions{Inputs: map[string]any{
		"ticker":    "AAPL",
		"dry_run":   false,
		"body_path": "../../testdata/aapl_2026q4.csv",
		"force":     true,
	}})
	if err != nil {
		t.Fatal(err)
	}
	if state.Status != StatusSucceeded {
		t.Fatalf("status=%s err=%s", state.Status, state.Error)
	}
	out, _ := state.Nodes["ingest"].Output.(map[string]any)
	if fmt.Sprint(out["status"]) != "succeeded" {
		t.Fatalf("ingest output %#v", out)
	}
	if fmt.Sprint(out["cells"]) == "0" || out["cells"] == nil {
		t.Fatalf("expected cells>0, got %#v", out["cells"])
	}
	if len(bridge.Memory.Writes) == 0 {
		t.Fatal("expected Memory sheet writes")
	}
	evidence := filepath.Join(t.TempDir(), "writes.json")
	if err := bridge.DumpWrites(evidence); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(evidence)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"writes"`) || !strings.Contains(string(raw), `"cells"`) {
		t.Fatalf("evidence missing content: %s", raw)
	}
	period, _ := out["period"].(map[string]any)
	if fmt.Sprint(period["label"]) != "2026Q4" {
		t.Fatalf("period %#v", period)
	}
}
