package orchestrator

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Engine runs FlowDefinitions locally (plugin local_mode equivalent).
// Scheduling is a DAG ready-queue pump (fit-ai-plugin SimpleFlowEngine),
// not the linear nextStep advance used by ftp3_runtime_hub.
type Engine struct {
	Registry Registry
	Events   chan Event // optional; if nil, events are discarded
	// Runs remembers finished/in-flight runs by RunID and ClientRequestID (hub-style idempotency).
	Runs *RunStore
}

// RunOptions configures one local execution.
type RunOptions struct {
	RunID           string
	ClientRequestID string // optional; same id returns the previous RunState (hub StartFlowRun idiom)
	Inputs          map[string]any
}

// Run submits inputs and drives the DAG to a terminal status.
func (e *Engine) Run(ctx context.Context, def FlowDefinition, opts RunOptions) (*RunState, error) {
	if err := Validate(def); err != nil {
		return nil, err
	}
	if opts.Inputs == nil {
		opts.Inputs = map[string]any{}
	}
	for name, spec := range def.Inputs {
		if spec.Required {
			if _, ok := opts.Inputs[name]; !ok {
				return nil, fmt.Errorf("缺少必填 Flow 输入：%s", name)
			}
		}
	}

	if e.Runs == nil {
		e.Runs = NewRunStore()
	}
	if opts.ClientRequestID != "" {
		if prev, ok := e.Runs.ByClientRequestID(opts.ClientRequestID); ok {
			cp := *prev
			return &cp, nil
		}
	}

	runID := opts.RunID
	if runID == "" {
		runID = uuid.NewString()
	}
	concurrency := def.Concurrency
	if concurrency <= 0 {
		concurrency = 4
	}
	graph := NewGraphIndex(def)
	state := &RunState{
		RunID:           runID,
		ClientRequestID: opts.ClientRequestID,
		Title:           def.Title,
		Status:          StatusRunning,
		Inputs:          opts.Inputs,
		Nodes:           make(map[string]*NodeState, len(def.Nodes)),
		StartedAt:       time.Now(),
	}
	for _, n := range def.Nodes {
		state.Nodes[n.ID] = &NodeState{
			ID:     n.ID,
			Type:   n.Type,
			Title:  n.Title,
			Status: StatusPending,
		}
	}
	e.Runs.Put(state)
	e.emit(Event{Seq: bump(state), RunID: runID, Kind: "run_start", Status: StatusRunning})

	var mu sync.Mutex
	active := 0
	failedStop := false

	waitCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	type doneMsg struct {
		nodeID  string
		attempt Attempt
		err     error
	}
	doneCh := make(chan doneMsg, len(def.Nodes)*4)

	// launch assumes mu is held.
	launch := func(node Node) {
		ns := state.Nodes[node.ID]
		ns.Status = StatusRunning
		attemptNo := len(ns.Attempts) + 1
		att := Attempt{No: attemptNo, Status: StatusRunning, StartedAt: time.Now()}
		ns.Attempts = append(ns.Attempts, att)
		active++
		e.emit(Event{Seq: bump(state), RunID: runID, Kind: "node_start", NodeID: node.ID, Status: StatusRunning})
		go func(n Node, attemptIdx int) {
			res, err := e.executeNode(waitCtx, state, n, &mu)
			msg := doneMsg{nodeID: n.ID, attempt: Attempt{
				No:        attemptIdx,
				StartedAt: att.StartedAt,
				EndedAt:   time.Now(),
			}, err: err}
			if err != nil {
				msg.attempt.Status = StatusFailed
				msg.attempt.Error = err.Error()
			} else {
				msg.attempt.Status = StatusSucceeded
				msg.attempt.Output = res.Output
			}
			doneCh <- msg
		}(node, attemptNo)
	}

	for {
		if waitCtx.Err() != nil {
			failedStop = true
		}

		mu.Lock()
		e.markSkips(def, graph, state)
		ready := e.collectReady(def, graph, state)
		for len(ready) > 0 && active < concurrency && !failedStop {
			n := ready[0]
			ready = ready[1:]
			launch(n)
		}
		allTerminal := nodesAllTerminal(state)
		waiting := active
		mu.Unlock()

		if (allTerminal && waiting == 0) || (failedStop && waiting == 0) {
			break
		}
		if waiting == 0 && !allTerminal {
			mu.Lock()
			for _, ns := range state.Nodes {
				if ns.Status == StatusPending || ns.Status == StatusReady {
					ns.Status = StatusFailed
					ns.Error = "依赖未满足，无法调度"
					failedStop = true
				}
			}
			mu.Unlock()
			break
		}

		select {
		case <-waitCtx.Done():
			failedStop = true
		case msg := <-doneCh:
			mu.Lock()
			active--
			ns := state.Nodes[msg.nodeID]
			if len(ns.Attempts) > 0 {
				ns.Attempts[len(ns.Attempts)-1] = msg.attempt
			}
			node := graph.nodesByID[msg.nodeID]
			if msg.err != nil {
				maxAttempts := 1
				backoff := 0
				if node.Retry != nil {
					maxAttempts = node.Retry.MaxAttempts
					backoff = node.Retry.BackoffMs
				}
				if len(ns.Attempts) < maxAttempts && waitCtx.Err() == nil {
					ns.Status = StatusPending
					mu.Unlock()
					if backoff > 0 {
						timer := time.NewTimer(time.Duration(backoff) * time.Millisecond)
						select {
						case <-waitCtx.Done():
							timer.Stop()
						case <-timer.C:
						}
					}
					continue
				}
				ns.Status = StatusFailed
				ns.Error = msg.err.Error()
				e.emit(Event{Seq: bump(state), RunID: runID, Kind: "node_end", NodeID: msg.nodeID, Status: StatusFailed, Error: ns.Error})
				onFailure := node.OnFailure
				if onFailure == "" {
					onFailure = "stop"
				}
				if onFailure == "stop" {
					failedStop = true
					cancel()
				}
			} else {
				ns.Status = StatusSucceeded
				ns.Output = msg.attempt.Output
				e.emit(Event{Seq: bump(state), RunID: runID, Kind: "node_end", NodeID: msg.nodeID, Status: StatusSucceeded, Output: ns.Output})
			}
			mu.Unlock()
		}
	}

	for {
		mu.Lock()
		left := active
		mu.Unlock()
		if left == 0 {
			break
		}
		msg := <-doneCh
		mu.Lock()
		active--
		ns := state.Nodes[msg.nodeID]
		if len(ns.Attempts) > 0 {
			ns.Attempts[len(ns.Attempts)-1] = msg.attempt
		}
		if msg.err != nil {
			if ns.Status == StatusRunning {
				ns.Status = StatusFailed
				ns.Error = msg.err.Error()
			}
		} else if ns.Status == StatusRunning {
			ns.Status = StatusSucceeded
			ns.Output = msg.attempt.Output
		}
		mu.Unlock()
	}

	mu.Lock()
	defer mu.Unlock()
	state.EndedAt = time.Now()
	if ctx.Err() != nil && !nodesAnyFailed(state) {
		state.Status = StatusCanceled
		state.Error = ctx.Err().Error()
	} else if nodesAnyFailed(state) {
		state.Status = StatusFailed
		for _, ns := range state.Nodes {
			if ns.Status == StatusFailed && state.Error == "" {
				state.Error = ns.Error
			}
		}
	} else {
		state.Status = StatusSucceeded
		if def.Output != nil {
			out, err := Resolve(def.Output, state.Inputs, state.Nodes)
			if err != nil {
				state.Status = StatusFailed
				state.Error = err.Error()
			} else {
				state.Output = out
			}
		} else {
			state.Output = defaultOutput(state)
		}
	}
	e.Runs.Put(state)
	e.emit(Event{Seq: bump(state), RunID: runID, Kind: "run_end", Status: state.Status, Error: state.Error, Output: state.Output})
	if state.Status == StatusFailed {
		return state, fmt.Errorf("flow 失败: %s", state.Error)
	}
	if state.Status == StatusCanceled {
		return state, fmt.Errorf("flow 已取消")
	}
	return state, nil
}

func (e *Engine) markSkips(def FlowDefinition, graph *GraphIndex, state *RunState) {
	for _, n := range def.Nodes {
		ns := state.Nodes[n.ID]
		if ns.Status != StatusPending && ns.Status != StatusReady {
			continue
		}
		if graph.ShouldSkip(n, func(id string) *NodeState { return state.Nodes[id] }) {
			ns.Status = StatusSkipped
			e.emit(Event{Seq: bump(state), RunID: state.RunID, Kind: "node_end", NodeID: n.ID, Status: StatusSkipped})
		}
	}
}

func (e *Engine) collectReady(def FlowDefinition, graph *GraphIndex, state *RunState) []Node {
	ready := make([]Node, 0)
	for _, n := range def.Nodes {
		ns := state.Nodes[n.ID]
		if ns.Status != StatusPending && ns.Status != StatusReady {
			continue
		}
		if !graph.IsReady(n, func(id string) *NodeState { return state.Nodes[id] }) {
			continue
		}
		if n.When != nil {
			ok, err := e.evalWhen(n.When, state)
			if err != nil || !ok {
				ns.Status = StatusSkipped
				if err != nil {
					ns.Error = err.Error()
				}
				e.emit(Event{Seq: bump(state), RunID: state.RunID, Kind: "node_end", NodeID: n.ID, Status: StatusSkipped, Error: ns.Error})
				continue
			}
		}
		ns.Status = StatusReady
		ready = append(ready, n)
	}
	return ready
}

func defaultOutput(state *RunState) map[string]any {
	out := make(map[string]any, len(state.Nodes))
	for id, ns := range state.Nodes {
		out[id] = ns.Output
	}
	return out
}

func nodesAllTerminal(state *RunState) bool {
	for _, ns := range state.Nodes {
		if !isTerminal(ns.Status) {
			return false
		}
	}
	return true
}

func nodesAnyFailed(state *RunState) bool {
	for _, ns := range state.Nodes {
		if ns.Status == StatusFailed {
			return true
		}
	}
	return false
}

func (e *Engine) executeNode(ctx context.Context, state *RunState, node Node, mu *sync.Mutex) (NodeResult, error) {
	mu.Lock()
	inputSpec := node.Input
	inputs := cloneMap(state.Inputs)
	nodesSnap := snapshotNodes(state.Nodes)
	mu.Unlock()

	resolved, err := Resolve(inputSpec, inputs, nodesSnap)
	if err != nil {
		return NodeResult{}, err
	}
	runner, err := e.Registry.runnerFor(node)
	if err != nil {
		return NodeResult{}, err
	}
	runCtx := ctx
	var cancel context.CancelFunc
	if node.TimeoutMs > 0 {
		runCtx, cancel = context.WithTimeout(ctx, time.Duration(node.TimeoutMs)*time.Millisecond)
		defer cancel()
	}
	return runner.Execute(runCtx, node, resolved)
}

func (e *Engine) evalWhen(w *WhenClause, state *RunState) (bool, error) {
	ref := map[string]any{"from": w.From, "key": w.Key, "nodeId": w.NodeID, "path": w.Path}
	val, err := resolveRef(w.From, ref, state.Inputs, state.Nodes)
	if err != nil {
		return false, err
	}
	return valuesEqual(val, w.Equals), nil
}

func (e *Engine) emit(ev Event) {
	if e.Events == nil {
		return
	}
	select {
	case e.Events <- ev:
	default:
	}
}

func bump(state *RunState) int {
	state.Seq++
	return state.Seq
}

func cloneMap(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func snapshotNodes(in map[string]*NodeState) map[string]*NodeState {
	out := make(map[string]*NodeState, len(in))
	for k, v := range in {
		cp := *v
		out[k] = &cp
	}
	return out
}
