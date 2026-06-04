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

func TestPragmasAcrossPool(t *testing.T) {
	conn, err := Open(filepath.Join(t.TempDir(), "pool.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer conn.Close()

	for i := 0; i < 8; i++ {
		var fk int
		if err := conn.QueryRow("PRAGMA foreign_keys").Scan(&fk); err != nil {
			t.Fatalf("fk query: %v", err)
		}
		if fk != 1 {
			t.Fatalf("pooled conn foreign_keys = %d, want 1", fk)
		}
	}
}

func TestConcurrentReadWrite(t *testing.T) {
	conn, err := Open(filepath.Join(t.TempDir(), "c.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer conn.Close()
	if _, err := conn.Exec(`CREATE TABLE k (id INTEGER PRIMARY KEY, v TEXT)`); err != nil {
		t.Fatalf("create: %v", err)
	}

	const workers = 8
	errCh := make(chan error, workers)
	for i := 0; i < workers; i++ {
		go func(n int) {
			_, err := conn.Exec(`INSERT INTO k (v) VALUES (?)`, n)
			if err == nil {
				var c int
				err = conn.QueryRow(`SELECT COUNT(*) FROM k`).Scan(&c)
			}
			errCh <- err
		}(i)
	}
	for i := 0; i < workers; i++ {
		if err := <-errCh; err != nil {
			t.Fatalf("worker %d: %v", i, err)
		}
	}
	var total int
	if err := conn.QueryRow(`SELECT COUNT(*) FROM k`).Scan(&total); err != nil {
		t.Fatalf("count: %v", err)
	}
	if total != workers {
		t.Fatalf("rows = %d, want %d", total, workers)
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
