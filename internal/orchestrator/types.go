// Package orchestrator is a configurable AI flow engine for earnings-sync.
//
// Design follows the fit-ai-plugin Flow Runtime (SimpleFlowEngine): a YAML DAG
// of typed nodes, dependency readiness (needs / needsMode), per-node retry,
// onFailure stop|continue, and pluggable runners for agent/tool steps.
// FTP/Workbench UI and outer plan persistence are intentionally omitted.
package orchestrator

import (
	"context"
	"time"
)

// Status is a flow or node lifecycle state.
type Status string

const (
	StatusPending   Status = "pending"
	StatusReady     Status = "ready"
	StatusRunning   Status = "running"
	StatusSucceeded Status = "succeeded"
	StatusFailed    Status = "failed"
	StatusSkipped   Status = "skipped"
	StatusCanceled  Status = "canceled"
)

// FlowDefinition is the authoring document (YAML/JSON).
type FlowDefinition struct {
	Title       string               `yaml:"title" json:"title,omitempty"`
	Agent       string               `yaml:"agent" json:"agent,omitempty"`
	Model       string               `yaml:"model" json:"model,omitempty"`
	Concurrency int                  `yaml:"concurrency" json:"concurrency,omitempty"`
	Inputs      map[string]InputSpec `yaml:"inputs" json:"inputs,omitempty"`
	Output      any                  `yaml:"output" json:"output,omitempty"`
	Nodes       []Node               `yaml:"nodes" json:"nodes"`
}

// InputSpec declares a flow-level input.
type InputSpec struct {
	Type        string `yaml:"type" json:"type,omitempty"`
	Title       string `yaml:"title" json:"title,omitempty"`
	Description string `yaml:"description" json:"description,omitempty"`
	Required    bool   `yaml:"required" json:"required,omitempty"`
}

// Node is one executable unit in the DAG.
type Node struct {
	ID          string       `yaml:"id" json:"id"`
	Type        string       `yaml:"type" json:"type"` // agent | tool | condition
	Title       string       `yaml:"title" json:"title,omitempty"`
	Agent       string       `yaml:"agent" json:"agent,omitempty"`
	Model       string       `yaml:"model" json:"model,omitempty"`
	Prompt      string       `yaml:"prompt" json:"prompt,omitempty"`
	Tool        string       `yaml:"tool" json:"tool,omitempty"`
	Input       any          `yaml:"input" json:"input,omitempty"`
	Needs       []string     `yaml:"needs" json:"needs,omitempty"`
	NeedsMode   string       `yaml:"needsMode" json:"needsMode,omitempty"` // all | any
	Retry       *RetryPolicy `yaml:"retry" json:"retry,omitempty"`
	TimeoutMs   int          `yaml:"timeoutMs" json:"timeoutMs,omitempty"`
	OnFailure   string       `yaml:"onFailure" json:"onFailure,omitempty"` // stop | continue
	When        *WhenClause  `yaml:"when" json:"when,omitempty"`
	Equals      any          `yaml:"equals" json:"equals,omitempty"` // for condition nodes: compare resolved input
	BranchTrue  string       `yaml:"branchTrue" json:"branchTrue,omitempty"`
	BranchFalse string       `yaml:"branchFalse" json:"branchFalse,omitempty"`
}

// RetryPolicy controls automatic re-attempts of a failing node.
type RetryPolicy struct {
	MaxAttempts int `yaml:"maxAttempts" json:"maxAttempts"`
	BackoffMs   int `yaml:"backoffMs" json:"backoffMs,omitempty"`
}

// WhenClause skips a node unless the referenced value equals Equals.
type WhenClause struct {
	From   string `yaml:"from" json:"from"` // node | input
	Key    string `yaml:"key" json:"key,omitempty"`
	NodeID string `yaml:"nodeId" json:"nodeId,omitempty"`
	Path   string `yaml:"path" json:"path,omitempty"`
	Equals any    `yaml:"equals" json:"equals"`
}

// Attempt records one execution try.
type Attempt struct {
	No        int       `json:"no"`
	Status    Status    `json:"status"`
	StartedAt time.Time `json:"started_at"`
	EndedAt   time.Time `json:"ended_at,omitempty"`
	Error     string    `json:"error,omitempty"`
	Output    any       `json:"output,omitempty"`
}

// NodeState is the runtime view of one node.
type NodeState struct {
	ID       string    `json:"id"`
	Type     string    `json:"type"`
	Title    string    `json:"title,omitempty"`
	Status   Status    `json:"status"`
	Attempts []Attempt `json:"attempts,omitempty"`
	Output   any       `json:"output,omitempty"`
	Error    string    `json:"error,omitempty"`
}

// RunState is the snapshot of one local flow run.
type RunState struct {
	RunID           string                `json:"run_id"`
	ClientRequestID string                `json:"client_request_id,omitempty"` // hub StartFlowRun 幂等键
	Title           string                `json:"title,omitempty"`
	Status          Status                `json:"status"`
	Seq             int                   `json:"seq"`
	Inputs          map[string]any        `json:"inputs,omitempty"`
	Nodes           map[string]*NodeState `json:"nodes"`
	Output          any                   `json:"output,omitempty"`
	Error           string                `json:"error,omitempty"`
	StartedAt       time.Time             `json:"started_at"`
	EndedAt         time.Time             `json:"ended_at,omitempty"`
}

// NodeResult is what a Runner returns.
type NodeResult struct {
	Output any
}

// Runner executes one node with already-resolved input.
type Runner interface {
	Execute(ctx context.Context, node Node, input any) (NodeResult, error)
}

// Event is emitted as the engine advances.
type Event struct {
	Seq     int
	RunID   string
	Kind    string // node_start | node_end | run_end | log
	NodeID  string
	Status  Status
	Error   string
	Output  any
	Message string
}
