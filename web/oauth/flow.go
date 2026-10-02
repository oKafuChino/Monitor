package oauth

import (
	"fmt"
	"net/http"
	"sync"
	"time"
	"github.com/gin-gonic/gin"
	"github.com/komari-monitor/komari/internal/config"
	"github.com/komari-monitor/komari/utils"
)

type Flow struct { Provider, Purpose, User, Session string; Expires time.Time; Generation uint64 }
var flowMu sync.Mutex
var flows = map[string]Flow{}

func BeginFlow(c *gin.Context, purpose, user, session string) (string, error) {
	enabled, err := config.GetAs[bool](config.OAuthEnabledKey, false)
	if err != nil || !enabled { return "", fmt.Errorf("OAuth is not enabled") }
	mu.Lock(); defer mu.Unlock()
	p := currentProvider
	if p == nil { return "", fmt.Errorf("OAuth provider unavailable") }
	url, state := p.GetAuthorizationURL(utils.GetCallbackURL(c))
	if url == "" || state == "" { return "", fmt.Errorf("OAuth provider cannot establish a secure authorization flow") }
	flowMu.Lock(); defer flowMu.Unlock()
	for key, flow := range flows { if !time.Now().Before(flow.Expires) { delete(flows, key) } }
	if len(flows) >= 1024 { return "", fmt.Errorf("too many authorization flows") }
	flows[state] = Flow{Provider:p.GetName(), Purpose:purpose, User:user, Session:session, Expires:time.Now().Add(5*time.Minute), Generation:providerGeneration}
	http.SetCookie(c.Writer, &http.Cookie{Name:"oauth_state", Value:state, MaxAge:300, Path:"/", HttpOnly:true, Secure:utils.GetScheme(c)=="https", SameSite:http.SameSiteLaxMode})
	return url, nil
}

func ConsumeFlow(c *gin.Context) (Flow, string, error) {
	state, _ := c.Cookie("oauth_state")
	http.SetCookie(c.Writer, &http.Cookie{Name:"oauth_state", MaxAge:-1, Path:"/", HttpOnly:true, Secure:utils.GetScheme(c)=="https", SameSite:http.SameSiteLaxMode})
	if state == "" || state != c.Query("state") { return Flow{}, "", fmt.Errorf("invalid OAuth state") }
	flowMu.Lock(); flow, ok := flows[state]; delete(flows,state); flowMu.Unlock()
	enabled, err := config.GetAs[bool](config.OAuthEnabledKey, false)
	configured, configErr := config.GetAs[string](config.OAuthProviderKey, "github")
	mu.Lock(); provider:=currentProvider; generation:=providerGeneration; providerName:=""; if provider!=nil { providerName=provider.GetName() }; mu.Unlock()
	if !ok || !time.Now().Before(flow.Expires) || err != nil || configErr != nil || !enabled || provider == nil || generation!=flow.Generation || providerName != flow.Provider || configured != flow.Provider { return Flow{}, "", fmt.Errorf("expired or changed OAuth flow") }
	if flow.Purpose == "bind" {
		session, _ := c.Cookie("session_token")
		if session == "" || session != flow.Session { return Flow{}, "", fmt.Errorf("binding session changed") }
	}
	return flow, state, nil
}
