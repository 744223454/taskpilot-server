package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadAppliesUploadDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	data := []byte("Auth:\n  AccessSecret: test-secret\n  AccessExpire: 900\n")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	configuration, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if configuration.Upload.Root != "uploads" || configuration.Upload.MaxFileBytes != 10<<20 || configuration.Upload.MaxPages != 50 || configuration.Upload.MaxTextChars != 50000 || configuration.Upload.MinEffectiveChars != 20 {
		t.Fatalf("upload defaults = %#v", configuration.Upload)
	}
	if configuration.Upload.MaxConcurrentExtractions != 2 || configuration.Upload.ExtractTimeout != 15 || configuration.Upload.SlotWaitTimeout != 3 {
		t.Fatalf("upload execution defaults = %#v", configuration.Upload)
	}
	if configuration.Auth.LoginRateLimit != 10 || configuration.Auth.LoginRateWindow != 300 || configuration.Auth.RegisterRateLimit != 20 || configuration.Auth.RegisterRateWindow != 3600 {
		t.Fatalf("auth rate limit defaults = %#v", configuration.Auth)
	}
}

func TestLoadRejectsInvalidAuthRateLimit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	data := []byte("Auth:\n  AccessSecret: test-secret\n  AccessExpire: 900\n  LoginRateLimit: -1\n")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("Load() accepted invalid auth rate limit")
	}
}

func TestLoadRejectsInvalidUploadConcurrency(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	data := []byte("Auth:\n  AccessSecret: test-secret\n  AccessExpire: 900\nUpload:\n  MaxConcurrentExtractions: 5\n")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("Load() accepted invalid upload concurrency")
	}
}
