package agent

import (
	v2 "github.com/komari-monitor/komari/protocol/v2"
	"testing"
)

func TestOutboundEventsRejectRemoteShell(t *testing.T) {
	const uuid = "isolated-event-boundary"
	MarkV2Client(uuid)
	for _, method := range []string{"agent.exec", "agent.terminal.request", "unknown.event"} {
		if DispatchV2Event(uuid, method, map[string]any{"command": "echo test"}) {
			t.Fatalf("dispatched %s", method)
		}
		if event := EnqueueV2Event(uuid, method, nil); event.ID != "" {
			t.Fatalf("queued %s", method)
		}
	}
	if events := TakeV2Events(uuid, nil, 100); len(events) != 0 {
		t.Fatalf("unexpected events: %#v", events)
	}
	for _, method := range []string{v2.MethodAgentFile, v2.MethodAgentPing, v2.MethodAgentStartupConfig, v2.MethodAgentSwitchVersion} {
		if !DispatchV2Event(uuid, method, nil) {
			t.Fatalf("retained event rejected: %s", method)
		}
	}
	if events := TakeV2Events(uuid, nil, 100); len(events) != 4 {
		t.Fatalf("retained events lost: %#v", events)
	}
}
