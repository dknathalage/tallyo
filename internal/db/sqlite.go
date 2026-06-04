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
	conn, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	conn.SetMaxOpenConns(1)
	for _, pragma := range []string{
		"PRAGMA journal_mode = WAL",
		"PRAGMA foreign_keys = ON",
		"PRAGMA busy_timeout = 5000",
	} {
		if _, err := conn.Exec(pragma); err != nil {
			conn.Close()
			return nil, fmt.Errorf("apply %q: %w", pragma, err)
		}
	}
	return conn, nil
}
