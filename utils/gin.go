package utils

import (
 "github.com/gin-gonic/gin"
 "net"
 "net/http"
 "os"
 "strings"
)

func ConfigureTrustedProxies(r *gin.Engine) error {
 var proxies []string
 for _, proxy := range strings.Split(os.Getenv("KOMARI_TRUSTED_PROXIES"), ",") { if proxy = strings.TrimSpace(proxy); proxy != "" { proxies = append(proxies, proxy) } }
 return r.SetTrustedProxies(proxies)
}

func RequestScheme(r *http.Request) string {
 if r.TLS != nil { return "https" }
 host, _, err := net.SplitHostPort(r.RemoteAddr); if err != nil { host = r.RemoteAddr }
 peer := net.ParseIP(host)
 trusted := false
 for _, value := range strings.Split(os.Getenv("KOMARI_TRUSTED_PROXIES"), ",") {
  value = strings.TrimSpace(value)
  if ip := net.ParseIP(value); ip != nil && ip.Equal(peer) { trusted = true }
  if _, network, err := net.ParseCIDR(value); err == nil && network.Contains(peer) { trusted = true }
 }
 if trusted { scheme := strings.ToLower(strings.TrimSpace(r.Header.Get("X-Forwarded-Proto"))); if scheme == "https" || scheme == "http" { return scheme } }
 return "http"
}

// https://github.com/labstack/echo/blob/98ca08e7dd64075b858e758d6693bf9799340756/context.go#L275-L294
func GetScheme(c *gin.Context) string {
	return RequestScheme(c.Request)
}

func GetCallbackURL(c *gin.Context) string {
	scheme := GetScheme(c)
	host := c.Request.Host
	return scheme + "://" + host + "/api/oauth_callback"
}
