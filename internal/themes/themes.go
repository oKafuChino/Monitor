// Package themes manages trusted, locally uploaded frontend packages.
// It deliberately has no remote download, update or marketplace client.
package themes

import (
	"archive/zip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/komari-monitor/komari/database/models"
	"github.com/komari-monitor/komari/internal/config"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const Default = "default"
const MaxArchiveSize int64 = 64 << 20
const maxExpandedSize int64 = 256 << 20
const maxFileSize int64 = 32 << 20
const maxManifestSize int64 = 1 << 20

var Root = filepath.Join(".", "data", "theme")
var mu sync.RWMutex
var identifier = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,63}$`)
var deviceName = regexp.MustCompile(`^(CON|PRN|AUX|NUL|COM[1-9]|LPT[1-9])$`)

type Manifest struct {
	Short string `json:"short"`
	Name any `json:"name"`
	Description any `json:"description,omitempty"`
	Version string `json:"version"`
	Author any `json:"author,omitempty"`
	URL string `json:"url,omitempty"`
	Preview string `json:"preview,omitempty"`
	Configuration models.Configuration `json:"configuration"`
}

func ValidShort(short string) bool {
	if !identifier.MatchString(short) || strings.EqualFold(short, Default) { return false }
	// Windows reserves these device names, even on otherwise valid paths.
	switch strings.ToUpper(short) { case "CON", "PRN", "AUX", "NUL", "COM1", "COM2", "COM3", "COM4", "COM5", "COM6", "COM7", "COM8", "COM9", "LPT1", "LPT2", "LPT3", "LPT4", "LPT5", "LPT6", "LPT7", "LPT8", "LPT9": return false }
	return true
}

func safePath(name string) bool {
	if !fs.ValidPath(name) || strings.ContainsAny(name, "\\:\x00<>\"|?*") { return false }
	for _, r := range name { if r < 32 || r == 127 { return false } }
	for _, part := range strings.Split(name, "/") {
		if strings.TrimRight(part, " .") != part || strings.HasPrefix(part, ".") { return false }
		stem := strings.ToUpper(strings.SplitN(part, ".", 2)[0])
		if deviceName.MatchString(stem) { return false }
	}
	return true
}

func readAt(rootPath, name string, limit int64) ([]byte, error) {
	if !safePath(name) { return nil, fmt.Errorf("invalid theme path") }
	info, err := os.Lstat(rootPath)
	if err != nil { return nil, err }
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 { return nil, fmt.Errorf("theme directory must not be a symlink") }
	root, err := os.OpenRoot(rootPath)
	if err != nil { return nil, err }
	defer root.Close()
	// OpenRoot confines symlinks to the theme directory; packages cannot escape it.
	f, err := root.Open(filepath.FromSlash(name))
	if err != nil { return nil, err }
	defer f.Close()
	info, err = f.Stat()
	if err != nil { return nil, err }
	if !info.Mode().IsRegular() || info.Size() > limit { return nil, fmt.Errorf("invalid or oversized theme file") }
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if int64(len(data)) > limit { return nil, fmt.Errorf("oversized theme file") }
	return data, err
}

func manifestAt(rootPath, short string) (Manifest, error) {
	var m Manifest
	if !ValidShort(short) { return m, fmt.Errorf("invalid theme identifier") }
	data, err := readAt(rootPath, "komari-theme.json", maxManifestSize)
	if err != nil { return m, err }
	if err := json.Unmarshal(data, &m); err != nil { return m, fmt.Errorf("invalid komari-theme.json: %w", err) }
	if m.Short != short || !hasName(m.Name) { return m, fmt.Errorf("theme identifier/name is invalid") }
	if m.Configuration.Type == "" || m.Configuration.Type == "managed" {
		var fields []models.ManagedThemeConfigurationItem
		raw, err := json.Marshal(m.Configuration.Data)
		if err != nil { return m, err }
		if err := json.Unmarshal(raw, &fields); err != nil { return m, fmt.Errorf("managed configuration must be an array of fields: %w", err) }
		if len(fields) > 1024 { return m, fmt.Errorf("too many theme configuration fields") }
		seen := map[string]bool{}
		for _, field := range fields {
			if !localized(field.Name) || !localized(field.Help) { return m, fmt.Errorf("invalid localized configuration label") }
			if field.Key == "" { continue }
			if len(field.Key) > 128 || seen[field.Key] || strings.HasPrefix(field.Key, "_komari_") { return m, fmt.Errorf("invalid or duplicate configuration key") }
			seen[field.Key] = true
		}
	}
	if _, err := readAt(rootPath, "dist/index.html", maxFileSize); err != nil { return m, fmt.Errorf("dist/index.html is required: %w", err) }
	return m, nil
}

func hasName(value any) bool {
	if !localized(value) { return false }
	switch v := value.(type) {
	case string: return strings.TrimSpace(v) != ""
	case map[string]any:
		for _, text := range v { if s, ok := text.(string); ok && strings.TrimSpace(s) != "" { return true } }
	}
	return false
}

func localized(value any) bool {
	switch v := value.(type) {
	case nil, string: return true
	case map[string]any:
		for _, text := range v { if _, ok := text.(string); !ok { return false } }
		return true
	}
	return false
}

func ManifestFor(short string) (Manifest, error) {
	mu.RLock(); defer mu.RUnlock()
	return manifestAt(filepath.Join(Root, short), short)
}

func List() ([]Manifest, error) {
	mu.RLock(); defer mu.RUnlock()
	entries, err := os.ReadDir(Root)
	if os.IsNotExist(err) { return []Manifest{}, nil }
	if err != nil { return nil, err }
	result := []Manifest{}
	for _, entry := range entries {
		if !entry.IsDir() || !ValidShort(entry.Name()) { continue }
		m, err := manifestAt(filepath.Join(Root, entry.Name()), entry.Name())
		if err == nil { result = append(result, m) }
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Short < result[j].Short })
	return result, nil
}

// Active falls back without erasing the selected theme or its configuration.
func Active() string {
	short, err := config.GetAs[string](config.ThemeKey)
	if err != nil || short == Default { return Default }
	if _, err := ManifestFor(short); err != nil { return Default }
	return short
}

func Select(short string) error {
	mu.Lock(); defer mu.Unlock()
	if short != Default {
		if _, err := manifestAt(filepath.Join(Root, short), short); err != nil { return err }
	}
	return config.Set(config.ThemeKey, short)
}

func Delete(db *gorm.DB, short string) error {
	mu.Lock(); defer mu.Unlock()
	if !ValidShort(short) { return fmt.Errorf("the built-in theme cannot be deleted") }
	selected, err := config.GetAs[string](config.ThemeKey)
	if err != nil { return err }
	if selected == short { return fmt.Errorf("switch to the default theme before deleting the active theme") }
	// Rename first, so failure to delete never leaves a partial installed theme.
	target := filepath.Join(Root, short)
	if _, err := os.Lstat(target); err != nil { return err }
	staging, err := os.MkdirTemp(Root, ".delete-")
	if err != nil { return err }
	cleanup := true
	defer func() { if cleanup { _ = os.RemoveAll(staging) } }()
	tomb := filepath.Join(staging, "theme")
	if err := os.Rename(target, tomb); err != nil { return err }
	if err := db.Where("short = ?", short).Delete(&models.ThemeConfiguration{}).Error; err != nil {
		if rollbackErr := os.Rename(tomb, target); rollbackErr != nil {
			cleanup = false
			return fmt.Errorf("delete settings: %w; theme files preserved at %s: %v", err, tomb, rollbackErr)
		}
		return err
	}
	return nil
}

func ReadFile(short, name string) ([]byte, error) {
	mu.RLock(); defer mu.RUnlock()
	if !ValidShort(short) || (name != "komari-theme.json" && !strings.HasPrefix(name, "dist/")) { return nil, fs.ErrNotExist }
	// Do not expose incomplete, invalid or manually corrupted theme directories.
	if _, err := manifestAt(filepath.Join(Root, short), short); err != nil { return nil, err }
	return readAt(filepath.Join(Root, short), name, maxFileSize)
}

// Install validates a complete ZIP in staging before replacing an installed theme.
// Installation does not activate a package and never changes built-in assets.
func Install(archive string) (Manifest, error) {
	var empty Manifest
	info, err := os.Stat(archive)
	if err != nil { return empty, err }
	if info.Size() > MaxArchiveSize { return empty, fmt.Errorf("theme ZIP exceeds 64 MiB") }
	z, err := zip.OpenReader(archive)
	if err != nil { return empty, fmt.Errorf("invalid theme ZIP: %w", err) }
	defer z.Close()
	if len(z.File) > 4096 { return empty, fmt.Errorf("theme ZIP contains too many entries") }
	mu.Lock(); defer mu.Unlock()
	if err := os.MkdirAll(Root, 0755); err != nil { return empty, err }
	staging, err := os.MkdirTemp(Root, ".install-")
	if err != nil { return empty, err }
	defer os.RemoveAll(staging)
	seen := map[string]bool{}
	var total int64
	for _, entry := range z.File {
		name := strings.TrimSuffix(entry.Name, "/")
		if !safePath(name) || entry.Mode()&os.ModeSymlink != 0 || (!entry.FileInfo().IsDir() && !entry.Mode().IsRegular()) { return empty, fmt.Errorf("unsafe ZIP entry: %s", entry.Name) }
		if name != "komari-theme.json" && name != "dist" && !strings.HasPrefix(name, "dist/") { return empty, fmt.Errorf("only komari-theme.json and dist/ are allowed") }
		key := strings.ToLower(name)
		if seen[key] { return empty, fmt.Errorf("duplicate ZIP entry: %s", name) }; seen[key] = true
		if entry.UncompressedSize64 > uint64(maxFileSize) { return empty, fmt.Errorf("theme file too large: %s", name) }
		target := filepath.Join(staging, filepath.FromSlash(name))
		if entry.FileInfo().IsDir() { if err := os.MkdirAll(target, 0755); err != nil { return empty, err }; continue }
		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil { return empty, err }
		source, err := entry.Open()
		if err != nil { return empty, err }
		out, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
		if err != nil { source.Close(); return empty, err }
		n, copyErr := io.Copy(out, io.LimitReader(source, maxFileSize+1))
		closeErr := out.Close(); sourceErr := source.Close()
		total += n
		if copyErr != nil { return empty, copyErr }
		if closeErr != nil { return empty, closeErr }; if sourceErr != nil { return empty, sourceErr }
		if n > maxFileSize || total > maxExpandedSize { return empty, fmt.Errorf("theme expanded size limit exceeded") }
	}
	data, err := readAt(staging, "komari-theme.json", maxManifestSize)
	if err != nil { return empty, err }
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil { return empty, err }
	m, err = manifestAt(staging, m.Short)
	if err != nil { return empty, err }
	target := filepath.Join(Root, m.Short)
	// Case-insensitive uniqueness keeps packages portable across Windows/Linux.
	entries, err := os.ReadDir(Root)
	if err != nil { return empty, err }
	for _, e := range entries { if strings.EqualFold(e.Name(), m.Short) && e.Name() != m.Short { return empty, fmt.Errorf("theme identifier differs only by case") } }
	backup := staging + "-previous"
	if _, err := os.Lstat(target); err == nil {
		if err := os.Rename(target, backup); err != nil { return empty, err }
	} else if !os.IsNotExist(err) { return empty, err }
	if err := os.Rename(staging, target); err != nil {
		if _, statErr := os.Lstat(backup); statErr == nil {
			if rollbackErr := os.Rename(backup, target); rollbackErr != nil { return empty, fmt.Errorf("install: %w; previous theme preserved at %s: %v", err, backup, rollbackErr) }
		}
		return empty, err
	}
	_ = os.RemoveAll(backup)
	return m, nil
}

// Settings are public frontend configuration, never a secret store. Each theme
// has its own row; default/admin settings remain owned by uisettings.
func Settings(db *gorm.DB, short string) (map[string]any, error) {
	m, err := ManifestFor(short)
	if err != nil { return nil, err }
	values := map[string]any{}
	data, _ := json.Marshal(m.Configuration.Data)
	var fields []models.ManagedThemeConfigurationItem
	if json.Unmarshal(data, &fields) == nil {
		for _, f := range fields { if f.Key != "" && f.Default != nil { values[f.Key] = f.Default } }
	}
	var row models.ThemeConfiguration
	err = db.Where("short = ?", short).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) { return values, nil }
	if err != nil { return nil, err }
	var stored map[string]any
	if json.Unmarshal([]byte(row.Data), &stored) != nil || stored == nil { return nil, fmt.Errorf("theme settings contain invalid JSON") }
	for key, value := range stored { if !strings.HasPrefix(key, "_komari_") { values[key] = value } }
	return values, nil
}

func SaveSettings(db *gorm.DB, short string, values map[string]any) error {
	mu.Lock(); defer mu.Unlock()
	if _, err := manifestAt(filepath.Join(Root, short), short); err != nil { return err }
	for key := range values { if strings.HasPrefix(key, "_komari_") { return fmt.Errorf("reserved setting key: %s", key) } }
	data, err := json.Marshal(values)
	if err != nil { return err }
	return db.Clauses(clause.OnConflict{Columns: []clause.Column{{Name:"short"}}, DoUpdates:clause.AssignmentColumns([]string{"data"})}).Create(&models.ThemeConfiguration{Short:short, Data:string(data)}).Error
}

// AssetPath maps a theme's root-relative frontend assets into its namespace.
func AssetPath(short, asset string) string { return path.Join("/themes", short, "dist", asset) }
