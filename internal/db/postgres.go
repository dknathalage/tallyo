package db

import (
	"database/sql"
	"fmt"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// Open opens a Postgres connection pool via the pgx/v5 stdlib driver.
//
// dsn is a libpq/pgx connection URL, e.g.
//
//	postgres://USER:PASSWORD@HOST:PORT/DBNAME?sslmode=disable
//
// On Cloud SQL the instance is reached over a Unix socket rather than TCP:
//
//	postgres://USER:PASSWORD@/DBNAME?host=/cloudsql/PROJECT:REGION:INSTANCE
//
// (note the empty host before the slash and the host= query parameter pointing
// at the Cloud SQL socket directory).
func Open(dsn string) (*sql.DB, error) {
	if dsn == "" {
		return nil, fmt.Errorf("Open: empty dsn")
	}
	conn, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("open postgres: %w", err)
	}
	// A small pool suits a single-org self-hosted server. Postgres handles
	// concurrent writers natively (MVCC), so unlike the old SQLite path there is
	// no single-writer serialization to design around.
	conn.SetMaxOpenConns(8)
	conn.SetMaxIdleConns(8)
	conn.SetConnMaxLifetime(30 * time.Minute)
	return conn, nil
}
