package security

import (
 "net/http"
 "net/url"
 "github.com/gin-gonic/gin"
 "github.com/komari-monitor/komari/internal/config"
)

// CookieMutationProtection is independent of CORS display preferences.
// Non-browser automation should use an explicit bearer credential.
func CookieMutationProtection() gin.HandlerFunc {
 return func(c *gin.Context) {
  method := c.Request.Method
  if !isAPIRequestPath(c.Request.URL.Path) || method==http.MethodGet || method==http.MethodHead || method==http.MethodOptions { c.Next(); return }
  session, _ := c.Cookie("session_token")
  if session=="" || IsAPIKeyRequest(c.Request) { c.Next(); return }
  origin := c.GetHeader("Origin")
  if origin=="" { if referrer,err:=url.Parse(c.GetHeader("Referer")); err==nil && referrer.Host!="" { origin=referrer.Scheme+"://"+referrer.Host } }
  allowlist,_:=config.GetAs[string](config.CorsAllowedOriginsKey,"")
  if origin=="" || (!OriginMatchesRequest(origin,c.Request) && !OriginInAllowlist(origin,allowlist)) { c.AbortWithStatusJSON(http.StatusForbidden,gin.H{"status":"error","message":"Cookie mutation requires a trusted browser origin"}); return }
  c.Next()
 }
}
