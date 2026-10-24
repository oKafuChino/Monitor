package public

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/komari-monitor/komari/internal/config"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestCustomThemePublicRenderingAndSystemIsolation(t *testing.T) {
	t.Chdir(t.TempDir())
	old := defaultDistCacheDir
	defaultDistCacheDir = filepath.Join(t.TempDir(), "dist")
	t.Cleanup(func() { defaultDistCacheDir = old })
	if err := extractDistArchive(embeddedDistArchive, defaultDistCacheDir); err != nil { t.Fatal(err) }
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil { t.Fatal(err) }
	sqlDB, _ := db.DB()
	t.Cleanup(func() { _ = sqlDB.Close() })
	config.SetDb(db)
	assets, err := filepath.Glob(filepath.Join(defaultDistCacheDir, "assets", "*.css"))
	if err != nil || len(assets) == 0 { t.Fatal("embedded CSS missing") }
	sameName := filepath.Base(assets[0])
	for name, content := range map[string]string{
		"komari-theme.json":`{"short":"custom","name":"Custom"}`,
		"dist/index.html":`<html lang="en"><head><script src="/assets/custom.js"></script></head><body>CUSTOM_THEME</body></html>`,
		"dist/assets/custom.js":"CUSTOM_ASSET",
		"dist/assets/"+sameName:"CUSTOM_OVERRIDE",
		"dist/sw.js":"CUSTOM_SW",
		"private.txt":"PRIVATE",
	} {
		target := filepath.Join("data/theme/custom", filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil { t.Fatal(err) }
		if err := os.WriteFile(target, []byte(content), 0644); err != nil { t.Fatal(err) }
	}
	if err := config.Set(config.ThemeKey, "custom"); err != nil { t.Fatal(err) }
	r := gin.New()
	Static(r.Group("/"), func(h ...gin.HandlerFunc) { r.NoRoute(h...) })
	request := func(target string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest("GET", target, nil))
		return w
	}
	for _, target := range []string{"/", "/index.html", "/instance/uuid"} {
		w := request(target)
		if w.Code != 200 || !strings.Contains(w.Body.String(), "CUSTOM_THEME") || !strings.Contains(w.Body.String(), "/themes/custom/dist/assets/custom.js") { t.Fatalf("custom page %s: %d %s", target, w.Code, w.Body) }
		if w.Header().Get("Cache-Control") != "no-store" { t.Fatal("theme page can be cached") }
	}
	for _, target := range []string{"/admin/login", "/admin/settings/appearance", "/install", "/database-recovery", "/assets/"+sameName, "/sw.js"} {
		w := request(target)
		if w.Code != 200 || strings.Contains(w.Body.String(), "CUSTOM_") { t.Fatalf("custom theme leaked into system path %s: %d", target, w.Code) }
	}
	for _, target := range []string{"/themes/custom/dist/assets/custom.js", "/assets/custom.js"} {
		w := request(target)
		if w.Code != 200 || w.Body.String() != "CUSTOM_ASSET" { t.Fatalf("custom asset %s: %d %s", target, w.Code, w.Body) }
		if w.Header().Get("Service-Worker-Allowed") != "/themes/custom/dist/" { t.Fatal("custom worker scope can escape theme namespace") }
	}
	for _, target := range []string{"/themes/custom/private.txt", "/themes/custom/dist/../../private.txt", "/themes/missing/dist/index.html"} {
		if w := request(target); w.Code != 404 { t.Fatalf("invalid asset exposed: %s %d", target, w.Code) }
	}
	if err := config.Set(config.ThemeKey, "missing"); err != nil { t.Fatal(err) }
	if w := request("/"); w.Code != 200 || strings.Contains(w.Body.String(), "CUSTOM_THEME") { t.Fatal("missing theme did not fall back to built-in") }
	if err := config.Set(config.ThemeKey, "default"); err != nil { t.Fatal(err) }
	if w := request("/"); w.Code != 200 || strings.Contains(w.Body.String(), "CUSTOM_THEME") { t.Fatal("reset did not restore default") }
	if w := request("/assets/custom.js"); w.Code != 404 { t.Fatal("inactive root asset fallback retained") }
}
