package audit

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/dknathalage/tallyo/internal/ids"
	"github.com/dknathalage/tallyo/internal/reqctx"
)

// Execer is satisfied by *sql.DB and *sql.Tx.
type Execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

type Entry struct {
	EntityType string
	EntityID   int64
	Action     string
	Changes    string // JSON; defaults to "{}"
	Context    string
}

// Log writes one audit row. Every DB mutation must call this.
//
// Every row is stamped with the acting tenant_id and user_id sourced from ctx
// (reqctx). Both are nullable: tenant_id is NULL for a mutation owned by no
// tenant (e.g. a price-list import running outside a request tenant), and
// user_id is NULL for system actions (the launch/hourly sweeps) and the
// pre-auth signup transaction. A real, authenticated mutation carries both.
func Log(ctx context.Context, db Execer, e Entry) error {
	if e.EntityType == "" {
		return fmt.Errorf("audit: empty entity_type")
	}
	if e.Action == "" {
		return fmt.Errorf("audit: empty action")
	}
	changes := e.Changes
	if changes == "" {
		changes = "{}"
	}
	tenant := nullInt64(reqctx.TenantFrom(ctx))
	user := nullInt64(reqctx.UserFrom(ctx))
	_, err := db.ExecContext(ctx,
		`INSERT INTO audit_log (uuid, tenant_id, user_id, entity_type, entity_id, action, changes, context, batch_id, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, NULL, ?)`,
		ids.New(), tenant, user, e.EntityType, e.EntityID, e.Action, changes, e.Context,
		time.Now().UTC().Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("audit insert: %w", err)
	}
	return nil
}

// nullInt64 maps a (value, present) pair from reqctx into a SQL argument: the
// raw id when present and non-zero, otherwise nil (stored as NULL).
func nullInt64(v int64, ok bool) any {
	if !ok || v == 0 {
		return nil
	}
	return v
}

// WithTx runs fn inside a transaction, writes the audit Entry in the SAME tx,
// and commits. Any error (begin, fn, audit, commit) rolls back, so the mutation
// and its audit row are atomic. This is the canonical audited-mutation helper.
//
// If Entry.Action == "", WithTx does NOT auto-log — use this when the entity id
// is generated inside fn and you log manually within fn instead.
// txBeginner is the subset of *sql.DB that WithTx needs; taking an interface
// keeps the audit helper decoupled from the concrete connection.
type txBeginner interface {
	BeginTx(context.Context, *sql.TxOptions) (*sql.Tx, error)
}

func WithTx(ctx context.Context, db txBeginner, e Entry, fn func(*sql.Tx) error) error {
	if db == nil {
		return fmt.Errorf("audit WithTx: nil db")
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("audit WithTx: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := fn(tx); err != nil {
		return err // caller's error, unwrapped so errors.Is works
	}
	if e.Action != "" {
		if err := Log(ctx, tx, e); err != nil {
			return fmt.Errorf("audit WithTx: log: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("audit WithTx: commit: %w", err)
	}
	return nil
}

// Changes marshals a map to a JSON string for Entry.Changes. Returns "{}" on
// marshal failure (audit must never break a mutation).
func Changes(m map[string]any) string {
	b, err := json.Marshal(m)
	if err != nil {
		return "{}"
	}
	return string(b)
}
