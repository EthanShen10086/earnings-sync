package orchestrator

// GraphIndex is a read-only index of node dependencies.
type GraphIndex struct {
	nodesByID  map[string]Node
	needsByID  map[string][]string
	dependents map[string][]string
	order      []string
}

// NewGraphIndex builds dependency maps, inferring needs from node input refs when omitted.
func NewGraphIndex(def FlowDefinition) *GraphIndex {
	g := &GraphIndex{
		nodesByID:  make(map[string]Node, len(def.Nodes)),
		needsByID:  make(map[string][]string, len(def.Nodes)),
		dependents: make(map[string][]string, len(def.Nodes)),
		order:      make([]string, 0, len(def.Nodes)),
	}
	for _, n := range def.Nodes {
		g.nodesByID[n.ID] = n
		g.order = append(g.order, n.ID)
	}
	for _, n := range def.Nodes {
		needs := n.Needs
		if needs == nil {
			needs = CollectNodeRefs(n.Input)
			if n.When != nil && n.When.From == "node" && n.When.NodeID != "" {
				needs = appendUnique(needs, n.When.NodeID)
			}
		}
		g.needsByID[n.ID] = needs
		for _, dep := range needs {
			g.dependents[dep] = appendUnique(g.dependents[dep], n.ID)
		}
	}
	return g
}

func (g *GraphIndex) Needs(nodeID string) []string {
	return g.needsByID[nodeID]
}

func (g *GraphIndex) IsReady(node Node, state func(string) *NodeState) bool {
	st := state(node.ID)
	if st == nil || (st.Status != StatusPending && st.Status != StatusReady) {
		return false
	}
	deps := g.needsByID[node.ID]
	if len(deps) == 0 {
		return true
	}
	mode := node.NeedsMode
	if mode == "" {
		mode = "all"
	}
	if mode == "any" {
		for _, dep := range deps {
			if s := state(dep); s != nil && s.Status == StatusSucceeded {
				return true
			}
		}
		return false
	}
	for _, dep := range deps {
		s := state(dep)
		if s == nil || s.Status != StatusSucceeded {
			return false
		}
	}
	return true
}

// ShouldSkip reports whether dependency failure / when-clause makes the node skippable.
func (g *GraphIndex) ShouldSkip(node Node, state func(string) *NodeState) bool {
	deps := g.needsByID[node.ID]
	mode := node.NeedsMode
	if mode == "" {
		mode = "all"
	}
	if mode == "any" {
		anyOK := false
		allTerminal := len(deps) > 0
		for _, dep := range deps {
			s := state(dep)
			if s == nil || !isTerminal(s.Status) {
				allTerminal = false
				continue
			}
			if s.Status == StatusSucceeded {
				anyOK = true
			}
		}
		return allTerminal && !anyOK
	}
	for _, dep := range deps {
		s := state(dep)
		if s == nil {
			continue
		}
		if s.Status == StatusFailed || s.Status == StatusSkipped || s.Status == StatusCanceled {
			// Upstream failed with continue: dependents that require all deps skip.
			return true
		}
	}
	return false
}

func (g *GraphIndex) Descendants(nodeID string) []string {
	seen := map[string]struct{}{}
	pending := append([]string{}, g.dependents[nodeID]...)
	for len(pending) > 0 {
		next := pending[0]
		pending = pending[1:]
		if _, ok := seen[next]; ok {
			continue
		}
		seen[next] = struct{}{}
		pending = append(pending, g.dependents[next]...)
	}
	out := make([]string, 0, len(seen))
	for id := range seen {
		out = append(out, id)
	}
	return out
}

func isTerminal(s Status) bool {
	switch s {
	case StatusSucceeded, StatusFailed, StatusSkipped, StatusCanceled:
		return true
	default:
		return false
	}
}

func appendUnique(list []string, item string) []string {
	for _, existing := range list {
		if existing == item {
			return list
		}
	}
	return append(list, item)
}
