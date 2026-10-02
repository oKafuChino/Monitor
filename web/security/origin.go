package security

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/komari-monitor/komari/internal/config"
	"github.com/komari-monitor/komari/utils"
)

func SplitAllowlist(raw string) []string {
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == '\n' || r == '\r'
	})
	entries := make([]string, 0, len(parts))
	for _, part := range parts {
		entry := strings.TrimSpace(part)
		if entry != "" {
			entries = append(entries, entry)
		}
	}
	return entries
}

func OriginMatchesHost(origin, host string) bool {
	_, originHost, ok := normalizeOrigin(origin)
	return ok && strings.EqualFold(originHost, host)
}

func OriginMatchesRequest(origin string, r *http.Request) bool {
 left, _, ok := normalizeOrigin(origin); if !ok { return false }
 right, _, ok := normalizeOrigin(utils.RequestScheme(r)+"://"+r.Host)
 return ok && left == right
}

func OriginInAllowlist(origin, rawAllowlist string) bool {
	normalizedOrigin, originHost, ok := normalizeOrigin(origin)
	if !ok {
		return false
	}
	for _, entry := range SplitAllowlist(rawAllowlist) {
		if entry == "*" {
			return true
		}
		if strings.Contains(entry, "://") {
			normalizedEntry, _, ok := normalizeOrigin(entry)
			if ok && strings.EqualFold(normalizedEntry, normalizedOrigin) {
				return true
			}
			continue
		}
		if strings.EqualFold(entry, originHost) {
			return true
		}
	}
	return false
}

func IsAPIKeyRequest(r *http.Request) bool {
	apiKeyConfig, err := config.GetAs[string](config.ApiKeyKey, "")
	if err != nil || apiKeyConfig == "" || len(apiKeyConfig) < 12 {
		return false
	}
	return r.Header.Get("Authorization") == "Bearer "+apiKeyConfig
}

func IsAuthorizationPreflight(r *http.Request) bool {
	if r.Method != http.MethodOptions {
		return false
	}
	for _, header := range strings.Split(r.Header.Get("Access-Control-Request-Headers"), ",") {
		if strings.EqualFold(strings.TrimSpace(header), "authorization") {
			return true
		}
	}
	return false
}

func normalizeOrigin(raw string) (string, string, bool) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Path != "" && parsed.Path != "/") {
		return "", "", false
	}
	host := strings.ToLower(parsed.Host)
	if (parsed.Scheme == "https" && parsed.Port() == "443") || (parsed.Scheme == "http" && parsed.Port() == "80") { host = strings.ToLower(parsed.Hostname()); if strings.Contains(host, ":") { host = "["+host+"]" } }
	return strings.ToLower(parsed.Scheme) + "://" + host, host, true
}
