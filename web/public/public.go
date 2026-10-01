package public

import (
	"embed"
	"io/fs"
	"mime"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/komari-monitor/komari/internal/config"
)

//go:embed defaultTheme/komari-theme.json
var PublicFS embed.FS

//go:embed defaultTheme/dist.tar.zst
var embeddedDistArchive []byte

// 常量定义
const (
	DataDir            = "./data"
	FaviconFile        = "favicon.ico"
	DefaultTheme       = "default"
	LanguageCookieName = "language"

	// 主题内部结构定义
	DistDir   = "dist"       // 静态资源存放目录
	IndexFile = "index.html" // 相对于 DistDir
)

// isRetiredFrontendPath prevents removed feature entry points from being
// mistaken for valid client-side routes by the SPA fallback.
func isRetiredFrontendPath(requestPath string) bool {
	for _, prefix := range []string{
		"/admin/files",
		"/admin/notification",
		"/admin/settings/notification",
		"/terminal",
	} {
		if requestPath == prefix || strings.HasPrefix(requestPath, prefix+"/") {
			return true
		}
	}
	return false
}

func normalizeHTMLLanguage(language string) string {
	language = strings.TrimSpace(strings.ReplaceAll(language, "_", "-"))
	if len(language) < 2 || len(language) > 32 {
		return ""
	}

	for _, r := range language {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' {
			continue
		}
		return ""
	}

	return language
}

func replaceHTMLLanguage(htmlStr, language string) string {
	language = normalizeHTMLLanguage(language)
	if language == "" {
		return htmlStr
	}

	replacements := []struct {
		old string
		new string
	}{
		{`<html lang="en">`, `<html lang="` + language + `">`},
		{`<html lang='en'>`, `<html lang='` + language + `'>`},
		{`<html>`, `<html lang="` + language + `">`},
	}

	for _, replacement := range replacements {
		if strings.Contains(htmlStr, replacement.old) {
			return strings.Replace(htmlStr, replacement.old, replacement.new, 1)
		}
	}

	return htmlStr
}

func stripServiceWorkerRegistration(html string) string {
	return strings.ReplaceAll(html, `<script id="vite-plugin-pwa:register-sw" src="/registerSW.js"></script>`, "")
}

// Static 注册静态资源和 SPA 路由处理
func Static(r *gin.RouterGroup, noRoute func(handlers ...gin.HandlerFunc)) {
	static(r, noRoute, false)
}

// StaticRestricted serves only the embedded default frontend. Restricted
// startup listeners must not let an installed theme override same-named JS,
// CSS, manifest, or favicon assets used by login and recovery pages.
func StaticRestricted(r *gin.RouterGroup, noRoute func(handlers ...gin.HandlerFunc)) {
	static(r, noRoute, true)
}

func static(r *gin.RouterGroup, noRoute func(handlers ...gin.HandlerFunc), forceDefaultTheme bool) {
	// 初始化嵌入式文件系统，指向 defaultTheme 根目录。
	defaultThemeFS, err := fs.Sub(PublicFS, "defaultTheme")
	if err != nil {
		panic("embedded default theme metadata is unavailable: " + err.Error())
	}

	getConfig := func() map[string]any {
		cfg, _ := config.GetMany(map[string]any{
			config.DescriptionKey: "A simple server monitor tool.",
			config.CustomHeadKey:  "",
			config.CustomBodyKey:  "",
			config.SitenameKey:    "Komari Monitor",
		})
		return cfg
	}

	// 核心逻辑：获取文件内容
	// filePath: 相对于主题根目录的路径 (例如 "theme.json" 或 "dist/assets/a.js")
	// 返回: content, contentType, exists
	getFileContent := func(relativePath string) ([]byte, string, bool) {
		cleanPath := strings.TrimPrefix(relativePath, "/")

		cleanPath = filepath.Clean(cleanPath)

		// 2. 尝试从嵌入式 defaultTheme/{cleanPath} 读取
		// fs.ReadFile 处理 embed 路径时使用 "/"
		embedPath := filepath.ToSlash(cleanPath)

		if strings.Contains(embedPath, "..") {
			return nil, "", false
		}

		if strings.HasPrefix(embedPath, DistDir+"/") {
			content, err := os.ReadFile(filepath.Join(defaultDistCacheDir, filepath.FromSlash(strings.TrimPrefix(embedPath, DistDir+"/"))))
			if err == nil {
				return content, mime.TypeByExtension(filepath.Ext(embedPath)), true
			}
		} else if content, err := fs.ReadFile(defaultThemeFS, embedPath); err == nil {
			return content, mime.TypeByExtension(filepath.Ext(embedPath)), true
		}

		return nil, "", false
	}

	// 核心逻辑：渲染 Index.html
	serveIndex := func(c *gin.Context) {
		reqPath := c.Request.URL.Path
		cfg := getConfig()

		shouldReplace := true

		// 特殊页面：强制使用 default 主题，且不进行内容替换
		if forceDefaultTheme || strings.HasPrefix(reqPath, "/admin") {
			shouldReplace = false
		}

		// 获取 dist/index.html (相对于主题根目录)
		targetFile := path.Join(DistDir, IndexFile)
		content, _, exists := getFileContent(targetFile)

		if !exists {
			c.String(http.StatusNotFound, "Embedded index file missing.")
			return
		}

		htmlStr := string(content)
		if forceDefaultTheme {
			htmlStr = stripServiceWorkerRegistration(htmlStr)
		}
		if language, err := c.Cookie(LanguageCookieName); err == nil {
			htmlStr = replaceHTMLLanguage(htmlStr, language)
		}

		// 如果不替换，保留系统内置页面内容，仅同步 html lang。
		if !shouldReplace {
			c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(htmlStr))
			return
		}

		// 执行 HTML 内容替换
		replacer := strings.NewReplacer(
			"<title>Komari Monitor</title>", "<title>"+cfg[config.SitenameKey].(string)+"</title>",
			"A simple server monitor tool.", cfg[config.DescriptionKey].(string),
			"</head>", cfg[config.CustomHeadKey].(string)+"</head>",
			"</body>", cfg[config.CustomBodyKey].(string)+"</body>",
		)

		c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(replacer.Replace(htmlStr)))
	}

	// ================= 路由定义 =================
	// 1. Favicon 优先策略
	r.GET("/favicon.ico", func(c *gin.Context) {
		// 优先：./data/favicon.ico
		localFavicon := filepath.Join(DataDir, FaviconFile)
		if !forceDefaultTheme {
			if _, err := os.Stat(localFavicon); err == nil {
				c.File(localFavicon)
				return
			}
		}

		// 其次：当前主题的 dist/favicon.ico 或 theme_root/favicon.ico ?
		// 通常构建后的资源在 dist 中，这里假设优先找 dist 内的，如果你的 favicon 在根目录，去掉 DistDir 拼接即可
		themeFaviconPath := path.Join(DistDir, FaviconFile)
		content, mimeType, exists := getFileContent(themeFaviconPath)
		if exists {
			c.Data(http.StatusOK, mimeType, content)
			return
		}

		c.Status(http.StatusNotFound)
	})

	// 3. SPA 路由 (noRoute)
	noRoute(func(c *gin.Context) {
		if strings.HasPrefix(c.Request.URL.Path, "/api/") || strings.HasPrefix(c.Request.URL.Path, "/themes/") {
			c.JSON(http.StatusNotFound, gin.H{"status": "error", "message": "Not found"})
			return
		}
		if c.Request.Method != http.MethodGet {
			c.Status(http.StatusNotFound)
			return
		}
		//
		func() {
			tempKey := c.Query("temp_key")
			if tempKey == "" {
				return
			}

			tempKeyExpireTime, err := config.GetAs[int64]("tempory_share_token_expire_at", 0)
			if err != nil {
				return
			}
			allowTempKey, err := config.GetAs[string]("tempory_share_token", "")
			if err != nil {
				return
			}

			if allowTempKey == "" || tempKey != allowTempKey {
				return
			}
			now := time.Now().Unix()
			if tempKeyExpireTime < now {
				return
			}
			expireSeconds := int(tempKeyExpireTime - now)
			if expireSeconds > 0 {
				c.SetCookie(
					"temp_key",    // key
					tempKey,       // value
					expireSeconds, // maxAge（秒）
					"/",           // path
					"",            // domain
					false,         // secure
					false,         // httpOnly
				)
			}
		}()
		reqPath := c.Request.URL.Path
		if isRetiredFrontendPath(reqPath) {
			c.Status(http.StatusNotFound)
			return
		}

		// SPA 静态资源回退
		distPath := path.Join(DistDir, reqPath)

		content, mimeType, exists := getFileContent(distPath)
		if exists {
			c.Data(http.StatusOK, mimeType, content)
			return
		}

		if ext := filepath.Ext(reqPath); ext != "" && ext != ".html" {
			c.Status(http.StatusNotFound)
			return
		}

		// 路由 (如 /dashboard, /settings) -> 返回 index.html
		serveIndex(c)
	})
}
