package tenantdb

import (
	"context"
	"database/sql"
	"fmt"
)

// Conn is a per-request routing handle: it implements db.Executor (the sqlc
// DBTX methods + BeginTx) by resolving the request's tenant from the ctx passed
// into each call and delegating to that tenant's *sql.DB. One Conn is shared by
// all tenant-plane repositories; it carries no tenant of its own — the ctx does.
type Conn struct{ reg *Registry }

// Tenant returns the shared routing handle for tenant-plane repositories.
func (r *Registry) Tenant() *Conn { return &Conn{reg: r} }

func (c *Conn) resolve(ctx context.Context) (*sql.DB, error) { return c.reg.ForTenant(ctx) }

// ExecContext routes to the request tenant's DB.
func (c *Conn) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	db, err := c.resolve(ctx)
	if err != nil {
		return nil, err
	}
	return db.ExecContext(ctx, query, args...)
}

// QueryContext routes to the request tenant's DB.
func (c *Conn) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	db, err := c.resolve(ctx)
	if err != nil {
		return nil, err
	}
	return db.QueryContext(ctx, query, args...)
}

// PrepareContext routes to the request tenant's DB.
func (c *Conn) PrepareContext(ctx context.Context, query string) (*sql.Stmt, error) {
	db, err := c.resolve(ctx)
	if err != nil {
		return nil, err
	}
	return db.PrepareContext(ctx, query)
}

// QueryRowContext routes to the request tenant's DB. *sql.Row carries no error
// channel, so a resolve failure (no tenant in ctx — a programmer error behind
// the auth middleware) panics, consistent with reqctx.MustTenant.
func (c *Conn) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	db, err := c.resolve(ctx)
	if err != nil {
		panic(fmt.Sprintf("tenantdb.Conn.QueryRowContext: %v", err))
	}
	return db.QueryRowContext(ctx, query, args...)
}

// BeginTx routes to the request tenant's DB (used by audit.WithTx).
func (c *Conn) BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error) {
	db, err := c.resolve(ctx)
	if err != nil {
		return nil, err
	}
	return db.BeginTx(ctx, opts)
}
