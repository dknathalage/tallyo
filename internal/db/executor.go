package db

import (
	"context"
	"database/sql"
)

// Executor is the database surface a repository needs: the sqlc DBTX methods
// (ExecContext/PrepareContext/QueryContext/QueryRowContext) plus BeginTx for
// audited transactions. Satisfied by *sql.DB (and *sql.Tx for Exec), so repos
// stay decoupled from the concrete connection.
type Executor interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	PrepareContext(context.Context, string) (*sql.Stmt, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
	BeginTx(context.Context, *sql.TxOptions) (*sql.Tx, error)
}
