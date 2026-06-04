package db

import (
	"path/filepath"
	"testing"
)

func TestOpenAppliesPragmas(t *testing.T) {
	dir := t.TempDir()
	conn, err := Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer conn.Close()

	var fk int
	if err := conn.QueryRow("PRAGMA foreign_keys").Scan(&fk); err != nil {
		t.Fatalf("query foreign_keys: %v", err)
	}
	if fk != 1 {
		t.Fatalf("foreign_keys = %d, want 1", fk)
	}

	var mode string
	if err := conn.QueryRow("PRAGMA journal_mode").Scan(&mode); err != nil {
		t.Fatalf("query journal_mode: %v", err)
	}
	if mode != "wal" {
		t.Fatalf("journal_mode = %q, want wal", mode)
	}
}

func TestDataDirHonorsEnvOverride(t *testing.T) {
	t.Setenv("DATA_DIR", "/tmp/custom-tallyo")
	got, err := DataDir()
	if err != nil {
		t.Fatalf("DataDir: %v", err)
	}
	if got != "/tmp/custom-tallyo" {
		t.Fatalf("DataDir = %q, want /tmp/custom-tallyo", got)
	}
}
