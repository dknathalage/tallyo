package db

import (
	"database/sql"
	"embed"
	"fmt"

	"github.com/pressly/goose/v3"
)

//go:embed migrations/control/*.sql migrations/tenant/*.sql
var migrationsFS embed.FS

const (
	controlVersionTable = "goose_db_version"     // control-plane sequence
	tenantVersionTable  = "goose_tenant_version" // per-tenant sequence (distinct so the two can coexist in one file)
)

// MigrateControl applies the control-plane migrations to the control DB.
// Idempotent. Holds tenants/users/invites/sessions/catalogue/audit.
func MigrateControl(conn *sql.DB) error {
	return migrate(conn, "migrations/control", controlVersionTable)
}

// MigrateTenant applies the per-tenant migrations to a single tenant DB.
// Idempotent — safe to call on every open (goose skips already-applied).
func MigrateTenant(conn *sql.DB) error {
	return migrate(conn, "migrations/tenant", tenantVersionTable)
}

// Migrate applies BOTH sequences to a single DB (combined single-file mode,
// used by tests and dev). The tenant audit_log uses IF NOT EXISTS so it does
// not clash with the control audit_log when both run on one file. Production
// uses MigrateControl + MigrateTenant against separate files.
func Migrate(conn *sql.DB) error {
	if err := MigrateControl(conn); err != nil {
		return err
	}
	return MigrateTenant(conn)
}

func migrate(conn *sql.DB, dir, versionTable string) error {
	if conn == nil {
		return fmt.Errorf("migrate: nil conn")
	}
	goose.SetBaseFS(migrationsFS)
	goose.SetTableName(versionTable)
	if err := goose.SetDialect("sqlite3"); err != nil {
		return fmt.Errorf("set goose dialect: %w", err)
	}
	if err := goose.Up(conn, dir); err != nil {
		return fmt.Errorf("goose up %s: %w", dir, err)
	}
	return nil
}
