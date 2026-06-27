package invoice

// Status mutations and bulk paths: single-status flip and bulk delete/status.
// Split out of repository.go to keep that file to core single-entity CRUD.

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/dknathalage/tallyo/internal/audit"
	"github.com/dknathalage/tallyo/internal/db/gen"
)

// UpdateStatus sets just the status column, atomically with one audit row.
func (r *InvoicesRepo) UpdateStatus(ctx context.Context, tenantID, id string, status string) error {
	return audit.WithTx(ctx, r.db, audit.Entry{
		EntityType: "invoice", EntityID: id, Action: "status",
		Changes: audit.Changes(map[string]any{"status": status}),
	}, func(tx *sql.Tx) error {
		now := time.Now().UTC().Format(time.RFC3339)
		if e := gen.New(tx).UpdateInvoiceStatus(ctx, gen.UpdateInvoiceStatusParams{
			Status: status, UpdatedAt: now, TenantID: tenantID, ID: id,
		}); e != nil {
			return fmt.Errorf("update status: %w", e)
		}
		return nil
	})
}

// BulkDelete removes several invoices and writes one audit row. Empty is a no-op.
func (r *InvoicesRepo) BulkDelete(ctx context.Context, tenantID string, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	return audit.WithTx(ctx, r.db, audit.Entry{Action: ""}, func(tx *sql.Tx) error {
		q := gen.New(tx)
		for _, id := range ids { // bounded by len(ids)
			if e := q.UnlinkSessionItemsFromInvoice(ctx, gen.UnlinkSessionItemsFromInvoiceParams{
				TenantID: tenantID, InvoiceID: sql.NullString{String: id, Valid: true},
			}); e != nil {
				return fmt.Errorf("unlink session items %s: %w", id, e)
			}
			if e := q.DeleteInvoice(ctx, gen.DeleteInvoiceParams{TenantID: tenantID, ID: id}); e != nil {
				return fmt.Errorf("delete %s: %w", id, e)
			}
		}
		return audit.Log(ctx, tx, audit.Entry{
			EntityType: "invoice", EntityID: "", Action: "bulk_delete",
			Changes: audit.Changes(map[string]any{"ids": ids}),
		})
	})
}

// BulkUpdateStatus sets the status of several invoices and writes one audit row.
func (r *InvoicesRepo) BulkUpdateStatus(ctx context.Context, tenantID string, ids []string, status string) error {
	if len(ids) == 0 {
		return nil
	}
	return audit.WithTx(ctx, r.db, audit.Entry{Action: ""}, func(tx *sql.Tx) error {
		q := gen.New(tx)
		now := time.Now().UTC().Format(time.RFC3339)
		for _, id := range ids { // bounded by len(ids)
			if e := q.UpdateInvoiceStatus(ctx, gen.UpdateInvoiceStatusParams{
				Status: status, UpdatedAt: now, TenantID: tenantID, ID: id,
			}); e != nil {
				return fmt.Errorf("status %s: %w", id, e)
			}
		}
		return audit.Log(ctx, tx, audit.Entry{
			EntityType: "invoice", EntityID: "", Action: "bulk_status",
			Changes: audit.Changes(map[string]any{"ids": ids, "status": status}),
		})
	})
}
