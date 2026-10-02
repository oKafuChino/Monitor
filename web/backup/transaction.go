package backup

import (
    "archive/zip"
    "context"
    "database/sql"
    "encoding/json"
    "fmt"
    "io"
    "net/url"
    "os"
    "path/filepath"
    "strings"
    "time"

    _ "github.com/mattn/go-sqlite3"
)

// RestoreTransaction keeps the original files until database startup and
// capability revocation succeed. The journal also handles interrupted starts.
type RestoreTransaction struct {
    Old []string `json:"old"`
    New []string `json:"new"`
    Committed bool `json:"committed"`
    ArchiveDir string `json:"archive_dir"`
}

const restoreJournal = "./data/.restore-journal.json"
const restoreStage = "./data/.restore-stage"
const restoreOriginal = "./data/.restore-original"

func (t *RestoreTransaction) journal() error {
    body, err := json.Marshal(t); if err != nil { return err }
    f, err := os.OpenFile(restoreJournal+".tmp", os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600); if err != nil { return err }
    _, err = f.Write(body); if err == nil { err = f.Sync() }; closeErr := f.Close()
    if err != nil { return err }; if closeErr != nil { return closeErr }
    return os.Rename(restoreJournal+".tmp", restoreJournal)
}

func safeTop(name string) bool { return name != "" && name != "." && name != ".." && !strings.ContainsAny(name, "/\\:") }
// Treat unreadable paths as present so permission/I/O errors cannot skip
// validation or turn a failed restore into a silently empty installation.
func exists(path string) bool { _, err := os.Lstat(path); return err == nil || !os.IsNotExist(err) }

func (t *RestoreTransaction) Rollback() error {
    if !exists(filepath.Join(restoreStage,"komari.db")) && exists(filepath.Join("data","komari.db")) {
        for _, suffix := range []string{"-wal","-shm","-journal"} {
            source := filepath.Join("data","komari.db"+suffix)
            if exists(source) { if err := os.Rename(source, filepath.Join(restoreStage,"failed-komari.db"+suffix)); err != nil { return err } }
        }
    }
    // Moving the new files back before the originals makes rollback itself
    // restartable without deleting either version.
    for _, name := range t.New {
        if !safeTop(name) { return fmt.Errorf("unsafe restore journal") }
        source, dest := filepath.Join("data", name), filepath.Join(restoreStage, name)
        if !exists(dest) && exists(source) { if err := os.Rename(source, dest); err != nil { return err } }
    }
    for _, name := range t.Old {
        if !safeTop(name) { return fmt.Errorf("unsafe restore journal") }
        source, dest := filepath.Join(restoreOriginal, name), filepath.Join("data", name)
        if exists(source) { if err := os.Rename(source, dest); err != nil { return err } }
    }
    if err := os.Remove(restoreJournal); err != nil && !os.IsNotExist(err) { return err }
    if err := os.Remove(restoreOriginal); err != nil && !os.IsNotExist(err) { return err }
    return nil
}

func (t *RestoreTransaction) Commit() error {
    t.Committed = true
    if err := t.journal(); err != nil { return err }
    return t.finishCommit()
}

func (t *RestoreTransaction) finishCommit() error {
    if !safeTop(t.ArchiveDir) { return fmt.Errorf("unsafe restore journal archive") }
    if err := os.MkdirAll("./data/backup", 0700); err != nil { return err }
    dest := filepath.Join("data", "backup", t.ArchiveDir)
    if exists(restoreOriginal) { if err := os.Rename(restoreOriginal, dest); err != nil { return err } }
    if exists("./data/backup.zip") { if err := os.Rename("./data/backup.zip", filepath.Join(dest, "restored.zip")); err != nil { return err } }
    if err := os.RemoveAll(restoreStage); err != nil { return err }
    return os.Remove(restoreJournal)
}

// BeginRestore performs complete streaming preflight before moving any live
// files. It intentionally refuses custom main database locations rather than
// silently restoring a different database; export/import remains available
// after using the documented default path.
func CheckRestoreTarget(databaseFile string) error {
    if databaseFile == "" { databaseFile = "./data/komari.db" }
    dbPath, err := filepath.Abs(databaseFile); if err != nil { return err }
    standard, err := filepath.Abs("./data/komari.db"); if err != nil { return err }
    if dbPath != standard { return fmt.Errorf("restore requires main database at ./data/komari.db; original data retained") }
    return nil
}

func BeginRestore(databaseFile string) (*RestoreTransaction, error) {
    if exists(restoreJournal) {
        body, err := os.ReadFile(restoreJournal); if err != nil { return nil, err }
        var pending RestoreTransaction
        if err := json.Unmarshal(body, &pending); err != nil { return nil, err }
        if pending.Committed { if err := pending.finishCommit(); err != nil { return nil, err } } else {
            if err := pending.Rollback(); err != nil { return nil, fmt.Errorf("recover interrupted restore: %w", err) }
            return nil, fmt.Errorf("interrupted restore rolled back; backup.zip retained; inspect it before retrying")
        }
    }
    if !exists("./data/backup.zip") { return nil, nil }
    if err := CheckRestoreTarget(databaseFile); err != nil { return nil,err }
    if err := ValidateArchive("./data/backup.zip"); err != nil { return nil, err }
    if err := os.RemoveAll(restoreStage); err != nil { return nil, err }
    if err := os.Mkdir(restoreStage, 0700); err != nil { return nil, err }
    reader, err := zip.OpenReader("./data/backup.zip"); if err != nil { return nil, err }
    defer reader.Close()
    var actual int64
    for _, entry := range reader.File {
        path := filepath.Join(restoreStage, filepath.FromSlash(entry.Name))
        if entry.FileInfo().IsDir() { if err := os.MkdirAll(path, 0700); err != nil { return nil, err }; continue }
        if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil { return nil, err }
        in, err := entry.Open(); if err != nil { return nil, err }
        out, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600); if err != nil { in.Close(); return nil, err }
        n, copyErr := io.Copy(out, io.LimitReader(in, MaxArchiveSize-actual+1)); actual += n
        inErr, outErr := in.Close(), out.Close()
        if copyErr != nil { return nil, copyErr }; if inErr != nil { return nil, inErr }; if outErr != nil { return nil, outErr }
        if actual > MaxArchiveSize || n != int64(entry.UncompressedSize64) { return nil, fmt.Errorf("backup expansion limit or size mismatch") }
    }
    if err := validateDatabases(restoreStage); err != nil { return nil,err }
    _ = os.Remove(filepath.Join(restoreStage, "komari-backup-markup"))
    if exists(restoreOriginal) { return nil, fmt.Errorf("previous restore originals require inspection at %s", restoreOriginal) }
    if err := os.Mkdir(restoreOriginal, 0700); err != nil { return nil, err }
    transaction := &RestoreTransaction{ArchiveDir:"pre-restore-"+time.Now().UTC().Format("20060102-150405.000000000")}
    old, err := os.ReadDir("./data"); if err != nil { return nil, err }
    for _, entry := range old { name := entry.Name(); if name == "backup" || name == "backup.zip" || strings.HasPrefix(name,".restore-") { continue }; transaction.Old = append(transaction.Old,name) }
    fresh, err := os.ReadDir(restoreStage); if err != nil { return nil, err }
    for _, entry := range fresh { transaction.New = append(transaction.New,entry.Name()) }
    if err := transaction.journal(); err != nil { return nil, err }
    for _, name := range transaction.Old { if err := os.Rename(filepath.Join("data",name),filepath.Join(restoreOriginal,name)); err != nil { rollbackErr := transaction.Rollback(); return nil, fmt.Errorf("restore switch: %v; rollback: %v",err,rollbackErr) } }
    for _, name := range transaction.New { if err := os.Rename(filepath.Join(restoreStage,name),filepath.Join("data",name)); err != nil { rollbackErr := transaction.Rollback(); return nil, fmt.Errorf("restore switch: %v; rollback: %v",err,rollbackErr) } }
    return transaction,nil
}

func validateDatabases(directory string) error {
    for _, name := range []string{"komari.db", "metrics.db"} {
        path := filepath.Join(directory, name)
        if !exists(path) { continue }
        absolute, err := filepath.Abs(path); if err != nil { return  err }
        dsn := (&url.URL{Scheme:"file", Path:filepath.ToSlash(absolute)}).String()+"?mode=ro&_query_only=1"
        db, err := sql.Open("sqlite3", dsn); if err != nil { return  err }
        ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
        var result string
        err = db.QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&result)
        if err == nil && result != "ok" { err = fmt.Errorf("integrity check failed") }
        if err == nil && name == "komari.db" { var tables int; err = db.QueryRowContext(ctx, "SELECT count(*) FROM sqlite_master WHERE type='table' AND name='users'").Scan(&tables); if err == nil && tables != 1 { err = fmt.Errorf("missing account schema") } }
        cancel(); closeErr := db.Close()
        if err != nil { return  fmt.Errorf("invalid backup database %s: %w", name, err) }; if closeErr != nil { return  closeErr }
    }
    return nil
}
