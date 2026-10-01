package upload

import (
	"encoding/json"
	"github.com/google/uuid"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRetiredUploadPurposesRejectedAtEveryStage(t *testing.T) {
	store := &Store{Root: t.TempDir(), MaxSize: 100}
	for _, purpose := range []Purpose{"theme", "plugin"} {
		if _, err := store.Init(purpose, "archive.zip", 1); err == nil {
			t.Fatalf("init accepted %s", purpose)
		}
		id := uuid.NewString()
		dir := filepath.Join(store.Root, id)
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		raw, _ := json.Marshal(Metadata{Purpose: purpose, Size: 1, Filename: "old.zip"})
		if err := os.WriteFile(filepath.Join(dir, "upload.json"), raw, 0600); err != nil {
			t.Fatal(err)
		}
		if err := store.SaveChunk(id, 0, strings.NewReader("x")); err == nil {
			t.Fatal("old upload accepted chunk")
		}
		if _, err := store.Merge(id); err == nil {
			t.Fatal("old upload merged")
		}
		if _, err := os.Stat(filepath.Join(dir, "archive.zip")); !os.IsNotExist(err) {
			t.Fatal("retired archive created")
		}
	}
}
func TestBackupChunkRoundTrip(t *testing.T) {
	store := &Store{Root: t.TempDir(), MaxSize: 100}
	session, err := store.Init(PurposeBackup, "backup.zip", 3)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveChunk(session.ID, 0, strings.NewReader("zip")); err != nil {
		t.Fatal(err)
	}
	merged, err := store.Merge(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(merged.ArchivePath)
	if err != nil || string(raw) != "zip" {
		t.Fatalf("backup transfer failed: %q %v", raw, err)
	}
}
