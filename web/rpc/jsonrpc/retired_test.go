package jsonrpc

import (
	"context"
	"github.com/komari-monitor/komari/pkg/rpc"
	"testing"
)

func TestRemovedExtensionAndCommandRPCsHaveNoHandler(t *testing.T) {
	for _, method := range []string{"exec", "getTasks", "getTaskById", "getTasksByClientId", "getSpecificTaskResult", "getTaskResultsByTaskId", "listPlugins", "setPluginEnabled", "getPluginLogs", "deletePlugin", "getPluginConfiguration", "setPluginConfiguration"} {
		result := Dispatch(context.Background(), &rpc.ContextMeta{Principal: rpc.NewAPIKeyPrincipal()}, &rpc.JsonRpcRequest{Version: rpc.RPC_VERSION, ID: 1, Method: "admin:" + method})
		if result.Error == nil || result.Error.Code != rpc.MethodNotFound {
			t.Fatalf("removed RPC %s: %#v", method, result)
		}
	}
}
