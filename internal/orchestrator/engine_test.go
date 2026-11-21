package orchestrator

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
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

func TestLoadDemoFile(t *testing.T) {
	def, err := LoadFile("../../configs/flows/earnings-ingest.flow.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if def.Title == "" || len(def.Nodes) < 3 {
		t.Fatalf("unexpected def: %+v", def)
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

func TestEarningsIngestDryRun(t *testing.T) {
	def, err := LoadFile("../../configs/flows/earnings-ingest.flow.yaml")
	if err != nil {
		t.Fatal(err)
	}
	eng := &Engine{Registry: Registry{Tools: BuiltinTools(), Chatter: MockChatter{Content: "ok"}}}
	state, err := eng.Run(context.Background(), def, RunOptions{Inputs: map[string]any{
		"ticker":  "AAPL",
		"dry_run": true,
	}})
	if err != nil {
		t.Fatal(err)
	}
	if state.Nodes["write"].Status != StatusSkipped {
		t.Fatalf("write=%s", state.Nodes["write"].Status)
	}
	if state.Nodes["notify"].Status != StatusSkipped {
		t.Fatalf("notify=%s", state.Nodes["notify"].Status)
	}
	if state.Nodes["summarize"].Status != StatusSucceeded {
		t.Fatalf("summarize=%s", state.Nodes["summarize"].Status)
	}
}
