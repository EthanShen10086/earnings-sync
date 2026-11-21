package orchestrator

// RunRegistry is the durable/in-memory registry of flow runs.
// Memory (*RunStore), FileRunStore (via Memory), and PGRunStore all satisfy this.
type RunRegistry interface {
	Put(state *RunState)
	Get(runID string) (*RunState, bool)
	ByClientRequestID(clientID string) (*RunState, bool)
	List(limit int) []*RunState
}

// Ensure *RunStore implements RunRegistry.
var _ RunRegistry = (*RunStore)(nil)
