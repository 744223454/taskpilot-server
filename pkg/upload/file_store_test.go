package upload

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"
)

func TestLocalFileStoreLifecycle(t *testing.T) {
	store, err := NewLocalFileStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewLocalFileStore() error = %v", err)
	}
	temporary, err := store.SaveTemp(context.Background(), strings.NewReader("%PDF-test"), 100)
	if err != nil {
		t.Fatalf("SaveTemp() error = %v", err)
	}
	if temporary.Size != 9 {
		t.Fatalf("temporary size = %d, want 9", temporary.Size)
	}
	finalKey, err := DocumentKey(42, time.Date(2030, time.January, 2, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("DocumentKey() error = %v", err)
	}
	if !strings.HasPrefix(finalKey, "documents/42/2030/01/") {
		t.Fatalf("final key = %q", finalKey)
	}
	if err := store.Promote(context.Background(), temporary.Key, finalKey); err != nil {
		t.Fatalf("Promote() error = %v", err)
	}
	files, err := store.List(context.Background(), "documents")
	if err != nil || len(files) != 1 || files[0].Key != finalKey {
		t.Fatalf("List() = %#v, %v", files, err)
	}
	if err := store.Delete(context.Background(), finalKey); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if err := store.Delete(context.Background(), finalKey); err != nil {
		t.Fatalf("second Delete() error = %v", err)
	}
}

func TestLocalFileStoreRejectsOversizeAndEscapingKeys(t *testing.T) {
	store, err := NewLocalFileStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewLocalFileStore() error = %v", err)
	}
	if _, err := store.SaveTemp(context.Background(), strings.NewReader("12345"), 4); !errors.Is(err, ErrFileTooLarge) {
		t.Fatalf("SaveTemp() error = %v, want ErrFileTooLarge", err)
	}
	if err := store.Delete(context.Background(), "../outside.pdf"); err == nil {
		t.Fatal("Delete() accepted escaping key")
	}
	if _, err := store.List(context.Background(), "/tmp"); err == nil {
		t.Fatal("List() accepted absolute prefix")
	}
	entries, err := os.ReadDir(store.root)
	if err != nil {
		t.Fatalf("ReadDir() error = %v", err)
	}
	if len(entries) > 1 || len(entries) == 1 && entries[0].Name() != ".tmp" {
		t.Fatalf("unexpected upload root entries = %#v", entries)
	}
}
