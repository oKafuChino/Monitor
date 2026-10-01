package config

import "time"

type Settings struct {
	ID                     uint   `json:"id,omitempty"`                                        // 1
	Sitename               string `json:"sitename" default:"Komari"`                           // 站点名称，默认 "Komari"
	Description            string `json:"description" default:"A simple server monitor tool."` // 站点描述
	CorsOriginCheckEnabled bool   `json:"cors_origin_check_enabled" default:"true"`            // 是否启用 API CORS 跨域请求校验，默认 true
	CorsAllowedOrigins     string `json:"cors_allowed_origins" default:""`                     // API 跨域允许列表
	WsOriginCheckEnabled   bool   `json:"ws_origin_check_enabled" default:"true"`              // 是否校验 WebSocket Origin
	WsAllowedOrigins       string `json:"ws_allowed_origins" default:""`                       // WebSocket Origin 允许列表
	Theme                  string `json:"theme" default:"default"`                             // 主题名称，默认 'default'
	PrivateSite            bool   `json:"private_site" default:"false"`                        // 是否为私有站点，默认 false
	ApiKey                 string `json:"api_key" default:""`                                  // API 密钥，默认空字符串
	AutoDiscoveryKey       string `json:"auto_discovery_key" default:""`                       // 自动发现密钥
	ScriptDomain           string `json:"script_domain" default:""`                            // 自定义脚本域名
	SendIpAddrToGuest      bool   `json:"send_ip_addr_to_guest" default:"false"`               // 是否向访客页面发送 IP 地址，默认 false
	SSRFProtectionEnabled  bool   `json:"ssrf_protection_enabled" default:"false"`             // 是否启用 SSRF 防护，默认关闭
	EulaAccepted           bool   `json:"eula_accepted" default:"false"`
	BaseScriptsURLKey      string `json:"base_scripts_url" default:""`
	// GeoIP 配置
	GeoIpEnabled  bool   `json:"geo_ip_enabled" default:"true"`
	GeoIpProvider string `json:"geo_ip_provider" default:"ipinfo"` // empty, mmdb, ip-api, geojs
	// OAuth 配置
	OAuthEnabled         bool   `json:"o_auth_enabled" default:"false"`
	OAuthProvider        string `json:"o_auth_provider" default:"github"`
	DisablePasswordLogin bool   `json:"disable_password_login" default:"false"`
	// 自定义美化
	CustomHead string `json:"custom_head" default:""`
	CustomBody string `json:"custom_body" default:""`

	UpdatedAt                  time.Time
}

const (
	SitenameKey               = "sitename"
	DescriptionKey            = "description"
	CorsOriginCheckEnabledKey = "cors_origin_check_enabled"
	CorsAllowedOriginsKey     = "cors_allowed_origins"
	WsOriginCheckEnabledKey   = "ws_origin_check_enabled"
	WsAllowedOriginsKey       = "ws_allowed_origins"
	ThemeKey                  = "theme"
	PrivateSiteKey            = "private_site"
	ApiKeyKey                 = "api_key"
	AutoDiscoveryKeyKey       = "auto_discovery_key"
	ScriptDomainKey           = "script_domain"
	SendIpAddrToGuestKey      = "send_ip_addr_to_guest"
	SSRFProtectionEnabledKey  = "ssrf_protection_enabled"
	EulaAcceptedKey           = "eula_accepted"
	BaseScriptsURLKey         = "base_scripts_url"
	GeoIpEnabledKey           = "geo_ip_enabled"
	GeoIpProviderKey          = "geo_ip_provider"
	OAuthEnabledKey           = "o_auth_enabled"
	OAuthProviderKey          = "o_auth_provider"
	DisablePasswordLoginKey   = "disable_password_login"
	CustomHeadKey             = "custom_head"
	CustomBodyKey             = "custom_body"

	UpdatedAtKey                  = "updated_at"
	ThemeMarketSourcesKey         = "theme_market_sources"
	PluginMarketSourcesKey        = "plugin_market_sources"
)
