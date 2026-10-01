// Package uisettings stores settings for the single embedded interface.
// Historical theme rows remain inert for backup and rollback compatibility.
package uisettings

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sync"

	"github.com/komari-monitor/komari/database/models"
	"github.com/komari-monitor/komari/internal/config"
	"github.com/komari-monitor/komari/web/public"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const migrationKey = "_builtin_ui_migration_v1"

var writeMu sync.Mutex
var ErrInvalidSetting = errors.New("invalid UI setting")
var ErrCorruptSettings = errors.New("built-in UI settings contain invalid JSON; restore or repair the default configuration before saving")

type field struct {
	Key     string `json:"key"`
	Type    string `json:"type"`
	Default any    `json:"default"`
}

func schema() map[string]field {
	raw, err := public.PublicFS.ReadFile("defaultTheme/komari-theme.json")
	if err != nil {
		panic(err)
	}
	var manifest struct {
		Configuration struct {
			Data []field `json:"data"`
		} `json:"configuration"`
	}
	if err := json.Unmarshal(raw, &manifest); err != nil {
		panic(err)
	}
	fields := map[string]field{}
	for _, f := range manifest.Configuration.Data {
		if f.Key != "" {
			fields[f.Key] = f
		}
	}
	fields["_komari_dashboard_v1"] = field{Key: "_komari_dashboard_v1", Type: "array"}
	fields["_komari_onboarding_v1"] = field{Key: "_komari_onboarding_v1", Type: "object"}
	return fields
}

func valid(f field, value any) bool {
	switch f.Type {
	case "switch":
		_, ok := value.(bool)
		return ok
	case "number":
		_, ok := value.(float64)
		return ok
	case "array":
		_, ok := value.([]any)
		return ok
	case "object":
		_, ok := value.(map[string]any)
		return ok
	default:
		_, ok := value.(string)
		return ok
	}
}

func sanitize(values map[string]any) map[string]any {
	result := map[string]any{}
	for key, f := range schema() {
		if value, ok := values[key]; ok && valid(f, value) {
			result[key] = value
		}
	}
	if state, ok := result["_komari_onboarding_v1"].(map[string]any); ok {
		seen := []any{}
		if old, ok := state["seen"].([]any); ok {
			for _, id := range old {
				if id == "install" || id == "workbench" || id == "notifications" {
					seen = append(seen, id)
				}
			}
		}
		opened, _ := state["workbenchOpened"].(bool)
		result["_komari_onboarding_v1"] = map[string]any{"seen": seen, "workbenchOpened": opened}
	}
	return result
}

func defaults() map[string]any {
	result := map[string]any{}
	for key, f := range schema() {
		if f.Default != nil {
			result[key] = f.Default
		}
	}
	return result
}

func readRow(db *gorm.DB, short string) (map[string]any, error) {
	var row models.ThemeConfiguration
	err := db.Where("short = ?", short).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return map[string]any{}, nil
	}
	if err != nil {
		return nil, err
	}
	var values map[string]any
	if json.Unmarshal([]byte(row.Data), &values) != nil || values == nil {
		return nil, ErrCorruptSettings
	}
	return values, nil
}

// Read merges the immutable schema defaults with validated persisted values.
func Read(db *gorm.DB) (map[string]any, error) {
	values, err := readRow(db, "default")
	if err != nil && !errors.Is(err, ErrCorruptSettings) {
		return nil, err
	}
	result := defaults()
	for key, value := range sanitize(values) {
		result[key] = value
	}
	return result, nil
}

// Public excludes internal admin layout and onboarding state from the public API.
func Public(db *gorm.DB) (map[string]any, error) {
	values, err := Read(db)
	delete(values, "_komari_dashboard_v1")
	delete(values, "_komari_onboarding_v1")
	return values, err
}

func save(db *gorm.DB, values map[string]any) error {
	raw, err := json.Marshal(values)
	if err != nil {
		return err
	}
	return db.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "short"}}, DoUpdates: clause.AssignmentColumns([]string{"data"})}).Create(&models.ThemeConfiguration{Short: "default", Data: string(raw)}).Error
}

// Patch takes a write lock before reading: concurrent updates to distinct keys
// cannot overwrite one another, including SQLite connections in other processes.
func Patch(db *gorm.DB, patch map[string]any) (map[string]any, error) {
	fields := schema()
	for key, value := range patch {
		f, ok := fields[key]
		if !ok || !valid(f, value) {
			return nil, fmt.Errorf("%w: unknown key or invalid value type: %s", ErrInvalidSetting, key)
		}
	}
	writeMu.Lock()
	defer writeMu.Unlock()
	var result map[string]any
	err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&models.ThemeConfiguration{Short: "default", Data: "{}"}).Error; err != nil {
			return err
		}
		if err := tx.Model(&models.ThemeConfiguration{}).Where("short = ?", "default").UpdateColumn("data", gorm.Expr("data")).Error; err != nil {
			return err
		}
		values, err := readRow(tx, "default")
		if err != nil {
			return err
		}
		result = sanitize(values)
		for key, value := range patch {
			result[key] = value
		}
		result = sanitize(result)
		return save(tx, result)
	})
	return result, err
}

// Migrate receives an already-open database and never recursively initializes it.
// Invalid JSON rows are logged and preserved verbatim for recovery.
func Migrate(db *gorm.DB) error {
	writeMu.Lock()
	defer writeMu.Unlock()
	return db.Transaction(func(tx *gorm.DB) error {
		var marker config.ConfigItem
		err := tx.Where("key = ?", migrationKey).First(&marker).Error
		if err == nil {
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		var old config.ConfigItem
		var active string
		err = tx.Where("key = ?", config.ThemeKey).First(&old).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		_ = json.Unmarshal([]byte(old.Value), &active)
		current, err := readRow(tx, "default")
		corrupt := errors.Is(err, ErrCorruptSettings)
		if err != nil && !corrupt {
			return err
		}
		if corrupt {
			log.Printf("UI migration: damaged default settings preserved; using embedded defaults")
		}
		merged := sanitize(current)
		if active != "" && active != "default" {
			legacy, err := readRow(tx, active)
			if errors.Is(err, ErrCorruptSettings) {
				log.Printf("UI migration: damaged legacy settings %q preserved", active)
			} else if err != nil {
				return err
			}
			for key, value := range sanitize(legacy) {
				if _, ok := merged[key]; !ok {
					merged[key] = value
				}
			}
		}
		for key, value := range defaults() {
			if _, ok := merged[key]; !ok {
				merged[key] = value
			}
		}
		if !corrupt {
			if err := save(tx, merged); err != nil {
				return err
			}
		}
		previous, _ := json.Marshal(active)
		items := []config.ConfigItem{
			{Key: config.ThemeKey, Value: "\"default\""},
			{Key: "_builtin_ui_previous_theme_v1", Value: string(previous)},
			{Key: migrationKey, Value: "true"},
		}
		return tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "key"}}, DoUpdates: clause.AssignmentColumns([]string{"value"})}).Create(&items).Error
	})
}
