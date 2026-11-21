package notify

import (
	"context"
	"sync"

	"github.com/EthanShen10086/earnings-sync/internal/model"
)

// Memory records outbound messages when FEATURE_NOTIFY=on without a real webhook.
// The Send path executes fully so pipeline notify steps stay testable.
type Memory struct {
	mu       sync.Mutex
	Messages []Message
}

func (m *Memory) Send(_ context.Context, msg Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := msg
	cp.Takeaway = append([]string(nil), msg.Takeaway...)
	cp.Warnings = append([]string(nil), msg.Warnings...)
	cp.Charts = append([]model.ChartSpec(nil), msg.Charts...)
	m.Messages = append(m.Messages, cp)
	return nil
}

// Len returns how many messages were accepted.
func (m *Memory) Len() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.Messages)
}

// Last returns the most recent message, if any.
func (m *Memory) Last() (Message, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.Messages) == 0 {
		return Message{}, false
	}
	return m.Messages[len(m.Messages)-1], true
}
