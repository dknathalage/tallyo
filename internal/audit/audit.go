package audit

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
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
	BatchID    string
}

// Log writes one audit row. Every DB mutation must call this.
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
	var batch any
	if e.BatchID != "" {
		batch = e.BatchID
	}
	_, err := db.ExecContext(ctx,
		`INSERT INTO audit_log (uuid, entity_type, entity_id, action, changes, context, batch_id, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		uuid.NewString(), e.EntityType, e.EntityID, e.Action, changes, e.Context, batch,
		time.Now().UTC().Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("audit insert: %w", err)
	}
	return nil
}

// WithTx runs fn inside a transaction, writes the audit Entry in the SAME tx,
// and commits. Any error (begin, fn, audit, commit) rolls back, so the mutation
// and its audit row are atomic. This is the canonical audited-mutation helper.
//
// If Entry.Action == "", WithTx does NOT auto-log — use this when the entity id
// is generated inside fn and you log manually within fn instead.
func WithTx(ctx context.Context, db *sql.DB, e Entry, fn func(*sql.Tx) error) error {
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
