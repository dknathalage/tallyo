package audit

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/dknathalage/tallyo/internal/db/gen"
)

// Record is the domain view of an audit_log row for read paths (e.g. the
// platform-admin tenant-detail trail). Nullable columns are flattened to plain
// strings ("" when NULL) so callers and JSON consumers do not deal with
// sql.NullString.
type Record struct {
	ID         string `json:"id"`
	TenantID   string `json:"tenantId,omitempty"`
	UserID     string `json:"userId,omitempty"`
	EntityType string `json:"entityType"`
	EntityID   string `json:"entityId,omitempty"`
	Action     string `json:"action"`
	Changes    string `json:"changes,omitempty"`
	Context    string `json:"context,omitempty"`
	BatchID    string `json:"batchId,omitempty"`
	CreatedAt  string `json:"createdAt"`
}

// Reader reads audit rows from the control DB. Audit writes go through the
// package-level Log/LogAs helpers; Reader owns the (rarer) read side.
type Reader struct {
	db *sql.DB
}

// NewReader constructs an audit Reader. A nil db is a programmer error.
func NewReader(db *sql.DB) *Reader {
	if db == nil {
		panic("audit: NewReader requires a non-nil *sql.DB")
	}
	return &Reader{db: db}
}

// ListByTenant returns the most recent audit rows for a tenant (newest first,
// capped at 50). Used by the platform-admin tenant-detail trail.
func (r *Reader) ListByTenant(ctx context.Context, tenantID string) ([]Record, error) {
	if tenantID == "" {
		return nil, fmt.Errorf("audit list: tenant id required")
	}
	rows, err := gen.New(r.db).ListAuditByTenant(ctx, sql.NullString{String: tenantID, Valid: true})
	if err != nil {
		return nil, fmt.Errorf("audit list by tenant: %w", err)
	}
	out := make([]Record, 0, len(rows))
	for i := range rows {
		out = append(out, recordFromRow(rows[i]))
	}
	return out, nil
}

// recordFromRow maps a generated audit_log row to the domain Record.
func recordFromRow(row gen.AuditLog) Record {
	return Record{
		ID:         row.ID,
		TenantID:   row.TenantID.String,
		UserID:     row.UserID.String,
		EntityType: row.EntityType,
		EntityID:   row.EntityID.String,
		Action:     row.Action,
		Changes:    row.Changes.String,
		Context:    row.Context.String,
		BatchID:    row.BatchID.String,
		CreatedAt:  row.CreatedAt,
	}
}
