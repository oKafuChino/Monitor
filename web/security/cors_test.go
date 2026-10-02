package security

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/komari-monitor/komari/internal/config"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestCorsMiddlewareValidatesAPIOrigins(t *testing.T) {
	setupCORSConfigDB(t, "")
	router := setupCORSRouter(true, "https://allowed.example")

	tests := []struct {
		name            string
		origin          string
		wantStatus      int
		wantAllowOrigin string
	}{
		{
			name:       "allows API requests without Origin",
			wantStatus: http.StatusOK,
		},
		{
			name:            "allows same origin",
			origin:          "http://api.example",
			wantStatus:      http.StatusOK,
			wantAllowOrigin: "http://api.example",
		},
		{
			name:            "allows configured Origin",
			origin:          "https://allowed.example",
			wantStatus:      http.StatusOK,
			wantAllowOrigin: "https://allowed.example",
		},
		{
			name:       "rejects unlisted Origin",
			origin:     "https://evil.example",
			wantStatus: http.StatusForbidden,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response := performCORSRequest(router, http.MethodGet, "/api/ping", "api.example", tt.origin)

			if response.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", response.Code, tt.wantStatus)
			}
			if got := response.Header().Get("Access-Control-Allow-Origin"); got != tt.wantAllowOrigin {
				t.Fatalf("Access-Control-Allow-Origin = %q, want %q", got, tt.wantAllowOrigin)
			}
		})
	}
}

func TestCorsMiddlewareAllowsAPIKeyRequestsFromAnyOrigin(t *testing.T) {
	setupCORSConfigDB(t, "123456789012")
	router := setupCORSRouter(true, "")

	request := httptest.NewRequest(http.MethodGet, "/api/ping", nil)
	request.Host = "api.example"
	request.Header.Set("Origin", "https://evil.example")
	request.Header.Set("Authorization", "Bearer 123456789012")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	if got := response.Header().Get("Access-Control-Allow-Origin"); got != "https://evil.example" {
		t.Fatalf("Access-Control-Allow-Origin = %q, want %q", got, "https://evil.example")
	}
}

func TestProxiedLoginPassesBothOriginGuards(t *testing.T) {
	t.Setenv("KOMARI_TRUSTED_PROXIES", "")
	setupCORSConfigDB(t, "")
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(NewCorsController(true, "").Middleware(), CookieMutationProtection())
	router.POST("/api/login", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "success"})
	})
	for _, tc := range []struct {
		name, origin, fetchSite string
		want int
	}{
		{"same origin through proxy", "https://panel.example", "same-origin", http.StatusOK},
		{"cross site with matching referer", "https://evil.example", "cross-site", http.StatusForbidden},
		{"same site is not same origin", "https://other.example", "same-site", http.StatusForbidden},
		{"referer alone cannot establish target origin", "https://evil.example", "", http.StatusForbidden},
	} {
		t.Run(tc.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "http://monitor:25774/api/login", nil)
			request.Header.Set("Origin", tc.origin)
			request.Header.Set("Referer", tc.origin+"/login")
			request.Header.Set("Sec-Fetch-Site", tc.fetchSite)
			// A stale cookie must not make a legitimate new login fail CSRF.
			request.AddCookie(&http.Cookie{Name: "session_token", Value: "expired-session"})
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != tc.want {
				t.Fatalf("status = %d, want %d", response.Code, tc.want)
			}
		})
	}
}

func TestCorsMiddlewareHandlesAPIPreflight(t *testing.T) {
	setupCORSConfigDB(t, "")
	router := setupCORSRouter(true, "https://allowed.example")

	allowed := performCORSRequest(router, http.MethodOptions, "/api/ping", "api.example", "https://allowed.example")
	if allowed.Code != http.StatusNoContent {
		t.Fatalf("allowed preflight status = %d, want %d", allowed.Code, http.StatusNoContent)
	}
	if got := allowed.Header().Get("Access-Control-Allow-Origin"); got != "https://allowed.example" {
		t.Fatalf("allowed preflight Access-Control-Allow-Origin = %q", got)
	}

	rejected := performCORSRequest(router, http.MethodOptions, "/api/ping", "api.example", "https://evil.example")
	if rejected.Code != http.StatusForbidden {
		t.Fatalf("rejected preflight status = %d, want %d", rejected.Code, http.StatusForbidden)
	}
}

func TestCorsMiddlewareAllowsAuthorizationPreflightFromAnyOrigin(t *testing.T) {
	setupCORSConfigDB(t, "")
	router := setupCORSRouter(true, "")

	request := httptest.NewRequest(http.MethodOptions, "/api/ping", nil)
	request.Host = "api.example"
	request.Header.Set("Origin", "https://evil.example")
	request.Header.Set("Access-Control-Request-Headers", "authorization")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusNoContent)
	}
	if got := response.Header().Get("Access-Control-Allow-Origin"); got != "https://evil.example" {
		t.Fatalf("Access-Control-Allow-Origin = %q, want %q", got, "https://evil.example")
	}
	if got := response.Header().Get("Access-Control-Allow-Credentials"); got != "" {
		t.Fatalf("Access-Control-Allow-Credentials = %q, want empty", got)
	}
}

func TestCorsMiddlewareDisabledSkipsAPIValidation(t *testing.T) {
	setupCORSConfigDB(t, "")
	router := setupCORSRouter(false, "")

	response := performCORSRequest(router, http.MethodGet, "/api/ping", "api.example", "https://evil.example")
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	if got := response.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("Access-Control-Allow-Origin = %q, want empty", got)
	}
}

func TestCorsMiddlewareSkipsNonAPIPaths(t *testing.T) {
	setupCORSConfigDB(t, "")
	router := setupCORSRouter(true, "")

	response := performCORSRequest(router, http.MethodGet, "/public", "api.example", "https://evil.example")
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	if got := response.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("Access-Control-Allow-Origin = %q, want empty", got)
	}
}

func setupCORSRouter(enabled bool, allowlist string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(NewCorsController(enabled, allowlist).Middleware())
	router.GET("/api/ping", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})
	router.GET("/public", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})
	return router
}

func setupCORSConfigDB(t *testing.T, apiKey string) {
	t.Helper()

	name := strings.NewReplacer("/", "_", " ", "_").Replace(t.Name())
	db, err := gorm.Open(sqlite.Open("file:"+name+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite test db: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get sqlite test db handle: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() {
		_ = sqlDB.Close()
	})

	config.SetDb(db)
	if apiKey != "" {
		if err := config.Set(config.ApiKeyKey, apiKey); err != nil {
			t.Fatalf("set api key: %v", err)
		}
	}
}

func performCORSRequest(handler http.Handler, method, path, host, origin string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, nil)
	request.Host = host
	if origin != "" {
		request.Header.Set("Origin", origin)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}
