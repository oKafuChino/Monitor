// Package backup contains the shared upload preparation for backup restores.
package backup

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

var restoreMutex sync.Mutex

const (
	MaxArchiveSize    int64 = 4 << 30 // 4 GiB
	maxArchiveEntries       = 100_000
)

// RestoreLock serializes staging a backup until the caller releases it.
// A successful restore must hold this lock until the process restarts so a
// second request cannot replace backup.zip in the meantime.
type RestoreLock struct {
	once sync.Once
}

func AcquireRestoreLock() (*RestoreLock, error) {
	if !restoreMutex.TryLock() {
		return nil, fmt.Errorf("another restore operation is already in progress")
	}
	return &RestoreLock{}, nil
}

func (l *RestoreLock) Release() {
	l.once.Do(restoreMutex.Unlock)
}

// SaveUploadedBackup validates a Komari backup and stages it for restoration
// during the next process startup.
func SaveUploadedBackup(file io.Reader, filename string) error {
	lock, err := AcquireRestoreLock()
	if err != nil {
		return err
	}
	defer lock.Release()
	return lock.SaveUploadedBackup(file, filename)
}

// SaveUploadedBackup stages a backup while the caller holds the restore lock.
func (l *RestoreLock) SaveUploadedBackup(file io.Reader, filename string) error {
	if !strings.HasSuffix(strings.ToLower(filename), ".zip") {
		return fmt.Errorf("uploaded file must be a ZIP archive")
	}
	if err := os.MkdirAll("./data", 0755); err != nil {
		return fmt.Errorf("create data directory: %w", err)
	}

	// Stage alongside backup.zip so the final rename is atomic on every
	// supported platform and never crosses filesystem boundaries.
	tempFile, err := os.CreateTemp("./data", ".backup-upload-*.zip")
	if err != nil {
		return fmt.Errorf("create temporary backup: %w", err)
	}
	tempPath := tempFile.Name()
	defer os.Remove(tempPath)
	written, err := io.Copy(tempFile, io.LimitReader(file, MaxArchiveSize+1))
	if err != nil {
		tempFile.Close()
		return fmt.Errorf("save uploaded backup: %w", err)
	}
	if written > MaxArchiveSize {
		tempFile.Close()
		return fmt.Errorf("backup archive exceeds the %d byte limit", MaxArchiveSize)
	}
	if err := tempFile.Close(); err != nil {
		return fmt.Errorf("close uploaded backup: %w", err)
	}

	if err := ValidateArchive(tempPath); err != nil {
		return err
	}

	finalPath := filepath.Join(".", "data", "backup.zip")
	if _, err := os.Stat(finalPath); err == nil { return fmt.Errorf("a backup is already pending restoration; inspect it locally before replacing") } else if !os.IsNotExist(err) { return err }
	if err := os.Rename(tempPath, finalPath); err != nil { return fmt.Errorf("stage backup atomically: %w", err) }
	return nil
}

// ValidateArchive checks the backup marker and bounds archive expansion before
// the startup restore path extracts it into data/.
func ValidateArchive(path string) error {
	reader, err := zip.OpenReader(path)
	if err != nil {
		return fmt.Errorf("open backup archive: %w", err)
	}
	defer reader.Close()
	validationDir, err := os.MkdirTemp("", "komari-backup-preflight-*")
	if err != nil { return fmt.Errorf("create backup preflight directory: %w",err) }
	defer os.RemoveAll(validationDir)

	if len(reader.File) > maxArchiveEntries {
		return fmt.Errorf("backup archive has too many files: %d", len(reader.File))
	}

	var expandedSize uint64
	hasMarkup := false
	seen := make(map[string]struct{}, len(reader.File))
	hasDatabase := false
	for _, entry := range reader.File {
		name := strings.TrimSuffix(entry.Name, "/")
		if name == "" || strings.HasPrefix(name, "/") || strings.Contains(name, "../") || name == ".." || strings.ContainsAny(name, "\\:") {
			return fmt.Errorf("backup archive contains an unsafe path: %q", entry.Name)
		}
		clean := filepath.ToSlash(filepath.Clean(name))
		if clean != name || strings.HasPrefix(clean, "../") || clean == "." {
			return fmt.Errorf("backup archive contains an unsafe path: %q", entry.Name)
		}
		if _, ok := seen[strings.ToLower(clean)]; ok { return fmt.Errorf("backup archive contains duplicate path: %q", entry.Name) }
		seen[strings.ToLower(clean)] = struct{}{}
		root := strings.ToLower(strings.Split(clean, "/")[0])
		for _, part := range strings.Split(clean,"/") { if strings.HasSuffix(part,".") || strings.HasSuffix(part," ") || len(part)>255 { return fmt.Errorf("invalid backup path: %q",entry.Name) } }
		if root == "backup" || root == "backup.zip" || root == "setup-token" || strings.HasPrefix(root, ".restore-") { return fmt.Errorf("reserved restore path: %q", entry.Name) }
		if entry.Mode()&os.ModeSymlink != 0 || entry.Mode()&os.ModeNamedPipe != 0 || entry.Mode()&os.ModeSocket != 0 || entry.Mode()&os.ModeDevice != 0 {
			return fmt.Errorf("backup archive contains a special file: %q", entry.Name)
		}
		if entry.Name == "komari-backup-markup" {
			hasMarkup = true
		}
		if clean == "komari.db" { hasDatabase = true }
		if entry.UncompressedSize64 > uint64(MaxArchiveSize) || expandedSize > uint64(MaxArchiveSize)-entry.UncompressedSize64 {
			return fmt.Errorf("backup archive expands beyond the %d byte limit", MaxArchiveSize)
		}
		expandedSize += entry.UncompressedSize64
		if entry.FileInfo().IsDir() { continue }
		body, err := entry.Open(); if err != nil { return fmt.Errorf("read backup entry %q: %w", entry.Name, err) }
		var target io.Writer = io.Discard
		var databaseFile *os.File
		if clean=="komari.db" || clean=="metrics.db" {
			databaseFile,err = os.OpenFile(filepath.Join(validationDir,clean),os.O_CREATE|os.O_EXCL|os.O_WRONLY,0600)
			if err != nil { body.Close(); return err }; target=databaseFile
		}
		read, copyErr := io.Copy(target, io.LimitReader(body, int64(entry.UncompressedSize64)+1))
		if databaseFile!=nil { if closeErr:=databaseFile.Close(); closeErr!=nil && copyErr==nil { copyErr=closeErr } }
		closeErr := body.Close()
		if copyErr != nil { return fmt.Errorf("read backup entry %q: %w", entry.Name, copyErr) }
		if closeErr != nil { return fmt.Errorf("close backup entry %q: %w", entry.Name, closeErr) }
		if read != int64(entry.UncompressedSize64) { return fmt.Errorf("backup entry size mismatch: %q", entry.Name) }
	}
	if !hasMarkup {
		return fmt.Errorf("invalid backup file: missing komari-backup-markup file")
	}
	if !hasDatabase { return fmt.Errorf("invalid backup file: missing komari.db") }
	if err := validateDatabases(validationDir); err != nil { return err }
	return nil
}
