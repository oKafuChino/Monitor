package themes

import (
	"archive/zip"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/komari-monitor/komari/database/models"
	"github.com/komari-monitor/komari/internal/config"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

const testManifest = `{"short":"sakura","name":{"en":"Sakura"},"version":"1","configuration":{"type":"managed","data":[{"key":"enabled","type":"switch","default":true}]}}`

func themeZIP(t *testing.T, entries map[string]string) string {
	t.Helper()
	archive := filepath.Join(t.TempDir(), "theme.zip")
	f, err := os.Create(archive)
	if err != nil { t.Fatal(err) }
	w := zip.NewWriter(f)
	for name, body := range entries {
		entry, err := w.Create(name)
		if err != nil { t.Fatal(err) }
		if _, err := entry.Write([]byte(body)); err != nil { t.Fatal(err) }
	}
	if err := w.Close(); err != nil { t.Fatal(err) }
	if err := f.Close(); err != nil { t.Fatal(err) }
	return archive
}

func themeDB(t *testing.T) *gorm.DB {
	t.Helper()
	t.Chdir(t.TempDir())
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil { t.Fatal(err) }
	sqlDB, err := db.DB()
	if err != nil { t.Fatal(err) }
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	config.SetDb(db)
	if err := db.AutoMigrate(&models.ThemeConfiguration{}); err != nil { t.Fatal(err) }
	if err := config.Set(config.ThemeKey, Default); err != nil { t.Fatal(err) }
	return db
}

func TestLocalThemeLifecycleAndSettingsIsolation(t *testing.T) {
	db := themeDB(t)
	if err := db.Create(&models.ThemeConfiguration{Short:Default, Data:`{"admin":"preserved"}`}).Error; err != nil { t.Fatal(err) }
	archive := themeZIP(t, map[string]string{"komari-theme.json":testManifest, "dist/index.html":"<html><head></head><body>first</body></html>", "dist/assets/app.js":"export default 1"})
	if _, err := Install(archive); err != nil { t.Fatal(err) }
	if Active() != Default { t.Fatal("install activated a theme implicitly") }
	list, err := List()
	if err != nil || len(list) != 1 || list[0].Short != "sakura" { t.Fatalf("list: %#v %v", list, err) }
	values, err := Settings(db, "sakura")
	if err != nil || values["enabled"] != true { t.Fatalf("defaults: %#v %v", values, err) }
	if err := SaveSettings(db, "sakura", map[string]any{"enabled":false, "zero":float64(0), "empty":""}); err != nil { t.Fatal(err) }
	if err := Select("sakura"); err != nil { t.Fatal(err) }
	if Active() != "sakura" { t.Fatal("theme was not selected") }
	if err := Delete(db, "sakura"); err == nil { t.Fatal("active theme deleted") }
	if err := Delete(db, Default); err == nil { t.Fatal("built-in theme deleted") }
	if err := SaveSettings(db, Default, map[string]any{}); err == nil { t.Fatal("custom endpoint changed built-in settings") }
	if err := SaveSettings(db, "sakura", map[string]any{"_komari_dashboard_v1":[]any{}}); err == nil { t.Fatal("admin setting key accepted") }
	// A broken update must not remove the installed package or its configuration.
	broken := themeZIP(t, map[string]string{"komari-theme.json":testManifest, "dist/assets/app.js":"bad"})
	if _, err := Install(broken); err == nil { t.Fatal("missing index accepted") }
	data, err := ReadFile("sakura", "dist/index.html")
	if err != nil || !strings.Contains(string(data), "first") { t.Fatal("failed update damaged installed theme") }
	updated := themeZIP(t, map[string]string{"komari-theme.json":testManifest, "dist/index.html":"updated"})
	if _, err := Install(updated); err != nil { t.Fatal(err) }
	data, err = ReadFile("sakura", "dist/index.html")
	if err != nil || string(data) != "updated" { t.Fatal("updated theme not served") }
	values, err = Settings(db, "sakura")
	if err != nil || values["enabled"] != false || values["zero"] != float64(0) || values["empty"] != "" { t.Fatalf("settings lost: %#v %v", values, err) }
	var builtin models.ThemeConfiguration
	if err := db.First(&builtin, "short = ?", Default).Error; err != nil || builtin.Data != `{"admin":"preserved"}` { t.Fatal("built-in settings changed") }
	if err := Select(Default); err != nil { t.Fatal(err) }
	if err := Delete(db, "sakura"); err != nil { t.Fatal(err) }
	if _, err := ReadFile("sakura", "dist/index.html"); err == nil { t.Fatal("deleted theme still available") }
	var count int64
	if err := db.Model(&models.ThemeConfiguration{}).Where("short = ?", "sakura").Count(&count).Error; err != nil || count != 0 { t.Fatal("deleted settings retained") }
}

func TestThemeValidationAndFallback(t *testing.T) {
	themeDB(t)
	for _, name := range []string{"../escape", "default", "DEFAULT", "a/b", "a\\b", "CON", "..", "a:", ""} {
		if ValidShort(name) { t.Fatalf("unsafe short accepted: %q", name) }
		if err := Select(name); err == nil && name != Default { t.Fatalf("missing theme selected: %q", name) }
	}
	if err := config.Set(config.ThemeKey, "missing"); err != nil { t.Fatal(err) }
	if Active() != Default { t.Fatal("missing theme did not fall back") }
	for name, files := range map[string]map[string]string{
		"traversal":{"komari-theme.json":testManifest, "dist/index.html":"ok", "../escaped":"bad"},
		"windows traversal":{"komari-theme.json":testManifest, "dist/index.html":"ok", `dist\..\escaped`:"bad"},
		"absolute":{"komari-theme.json":testManifest, "dist/index.html":"ok", "/escaped":"bad"},
		"private files":{"komari-theme.json":testManifest, "dist/index.html":"ok", "server.env":"bad"},
		"builtin overwrite":{"komari-theme.json":strings.Replace(testManifest, "sakura", "default", 1), "dist/index.html":"bad"},
		"case collision":{"komari-theme.json":testManifest, "dist/index.html":"ok", "dist/A.js":"1", "dist/a.js":"2"},
		"invalid JSON":{"komari-theme.json":"{", "dist/index.html":"ok"},
	} {
		t.Run(name, func(t *testing.T) { if _, err := Install(themeZIP(t, files)); err == nil { t.Fatal("invalid package accepted") } })
	}
	for _, target := range []string{"../../secret", "dist/../../secret", "dist/C:/secret", "server.env"} {
		if _, err := ReadFile("sakura", target); err == nil { t.Fatalf("private path accepted: %s", target) }
	}
	archive := themeZIP(t, map[string]string{"komari-theme.json":testManifest, "dist/index.html":"ok"})
	if _, err := Install(archive); err != nil { t.Fatal(err) }
	if err := config.Set(config.ThemeKey, "sakura"); err != nil { t.Fatal(err) }
	if err := os.Remove(filepath.Join(Root, "sakura", "dist", "index.html")); err != nil { t.Fatal(err) }
	if Active() != Default { t.Fatal("incomplete theme did not fall back") }
}

func TestThemeRejectsSymlinksAndOversizedArchives(t *testing.T) {
	themeDB(t)
	archive := filepath.Join(t.TempDir(), "symlink.zip")
	f, err := os.Create(archive)
	if err != nil { t.Fatal(err) }
	w := zip.NewWriter(f)
	header := &zip.FileHeader{Name:"dist/link"}
	header.SetMode(os.ModeSymlink | 0777)
	entry, err := w.CreateHeader(header)
	if err != nil { t.Fatal(err) }
	if _, err := entry.Write([]byte("../../private")); err != nil { t.Fatal(err) }
	if err := w.Close(); err != nil { t.Fatal(err) }
	if err := f.Close(); err != nil { t.Fatal(err) }
	if _, err := Install(archive); err == nil { t.Fatal("symlink accepted") }
	f, err = os.Create(filepath.Join(t.TempDir(), "oversized.zip"))
	if err != nil { t.Fatal(err) }
	if err := f.Truncate(MaxArchiveSize+1); err != nil { t.Fatal(err) }
	if err := f.Close(); err != nil { t.Fatal(err) }
	if _, err := Install(f.Name()); err == nil { t.Fatal("oversized archive accepted") }
}

func TestPrepareHTMLNamespacesAssetsButNotAPIsOrNavigation(t *testing.T) {
	source := []byte(`<html><head><base href="/"><link href="/assets/style.css"></head><body><script src="/assets/main.js"></script><img src="image.png"><a href="/instance/node">Node</a><script src="https://example.invalid/x.js"></script><script src="/api/custom.js"></script></body></html>`)
	output, err := PrepareHTML(source, "sakura")
	if err != nil { t.Fatal(err) }
	text := string(output)
	for _, expected := range []string{`<base href="/themes/sakura/dist/"`, `/themes/sakura/dist/assets/main.js`, `/themes/sakura/dist/assets/style.css`, `/themes/sakura/dist/image.png`, `href="/instance/node"`, `src="https://example.invalid/x.js"`, `src="/api/custom.js"`} {
		if !strings.Contains(text, expected) { t.Fatalf("missing %s in %s", expected, text) }
	}
	if strings.Contains(text, `<base href="/"`) { t.Fatal("old base retained") }
}
