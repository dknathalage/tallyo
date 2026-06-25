package invoice

// uuid → row-id resolvers and the per-client aggregate. These translate inbound
// public uuids to internal row ids (("", nil) on no-match for single resolvers;
// an error for the bulk/required resolvers so callers can 400). Split out of
// repository.go to keep that file to core CRUD.

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/dknathalage/tallyo/internal/db/gen"
)

// ResolveClientID resolves a client uuid to its row id (uuid), scoped to
// the tenant. Returns ("", nil) when no client matches (caller 404s).
func (r *InvoicesRepo) ResolveClientID(ctx context.Context, tenantID string, clientUUID string) (string, error) {
	id, err := gen.New(r.db).GetClientIDByUUID(ctx, gen.GetClientIDByUUIDParams{TenantID: tenantID, ID: clientUUID})
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("resolve client uuid: %w", err)
	}
	return id, nil
}

// ResolvePayerID resolves a payer uuid to its row id (uuid), scoped to
// the tenant. Returns ("", nil) when no payer matches (caller 400s).
func (r *InvoicesRepo) ResolvePayerID(ctx context.Context, tenantID string, payerUUID string) (string, error) {
	id, err := gen.New(r.db).GetPayerIDByUUID(ctx, gen.GetPayerIDByUUIDParams{TenantID: tenantID, ID: payerUUID})
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("resolve payer uuid: %w", err)
	}
	return id, nil
}

// ResolveSessionIDs resolves session uuids to row ids (uuid) (preserving order),
// tenant-scoped. An unknown uuid is an error so draft-from-sessions can 400.
func (r *InvoicesRepo) ResolveSessionIDs(ctx context.Context, tenantID string, sessionUUIDs []string) ([]string, error) {
	q := gen.New(r.db)
	out := make([]string, 0, len(sessionUUIDs))
	for i := range sessionUUIDs { // bounded by len(sessionUUIDs)
		id, err := q.GetSessionIDByUUID(ctx, gen.GetSessionIDByUUIDParams{TenantID: tenantID, ID: sessionUUIDs[i]})
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("unknown session %q", sessionUUIDs[i])
		}
		if err != nil {
			return nil, fmt.Errorf("resolve session uuid: %w", err)
		}
		out = append(out, id)
	}
	return out, nil
}

// ResolveInvoiceIDs resolves invoice uuids to row ids (uuid) (preserving
// order), tenant-scoped. An unknown uuid is an error so bulk ops can 400.
func (r *InvoicesRepo) ResolveInvoiceIDs(ctx context.Context, tenantID string, invoiceUUIDs []string) ([]string, error) {
	q := gen.New(r.db)
	out := make([]string, 0, len(invoiceUUIDs))
	for i := range invoiceUUIDs { // bounded by len(invoiceUUIDs)
		id, err := q.GetInvoiceIDByUUID(ctx, gen.GetInvoiceIDByUUIDParams{TenantID: tenantID, ID: invoiceUUIDs[i]})
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("unknown invoice %q", invoiceUUIDs[i])
		}
		if err != nil {
			return nil, fmt.Errorf("resolve invoice uuid: %w", err)
		}
		out = append(out, id)
	}
	return out, nil
}

// ClientStats returns the count and summed totals of a client's
// invoices.
func (r *InvoicesRepo) ClientStats(ctx context.Context, tenantID, clientID string) (*ClientStats, error) {
	row, err := gen.New(r.db).ClientInvoiceStats(ctx, gen.ClientInvoiceStatsParams{
		TenantID: tenantID,
		ClientID: clientID,
	})
	if err != nil {
		return nil, fmt.Errorf("client stats: %w", err)
	}
	return &ClientStats{InvoiceCount: row.InvoiceCount, TotalInvoiced: row.TotalInvoiced, TotalPaid: row.TotalPaid}, nil
}
