package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

// DataDir resolves the application data directory.
// DATA_DIR env overrides; otherwise ./data relative to the working directory.
func DataDir() (string, error) {
	if override := os.Getenv("DATA_DIR"); override != "" {
		return override, nil
	}
	return "data", nil
}

// Open opens a modernc SQLite connection at path and applies pragmas.
func Open(path string) (*sql.DB, error) {
	if path == "" {
		return nil, fmt.Errorf("Open: empty path")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}
	// _txlock=immediate makes every explicit BeginTx issue BEGIN IMMEDIATE, so
	// the write lock is grabbed at transaction start. This eliminates the
	// read->write upgrade race (SQLITE_BUSY_SNAPSHOT, which busy_timeout cannot
	// wait out) hit by the numbering create path: its MAX-read and INSERT now run
	// under a single held write lock. Autocommit reads are unaffected, so WAL
	// read concurrency is preserved; only explicit (mutating) transactions
	// serialize, which is correct for this single-org app.
	dsn := "file:" + path +
		"?_pragma=journal_mode(WAL)" +
		"&_pragma=foreign_keys(1)" +
		"&_pragma=busy_timeout(5000)" +
		"&_txlock=immediate"
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
