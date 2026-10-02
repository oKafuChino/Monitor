package server

import (
	"encoding/json"
	"fmt"
	"strings"

	v2 "github.com/komari-monitor/komari-agent/protocol/v2"
)

type httpStatusError struct {
	StatusCode int
	Status     string
	Body       string
}

func (e *httpStatusError) Error() string {
	if e == nil {
		return ""
	}
	if e.Body != "" {
		return fmt.Sprintf("status code: %d,%s", e.StatusCode, redactCredentials(e.Body))
	}
	if e.Status != "" {
		return e.Status
	}
	return fmt.Sprintf("status code: %d", e.StatusCode)
}

func redactCredentials(value string) string {
 for _, secret := range []string{flags.Token,flags.AutoDiscoveryKey} { if secret!="" { value = strings.ReplaceAll(value,secret,"[redacted]") } }
 return value
}

func parseV2Response(body []byte) (*v2.Response, error) {
	var rpcResp v2.Response
	if err := json.Unmarshal(body, &rpcResp); err != nil {
		return nil, fmt.Errorf("invalid v2 JSON-RPC response: %w, body: %s", err, bodySnippet(body))
	}
	if rpcResp.JSONRPC != v2.Version {
		return nil, fmt.Errorf("invalid v2 JSON-RPC version %q, body: %s", rpcResp.JSONRPC, bodySnippet(body))
	}
	if rpcResp.Error != nil {
		return &rpcResp, fmt.Errorf("v2 rpc error %d: %s", rpcResp.Error.Code, redactCredentials(rpcResp.Error.Message))
	}
	return &rpcResp, nil
}

func bodySnippet(body []byte) string {
	body = []byte(redactCredentials(string(body)))
	const max = 120
	if len(body) > max {
		body = body[:max]
	}
	return fmt.Sprintf("%q", string(body))
}
