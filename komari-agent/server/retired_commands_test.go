package server

import "testing"

func TestRetiredEventsAreRejected(t *testing.T) {
	for _, method := range []string{"agent.exec", "agent.terminal.request", "agent.file", "agent.message", "agent.event"} {
		if processV2Event(nil, method, nil, "", nil) { t.Fatalf("retired event accepted: %s", method) }
	}
}
