package generic

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/komari-monitor/komari/utils"
	"github.com/komari-monitor/komari/web/oauth/factory"
	"github.com/patrickmn/go-cache"
)

func (g *Generic) GetName() string {
	return "generic"
}
func (g *Generic) GetConfiguration() factory.Configuration {
	return &g.Addition
}

func (g *Generic) GetAuthorizationURL(redirectURI string) (string, string) {
	state := utils.GenerateRandomString(16)

	// 构建GitHub OAuth授权URL
	authURL := fmt.Sprintf(
		"%s?client_id=%s&state=%s&scope=%s&redirect_uri=%s&response_type=code",
		g.Addition.AuthURL,
		url.QueryEscape(g.Addition.ClientId),
		url.QueryEscape(state),
		url.QueryEscape(g.Addition.Scope),
		url.QueryEscape(redirectURI),
	)
	g.stateCache.Set(state, true, cache.DefaultExpiration)
	return authURL, state
}
func (g *Generic) OnCallback(ctx *gin.Context, state string, query map[string]string, callbackURI string) (factory.OidcCallback, error) {
	code := query["code"]

	// 验证state防止CSRF攻击
	if g.stateCache == nil {
		return factory.OidcCallback{}, fmt.Errorf("state cache not initialized")
	}
	if _, ok := g.stateCache.Get(state); !ok {
		return factory.OidcCallback{}, fmt.Errorf("invalid state")
	}
	if state == "" {
		return factory.OidcCallback{}, fmt.Errorf("invalid state")
	}

	g.stateCache.Delete(state)
	// 获取code
	if code == "" {
		return factory.OidcCallback{}, fmt.Errorf("no code provided")
	}

	// 获取访问令牌
	data := url.Values{
		"client_id":     {g.Addition.ClientId},
		"client_secret": {g.Addition.ClientSecret},
		"code":          {code},
		"redirect_uri":  {callbackURI},
		"grant_type":    {"authorization_code"},
	}

	req, err := http.NewRequestWithContext(ctx.Request.Context(), "POST", g.Addition.TokenURL, strings.NewReader(data.Encode()))
	if err != nil { return factory.OidcCallback{}, fmt.Errorf("invalid provider endpoint") }
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	var tokenResp struct {
		AccessToken string `json:"access_token"`
	}

	if err := factory.FetchJSON(req, &tokenResp); err != nil { return factory.OidcCallback{}, err }
	if tokenResp.AccessToken == "" { return factory.OidcCallback{}, fmt.Errorf("provider returned no access token") }

	// 获取用户信息
	userReq, err := http.NewRequestWithContext(ctx.Request.Context(), "GET", g.Addition.UserInfoURL, nil)
	if err != nil { return factory.OidcCallback{}, fmt.Errorf("invalid provider user endpoint") }
	userReq.Header.Set("Authorization", "Bearer "+tokenResp.AccessToken)
	userReq.Header.Set("Accept", "application/json")

	var user map[string]interface{}
	if err := factory.FetchJSON(userReq, &user); err != nil { return factory.OidcCallback{}, err }

	userId, ok := user[g.Addition.UserIDField]
	if !ok {
		return factory.OidcCallback{}, fmt.Errorf("user id field '%s' not found in user info response", g.Addition.UserIDField)
	}

	return factory.OidcCallback{UserId: fmt.Sprintf("%v", userId)}, nil
}
func (g *Generic) Init() error {
	g.stateCache = cache.New(time.Minute*5, time.Minute*10)
	return nil
}
func (g *Generic) Destroy() error {
	g.stateCache.Flush()
	return nil
}

var _ factory.IOidcProvider = (*Generic)(nil)
