package jsonrpc

import (
 "testing"
 "github.com/komari-monitor/komari/internal/config"
)

func TestSharePublicBaseSettingsValidation(t *testing.T) {
 for _, value := range []any{nil, 123, "http://share.example.com", "https://share.example.com/path", "https://share.example.com?q=1", "https://user:pass@share.example.com", "https://share.example.com#fragment"} {
  if err := validateSharePublicBaseSetting(map[string]interface{}{config.SharePublicBaseURLKey:value}); err == nil { t.Fatalf("accepted unsafe URL: %#v",value) }
 }
 for _, value := range []string{"", "https://share.example.com", "http://localhost:25775", "http://127.0.0.1:25775"} {
  cfg := map[string]interface{}{config.SharePublicBaseURLKey:"  "+value+"  "}
  if err := validateSharePublicBaseSetting(cfg); err != nil { t.Fatal(err) }
  if cfg[config.SharePublicBaseURLKey] != value { t.Fatal("URL not normalized") }
 }
 if err := validateSharePublicBaseSetting(map[string]interface{}{"sitename":"Monitor"}); err != nil { t.Fatal(err) }
}
