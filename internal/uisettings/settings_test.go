package uisettings

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"reflect"
	"sync"
	"testing"

	"github.com/komari-monitor/komari/database/models"
	"github.com/komari-monitor/komari/internal/config"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func testDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "settings.db")+"?_busy_timeout=5000&_journal_mode=WAL"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err = db.AutoMigrate(&models.ThemeConfiguration{}, &config.ConfigItem{}); err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { _ = sqlDB.Close() })
	return db
}
func insertRow(t *testing.T, db *gorm.DB, short, data string) {
	t.Helper()
	if err := db.Create(&models.ThemeConfiguration{Short: short, Data: data}).Error; err != nil {
		t.Fatal(err)
	}
}
func TestMigrationAndPublicFiltering(t *testing.T) {
	db := testDB(t)
	insertRow(t, db, "default", `{"mainContentWidth":0,"showIpTagsInCard":false,"backgroundImageUrlDesktop":"","thirdPartySecret":"hidden"}`)
	insertRow(t, db, "custom", `{"mainContentWidth":90,"backgroundImageUrlDesktop":"old.jpg","backgroundImageUrlMobile":"mobile.jpg","chartDashboardTemplate":"[]","_komari_dashboard_v1":[],"pluginKey":"hidden"}`)
	if err := db.Create(&config.ConfigItem{Key: config.ThemeKey, Value: `"custom"`}).Error; err != nil {
		t.Fatal(err)
	}
	if err := Migrate(db); err != nil {
		t.Fatal(err)
	}
	result, err := Read(db)
	if err != nil {
		t.Fatal(err)
	}
	if result["mainContentWidth"] != float64(0) || result["showIpTagsInCard"] != false || result["backgroundImageUrlDesktop"] != "" || result["backgroundImageUrlMobile"] != "mobile.jpg" {
		t.Fatalf("wrong merged settings: %#v", result)
	}
	if _, err := Patch(db, map[string]any{"mainContentWidth": float64(75)}); err != nil {
		t.Fatal(err)
	}
	// Backup or SQL changes cannot cause a second migration to overwrite user settings.
	db.Model(&config.ConfigItem{}).Where("key = ?", config.ThemeKey).Update("value", `"custom"`)
	if err := Migrate(db); err != nil {
		t.Fatal(err)
	}
	result, _ = Read(db)
	if result["mainContentWidth"] != float64(75) {
		t.Fatal("migration not idempotent")
	}
	exposed, err := Public(db)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"thirdPartySecret", "pluginKey", "_komari_dashboard_v1"} {
		if _, ok := exposed[key]; ok {
			t.Fatalf("public leak: %s", key)
		}
	}
	var legacy models.ThemeConfiguration
	db.Where("short = ?", "custom").First(&legacy)
	if legacy.Data == "" {
		t.Fatal("legacy row removed")
	}
}
func TestEmptyAndCorruptMigration(t *testing.T) {
	for _, raw := range []string{"", "{broken", "null", "[]"} {
		t.Run(raw, func(t *testing.T) {
			db := testDB(t)
			if raw != "" {
				insertRow(t, db, "default", raw)
			}
			if err := Migrate(db); err != nil {
				t.Fatal(err)
			}
			result, err := Read(db)
			if err != nil || result["showIpTagsInCard"] != true {
				t.Fatalf("defaults unavailable: %v %v", result, err)
			}
			if raw != "" {
				var row models.ThemeConfiguration
				db.First(&row, "short = ?", "default")
				if row.Data != raw {
					t.Fatal("damaged original overwritten")
				}
				if _, err = Patch(db, map[string]any{"showIpTagsInCard": false}); !errors.Is(err, ErrCorruptSettings) {
					t.Fatal("damaged row overwritten by patch")
				}
			}
		})
	}
}
func TestPatchFalseEmptyAndConcurrentKeys(t *testing.T) {
	db := testDB(t)
	if err := Migrate(db); err != nil {
		t.Fatal(err)
	}
	patches := []map[string]any{{"showIpTagsInCard": false}, {"mainContentWidth": float64(0)}, {"backgroundImageUrlDesktop": ""}, {"_komari_dashboard_v1": []any{}}, {"customFooterHtml": "footer"}}
	var wg sync.WaitGroup
	for _, patch := range patches {
		wg.Add(1)
		go func(p map[string]any) {
			defer wg.Done()
			if _, err := Patch(db, p); err != nil {
				t.Error(err)
			}
		}(patch)
	}
	wg.Wait()
	result, _ := Read(db)
	for _, patch := range patches {
		for key, value := range patch {
			if !reflect.DeepEqual(value, result[key]) {
				t.Fatalf("lost key %s: %#v", key, result)
			}
		}
	}
	for _, patch := range []map[string]any{{"theme": "custom"}, {"path": "../custom"}, {"showIpTagsInCard": "false"}, {"mainContentWidth": nil}, {"_komari_dashboard_v1": map[string]any{}}} {
		if _, err := Patch(db, patch); err == nil {
			t.Fatalf("invalid patch accepted: %v", patch)
		}
	}
}
func TestMigrationRollback(t *testing.T) {
	db := testDB(t)
	insertRow(t, db, "default", `{"mainContentWidth":30}`)
	if err := db.Exec(`CREATE TRIGGER reject_ui_marker BEFORE INSERT ON configs WHEN NEW.key = '_builtin_ui_migration_v1' BEGIN SELECT RAISE(ABORT, 'test failure'); END;`).Error; err != nil {
		t.Fatal(err)
	}
	if err := Migrate(db); err == nil {
		t.Fatal("expected transactional failure")
	}
	var row models.ThemeConfiguration
	db.First(&row, "short = ?", "default")
	if row.Data != `{"mainContentWidth":30}` {
		t.Fatal("settings committed despite failed marker")
	}
	var count int64
	db.Model(&config.ConfigItem{}).Count(&count)
	if count != 0 {
		t.Fatal("partial migration config committed")
	}
}
func TestCorruptLegacyAndInvalidTypes(t *testing.T) {
	db := testDB(t)
	insertRow(t, db, "custom", `{"showIpTagsInCard":"false","backgroundImageUrlMobile":123,"customFooterHtml":"kept"}`)
	db.Create(&config.ConfigItem{Key: config.ThemeKey, Value: `"custom"`})
	if err := Migrate(db); err != nil {
		t.Fatal(err)
	}
	data, _ := Read(db)
	if data["showIpTagsInCard"] != true || data["backgroundImageUrlMobile"] != "" || data["customFooterHtml"] != "kept" {
		t.Fatalf("bad validation: %v", data)
	}
	var old config.ConfigItem
	db.First(&old, "key = ?", "_builtin_ui_previous_theme_v1")
	var active string
	_ = json.Unmarshal([]byte(old.Value), &active)
	if active != "custom" {
		t.Fatal("rollback selection missing")
	}
}
