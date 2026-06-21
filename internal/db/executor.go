package db

import (
	"context"
	"database/sql"
)

// Executor is the database surface a repository needs: the sqlc DBTX methods
// (ExecContext/PrepareContext/QueryContext/QueryRowContext) plus BeginTx for
// audited transactions. Both *sql.DB and the per-tenant routing handle
// (tenantdb.Conn) satisfy it, so a repository written against Executor works
// unchanged in single-DB (tests) and per-tenant (production) modes.
type Executor interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	PrepareContext(context.Context, string) (*sql.Stmt, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
	BeginTx(context.Context, *sql.TxOptions) (*sql.Tx, error)
}
