package server

import (
	"github.com/komari-monitor/komari/cmd/flags"
	"github.com/komari-monitor/komari/database/dbcore"
	"github.com/komari-monitor/komari/internal/config"
	"github.com/komari-monitor/komari/internal/notifications"
	"github.com/komari-monitor/komari/utils/messageSender"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestStartupLeavesLegacyExtensionsInert(t *testing.T) {
	t.Chdir(t.TempDir())
	for _, dir := range []string{"data/plugin/old", "data/theme/old", "data/plugin-data/old"} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
	}
	fixtures := map[string]string{
		"data/plugin/old/index.js":           `require('fs').writeFileSync('./executed.txt','executed');`,
		"data/plugin/state.json":             `{"old":{"enabled":true}}`,
		"data/plugin/old/komari-plugin.json": `{"name":"old","short":"old","version":"1.0.0","entry":"index.js"}`,
		"data/theme/old/index.html":          "legacy theme",
		"data/plugin-data/old/preserved.txt": "historical data",
	}
	for p, data := range fixtures {
		if err := os.WriteFile(p, []byte(data), 0644); err != nil {
			t.Fatal(err)
		}
	}
	old := flags.DatabaseFile
	flags.DatabaseFile = filepath.Join("data", "komari.db")
	t.Cleanup(func() { flags.DatabaseFile = old; _ = dbcore.Close() })
	app := New(Options{ListenAddr: "127.0.0.1:0"})
	if err := app.Bootstrap(); err != nil {
		t.Fatal(err)
	}
	defer app.Shutdown()
	if err := config.Set("notification_method", "old-plugin-channel"); err != nil {
		t.Fatal(err)
	}
	if err := notifications.Initialize(); err != nil {
		t.Fatal(err)
	}
	defer notifications.Shutdown()
	if !messageSender.NotificationChannelRegistered("webhook") {
		t.Fatal("built-in webhook missing")
	}
	if messageSender.NotificationChannelRegistered("old-plugin-channel") {
		t.Fatal("legacy channel loaded")
	}
	if err := app.BuildRouter(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat("executed.txt"); !os.IsNotExist(err) {
		t.Fatal("legacy plugin executed")
	}
	for p, want := range fixtures {
		data, err := os.ReadFile(p)
		if err != nil || string(data) != want {
			t.Fatalf("historical data changed: %s", p)
		}
	}
}

func TestFreshInstallBootstrapProcess(t *testing.T) {
	if os.Getenv("KOMARI_TEST_FRESH_BOOTSTRAP") != "1" {
		child := exec.Command(os.Args[0], "-test.run=^TestFreshInstallBootstrapProcess$")
		child.Env = append(os.Environ(), "KOMARI_TEST_FRESH_BOOTSTRAP=1")
		if output, err := child.CombinedOutput(); err != nil {
			t.Fatalf("fresh process: %v\n%s", err, output)
		}
		return
	}
	t.Chdir(t.TempDir())
	flags.DatabaseFile = "./data/komari.db"
	app := New(Options{})
	if err := app.Bootstrap(); err != nil {
		t.Fatal(err)
	}
	defer app.Shutdown()
	required, err := app.InstallRequired()
	if err != nil || !required {
		t.Fatalf("expected new-install guide: %v %v", required, err)
	}
	for _, dir := range []string{"data/theme", "data/plugin", "data/plugin-data"} {
		if _, err := os.Stat(dir); !os.IsNotExist(err) {
			t.Fatalf("retired directory created: %s", dir)
		}
	}
}
