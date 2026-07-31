package upload

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCleanupHonorsReferencesAndGracePeriods(t *testing.T) {
	store, err := NewLocalFileStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewLocalFileStore() error = %v", err)
	}
	now := time.Now()
	oldTemp := saveTestFile(t, store, ".tmp/old.pdf", now.Add(-2*time.Hour))
	saveTestFile(t, store, ".tmp/fresh.pdf", now.Add(-30*time.Minute))
	oldOrphan := saveTestFile(t, store, "documents/1/2030/01/orphan.pdf", now.Add(-48*time.Hour))
	referenced := saveTestFile(t, store, "documents/1/2030/01/referenced.pdf", now.Add(-48*time.Hour))
	saveTestFile(t, store, "documents/1/2030/01/fresh.pdf", now.Add(-12*time.Hour))

	stats, err := Cleanup(context.Background(), store, map[string]struct{}{referenced.Key: {}}, now, time.Hour, 24*time.Hour)
	if err != nil {
		t.Fatalf("Cleanup() error = %v", err)
	}
	if stats.Scanned != 5 || stats.Deleted != 2 || stats.Skipped != 3 || stats.Failed != 0 {
		t.Fatalf("Cleanup() stats = %#v", stats)
	}
	if _, err := os.Stat(oldTemp.LocalPath); !os.IsNotExist(err) {
		t.Fatalf("old temporary file still exists, error = %v", err)
	}
	if _, err := os.Stat(oldOrphan.LocalPath); !os.IsNotExist(err) {
		t.Fatalf("old orphan file still exists, error = %v", err)
	}
	if _, err := os.Stat(referenced.LocalPath); err != nil {
		t.Fatalf("referenced file was deleted: %v", err)
	}
}

func saveTestFile(t *testing.T, store *LocalFileStore, key string, modTime time.Time) File {
	t.Helper()
	path, err := store.resolve(key)
	if err != nil {
		t.Fatalf("resolve(%q) error = %v", key, err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(path, []byte(strings.Repeat("x", 8)), 0o640); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if err := os.Chtimes(path, modTime, modTime); err != nil {
		t.Fatalf("Chtimes() error = %v", err)
	}
	return File{Key: key, LocalPath: path, Size: 8, ModTime: modTime}
}
