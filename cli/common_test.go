package cli //nolint:testpackage

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteConfigFileNew(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config.toml")
	if err := writeConfigFile(path, []byte("key = \"value\"\n")); err != nil {
		t.Fatalf("writeConfigFile() error = %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("os.ReadFile() error = %v", err)
	}
	if string(data) != "key = \"value\"\n" {
		t.Errorf("content = %q, want %q", data, "key = \"value\"\n")
	}
}

func TestWriteConfigFileOverwritesExisting(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "existing.toml")
	if err := os.WriteFile(path, []byte("old"), filePermission); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	if err := writeConfigFile(path, []byte("new")); err != nil {
		t.Fatalf("writeConfigFile() error = %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("os.ReadFile() error = %v", err)
	}
	if string(data) != "new" {
		t.Errorf("content = %q, want %q", data, "new")
	}
}

func TestWriteConfigFileClearsStaleDirectory(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "stale.toml")
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatalf("os.Mkdir() error = %v", err)
	}

	if err := writeConfigFile(path, []byte("recovered")); err != nil {
		t.Fatalf("writeConfigFile() error = %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("os.Stat() error = %v", err)
	}
	if info.IsDir() {
		t.Error("path is still a directory after writeConfigFile")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("os.ReadFile() error = %v", err)
	}
	if string(data) != "recovered" {
		t.Errorf("content = %q, want %q", data, "recovered")
	}
}

func TestWriteConfigFileLeavesNonEmptyDirectory(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "busy.toml")
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatalf("os.Mkdir() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(path, "keep.txt"), []byte("x"), filePermission); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	if err := writeConfigFile(path, []byte("data")); err == nil {
		t.Fatal("writeConfigFile() expected error for non-empty directory, got nil")
	}
}
