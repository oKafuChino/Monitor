package security

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOriginMatchesRequestBehindProxy(t *testing.T) {
	t.Setenv("KOMARI_TRUSTED_PROXIES", "")
	for _, tc := range []struct {
		name, target, origin, fetchSite string
		want bool
	}{
		{"TLS termination", "http://panel.example/api/login", "https://panel.example", "same-origin", true},
		{"rewritten upstream host", "http://127.0.0.1:25774/api/login", "https://panel.example", "same-origin", true},
		{"public nonstandard port", "http://monitor:25774/api/login", "https://panel.example:8443", "same-origin", true},
		{"cross site", "http://panel.example/api/login", "https://evil.example", "cross-site", false},
		{"sibling subdomain", "http://panel.example/api/login", "https://other.example", "same-site", false},
		{"different port", "http://panel.example/api/login", "http://panel.example:8080", "same-site", false},
		{"opaque origin", "http://panel.example/api/login", "null", "same-origin", false},
		{"missing origin", "http://panel.example/api/login", "", "same-origin", false},
		{"invalid origin", "http://panel.example/api/login", "https://panel.example/path", "same-origin", false},
		{"missing metadata does not trust forwarded proto", "http://panel.example/api/login", "https://panel.example", "", false},
		{"direct HTTP", "http://panel.example/api/login", "http://panel.example", "", true},
		{"direct HTTPS", "https://panel.example/api/login", "https://panel.example:443", "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, tc.target, nil)
			r.Header.Set("Origin", tc.origin)
			r.Header.Set("Sec-Fetch-Site", tc.fetchSite)
			r.Header.Set("X-Forwarded-Proto", "https")
			if got := OriginMatchesRequest(tc.origin, r); got != tc.want {
				t.Fatalf("OriginMatchesRequest = %v, want %v", got, tc.want)
			}
		})
	}
}
