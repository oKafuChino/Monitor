package admin

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBackupKeepsBothLegacyPluginDataDirectories(t *testing.T) {
	source, target := t.TempDir(), t.TempDir()
	for _, dir := range []string{"plugin", "theme", "plugin-data", "plguin-data"} {
		if err := os.MkdirAll(filepath.Join(source, dir), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(source, dir, "history.txt"), []byte(dir), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := copyWhitelistedFilesFrom(source, target); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{"plugin", "theme", "plugin-data", "plguin-data"} {
		raw, err := os.ReadFile(filepath.Join(target, dir, "history.txt"))
		if err != nil || string(raw) != dir {
			t.Fatalf("backup lost %s: %v", dir, err)
		}
	}
}
