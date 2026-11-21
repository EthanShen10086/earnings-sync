package orchestrator

import (
	"context"

	"github.com/google/uuid"
)

type ctxKey string

const (
	ctxRequestID ctxKey = "orchestrator_request_id"
	ctxTraceID   ctxKey = "orchestrator_trace_id"
	ctxRunID     ctxKey = "orchestrator_run_id"
)

// WithIDs attaches request/trace/run identifiers to ctx for log correlation.
func WithIDs(ctx context.Context, requestID, traceID, runID string) context.Context {
	if requestID != "" {
		ctx = context.WithValue(ctx, ctxRequestID, requestID)
	}
	if traceID != "" {
		ctx = context.WithValue(ctx, ctxTraceID, traceID)
	}
	if runID != "" {
		ctx = context.WithValue(ctx, ctxRunID, runID)
	}
	return ctx
}

// RequestIDFrom returns the request id from ctx, or empty.
func RequestIDFrom(ctx context.Context) string {
	v, _ := ctx.Value(ctxRequestID).(string)
	return v
}

// TraceIDFrom returns the trace id from ctx, or empty.
func TraceIDFrom(ctx context.Context) string {
	v, _ := ctx.Value(ctxTraceID).(string)
	return v
}

// RunIDFrom returns the run id from ctx, or empty.
func RunIDFrom(ctx context.Context) string {
	v, _ := ctx.Value(ctxRunID).(string)
	return v
}

// EnsureIDs fills blank request/trace ids with UUIDs.
func EnsureIDs(requestID, traceID string) (string, string) {
	if requestID == "" {
		requestID = uuid.NewString()
	}
	if traceID == "" {
		traceID = uuid.NewString()
	}
	return requestID, traceID
}
