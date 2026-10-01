package client

import (
	v2 "github.com/komari-monitor/komari/protocol/v2"
	"testing"
)

func TestRetiredCommandMethodsAreNotAccepted(t *testing.T) {
	for _, method := range []string{"agent.taskResult", "agent.exec", "agent.terminal.request"} {
		response := handleV2RPC("isolated-agent", v2.Request{JSONRPC: v2.Version, ID: 1, Method: method, Params: map[string]any{"task_id": "old", "result": "ignored", "exit_code": 0}}, false)
		if response.Error == nil || response.Error.Code != -32601 {
			t.Fatalf("retired method %s accepted: %#v", method, response)
		}
	}
}
