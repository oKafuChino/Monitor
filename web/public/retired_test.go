package public

import (
	"github.com/gin-gonic/gin"
	"github.com/komari-monitor/komari/internal/config"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNormalSiteNeverServesLegacyTheme(t *testing.T) {
	old := defaultDistCacheDir
	defaultDistCacheDir = filepath.Join(t.TempDir(), "dist")
	t.Cleanup(func() { defaultDistCacheDir = old })
	if err := extractDistArchive(embeddedDistArchive, defaultDistCacheDir); err != nil {
		t.Fatal(err)
	}
	t.Chdir(t.TempDir())
	if err := os.MkdirAll("data/theme/legacy/dist", 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("data/theme/legacy/dist/index.html", []byte("LEGACY_THEME_EXECUTION"), 0644); err != nil {
		t.Fatal(err)
	}
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { _ = sqlDB.Close() })
	config.SetDb(db)
	if err := config.SetMany(map[string]any{config.ThemeKey: "legacy", config.CustomHeadKey: "<!--SITE_HEAD-->", config.CustomBodyKey: "<!--SITE_BODY-->"}); err != nil {
		t.Fatal(err)
	}
	r := gin.New()
	Static(r.Group("/"), func(h ...gin.HandlerFunc) { r.NoRoute(h...) })
	for _, p := range []string{"/", "/admin/files", "/themes/legacy/dist/index.html", "/themes/default/komari-theme.json", "/api/admin/theme/list", "/assets/missing.js"} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest("GET", p, nil))
		body := w.Body.String()
		if strings.Contains(body, "LEGACY_THEME_EXECUTION") {
			t.Fatalf("legacy theme served: %s", p)
		}
		if p == "/" {
			if w.Code != 200 || !strings.Contains(body, "SITE_HEAD") || !strings.Contains(body, "SITE_BODY") {
				t.Fatal("site customization lost")
			}
		} else if p == "/admin/files" {
			if w.Code != 200 || strings.Contains(body, "SITE_HEAD") {
				t.Fatal("admin injection isolation lost")
			}
		} else if w.Code != 404 {
			t.Fatalf("removed path fell into SPA: %s %d", p, w.Code)
		}
	}
}
