package github

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

func init() {

}

func (g *Github) GetName() string {
	return "github"
}
func (g *Github) GetConfiguration() factory.Configuration {
	return &g.Addition
}

func (g *Github) GetAuthorizationURL(_ string) (string, string) {
	state := utils.GenerateRandomString(16)

	// 构建GitHub OAuth授权URL
	authURL := fmt.Sprintf(
		"https://github.com/login/oauth/authorize?client_id=%s&state=%s&scope=user:email",
		url.QueryEscape(g.Addition.ClientId),
		url.QueryEscape(state),
	)
	g.stateCache.Set(state, true, cache.DefaultExpiration)
	return authURL, state
}
func (g *Github) OnCallback(ctx *gin.Context, state string, query map[string]string, _ string) (factory.OidcCallback, error) {
	code := query["code"]

	// 验证state防止CSRF攻击
	// state, _ := c.Cookie("oauth_state")
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
	//code := c.Query("code")
	if code == "" {
		return factory.OidcCallback{}, fmt.Errorf("no code provided")
	}

	// 获取访问令牌
	tokenURL := "https://github.com/login/oauth/access_token"
	data := url.Values{
		"client_id":     {g.Addition.ClientId},
		"client_secret": {g.Addition.ClientSecret},
		"code":          {code},
	}

	req, err := http.NewRequestWithContext(ctx.Request.Context(), "POST", tokenURL, strings.NewReader(data.Encode()))
	if err != nil { return factory.OidcCallback{}, fmt.Errorf("invalid provider endpoint") }
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	var tokenResp struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
		Scope       string `json:"scope"`
	}

	if err := factory.FetchJSON(req, &tokenResp); err != nil { return factory.OidcCallback{}, err }
	if tokenResp.AccessToken == "" { return factory.OidcCallback{}, fmt.Errorf("provider returned no access token") }

	// 获取用户信息
	userReq, err := http.NewRequestWithContext(ctx.Request.Context(), "GET", "https://api.github.com/user", nil)
	if err != nil { return factory.OidcCallback{}, fmt.Errorf("invalid provider user endpoint") }
	userReq.Header.Set("Authorization", "Bearer "+tokenResp.AccessToken)
	userReq.Header.Set("Accept", "application/json")

	var githubUser GitHubUser
	if err := factory.FetchJSON(userReq, &githubUser); err != nil { return factory.OidcCallback{}, err }

	return factory.OidcCallback{UserId: fmt.Sprintf("%d", githubUser.ID)}, nil
}
func (g *Github) Init() error {
	g.stateCache = cache.New(time.Minute*5, time.Minute*10)
	return nil
}
func (g *Github) Destroy() error {
	g.stateCache.Flush()
	return nil
}

var _ factory.IOidcProvider = (*Github)(nil)
