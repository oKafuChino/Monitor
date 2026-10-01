// Package share owns a closed route table, independent of the main router.
package share

import (
 "net/http"
 "net/url"
 "time"
 "github.com/gin-gonic/gin"
 "github.com/komari-monitor/komari/internal/sharing"
)

func New(service *sharing.Service, origin *url.URL, proxies []string) (*gin.Engine,error) {
 assets,err:=loadAssets(); if err!=nil { return nil,err }
 r:=gin.New()
 if err:=r.SetTrustedProxies(proxies);err!=nil { return nil,err }
 // gin.Recovery dumps request headers/URI on panic, which would leak tokens.
 r.Use(func(c *gin.Context) { defer func(){if recover()!=nil {c.AbortWithStatus(500)}}();c.Next() })
 // No request logger: entry paths contain a capability. Proxy logging must
 // also be disabled, as documented in the deployment example.
 limits:=newLimiter()
 r.Use(func(c *gin.Context) {
  c.Header("Cache-Control","no-store")
  c.Header("Referrer-Policy","no-referrer")
  c.Header("X-Robots-Tag","noindex, nofollow")
  c.Header("X-Content-Type-Options","nosniff")
  c.Header("X-Frame-Options","DENY")
  // Recharts positions SVG tooltips with inline styles; scripts remain local.
  c.Header("Content-Security-Policy","default-src 'none'; script-src 'self'; style-src 'self' 'unsafe-inline'; connect-src 'self'; img-src 'self' data:; font-src 'self'; frame-ancestors 'none'; object-src 'none'; base-uri 'none'; form-action 'none'")
  if c.Request.Host!=origin.Host { c.AbortWithStatus(404);return }
  if o:=c.GetHeader("Origin"); o!="" && o!=origin.String() { c.AbortWithStatus(403);return }
  if c.GetHeader("Sec-Fetch-Site")=="cross-site" && c.Request.Method!=http.MethodGet { c.AbortWithStatus(403);return }
  if c.Request.URL.RawQuery!="" { c.AbortWithStatus(400);return }
  category:="static"; budget:=180
  if c.Request.URL.Path=="/api/share/rpc" { category="data";budget=120 }
  if len(c.Request.URL.Path)>3 && c.Request.URL.Path[:3]=="/s/" { category="exchange";budget=20 }
  if !limits.allow("global",6000) || !limits.allow(category+":"+c.ClientIP(),budget) { c.AbortWithStatus(429);return }
  c.Next()
 })
 secure:=origin.Scheme=="https"
 cookieName:="monitor_share_dev";if secure { cookieName="__Host-monitor_share" }
 r.GET("/healthz",func(c *gin.Context){c.Status(204)})
 r.GET("/s/:token",func(c *gin.Context) {
  if !limits.allow("token:"+sharing.Digest(c.Param("token")),30) { c.AbortWithStatus(429);return }
  credential,expiry,err:=service.Exchange(c.Param("token"));if err!=nil { c.AbortWithStatus(404);return }
  http.SetCookie(c.Writer,&http.Cookie{Name:cookieName,Value:credential,Path:"/",Secure:secure,HttpOnly:true,SameSite:http.SameSiteLaxMode,MaxAge:int(time.Until(expiry).Seconds()),Expires:expiry})
  c.Redirect(302,"/node")
 })
 authenticate:=func(c *gin.Context) (*sharing.Context,bool) {
  credential,_:=c.Cookie(cookieName); share,err:=service.Authenticate(credential)
  if err!=nil { http.SetCookie(c.Writer,&http.Cookie{Name:cookieName,Path:"/",MaxAge:-1,Secure:secure,HttpOnly:true,SameSite:http.SameSiteLaxMode});c.AbortWithStatus(404);return nil,false }
  if !limits.allow("link:"+share.Link.ID,300) { c.AbortWithStatus(429);return nil,false };return share,true
 }
 r.GET("/node",func(c *gin.Context){if _,ok:=authenticate(c);ok {c.Data(200,"text/html; charset=utf-8",assets["index.html"])}})
 r.GET("/assets/:name",func(c *gin.Context){serveAsset(c,assets)})
 r.POST("/api/share/rpc",func(c *gin.Context){share,ok:=authenticate(c);if !ok{return};serveRPC(c,share,service,limits)})
 r.NoRoute(func(c *gin.Context){c.Status(404)})
 return r,nil
}
