package invoice

// Status mutations and bulk/sweep paths: single-status flip, bulk delete/status,
// and the per-tenant overdue sweep. Split out of repository.go to keep that file
// to core single-entity CRUD.

import (
	"context"
	"database/sql"
	"errors"
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

// MarkOverdueForTenant flips every 'sent' invoice of one tenant whose due date
// has passed to 'overdue', auditing each, atomically. Returns the affected
// invoices. This is the per-tenant sweep path (spec §8): the caller iterates
// active tenants and skips suspended ones.
func (r *InvoicesRepo) MarkOverdueForTenant(ctx context.Context, tenantID string) ([]OverdueInvoice, error) {
	if tenantID == "" {
		return nil, errors.New("mark overdue: tenant id required")
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("mark overdue: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	q := gen.New(tx)
	rows, err := q.SelectOverdueInvoicesForTenant(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("mark overdue: select: %w", err)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	out := make([]OverdueInvoice, 0, len(rows))
	for i := range rows { // bounded by len(rows)
		if e := flipOverdue(ctx, tx, q, rows[i].TenantID, rows[i].ID, now); e != nil {
			return nil, fmt.Errorf("mark overdue: %w", e)
		}
		out = append(out, OverdueInvoice{ID: rows[i].ID, TenantID: rows[i].TenantID, Number: rows[i].Number})
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("mark overdue: commit: %w", err)
	}
	return out, nil
}

// ActiveTenantIDs returns the ids of tenants whose status is 'active' (suspended
// tenants are excluded), used by the per-tenant sweeps.
func (r *InvoicesRepo) ActiveTenantIDs(ctx context.Context) ([]string, error) {
	ids, err := gen.New(r.db).ListActiveTenantIDs(ctx)
	if err != nil {
		return nil, fmt.Errorf("active tenant ids: %w", err)
	}
	return ids, nil
}

// flipOverdue sets one invoice to overdue and logs the transition.
func flipOverdue(ctx context.Context, tx *sql.Tx, q *gen.Queries, tenantID, id string, now string) error {
	if e := q.UpdateInvoiceStatus(ctx, gen.UpdateInvoiceStatusParams{
		Status: "overdue", UpdatedAt: now, TenantID: tenantID, ID: id,
	}); e != nil {
		return e
	}
	return audit.Log(ctx, tx, audit.Entry{
		EntityType: "invoice", EntityID: id, Action: "status",
		Changes: audit.Changes(map[string]any{"from": "sent", "to": "overdue"}),
	})
}
