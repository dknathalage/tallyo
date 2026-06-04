package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

// DataDir resolves the application data directory.
// DATA_DIR env overrides; otherwise os.UserConfigDir()/Tallyo.
func DataDir() (string, error) {
	if override := os.Getenv("DATA_DIR"); override != "" {
		return override, nil
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve user config dir: %w", err)
	}
	return filepath.Join(base, "Tallyo"), nil
}

// Open opens a modernc SQLite connection at path and applies pragmas.
func Open(path string) (*sql.DB, error) {
	if path == "" {
		return nil, fmt.Errorf("Open: empty path")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}
	dsn := "file:" + path +
		"?_pragma=journal_mode(WAL)" +
		"&_pragma=foreign_keys(1)" +
		"&_pragma=busy_timeout(5000)"
	conn, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	// WAL permits concurrent readers with a single serialized writer; a small
	// pool suits a single-org self-hosted server. busy_timeout (per-connection,
	// via DSN above) makes writers wait rather than erroring SQLITE_BUSY.
	conn.SetMaxOpenConns(8)
	conn.SetMaxIdleConns(8)
	conn.SetConnMaxLifetime(0)
	return conn, nil
}
