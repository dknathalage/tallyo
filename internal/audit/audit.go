package audit

import (
	"context"
	"database/sql"
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
