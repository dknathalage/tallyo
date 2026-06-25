package estimate

// uuid → row-id resolvers: translate inbound public uuids to internal row ids
// (("", nil) on no-match for the single resolvers; an error for the bulk
// resolver so callers can 400). Split out of repository.go to keep that file to
// core CRUD.

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/dknathalage/tallyo/internal/db/gen"
)

// ResolveEstimateID resolves an estimate uuid to its row id (uuid), scoped to the
// tenant. Returns ("", nil) when no estimate matches the uuid (caller 404s).
func (r *EstimatesRepo) ResolveEstimateID(ctx context.Context, tenantID string, estimateUUID string) (string, error) {
	id, err := gen.New(r.db).GetEstimateIDByUUID(ctx, gen.GetEstimateIDByUUIDParams{TenantID: tenantID, ID: estimateUUID})
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("resolve estimate uuid: %w", err)
	}
	return id, nil
}

// ResolveEstimateIDs resolves estimate uuids to their row ids (uuid) (preserving
// order), tenant-scoped. An unknown uuid is an error so bulk ops can 400.
func (r *EstimatesRepo) ResolveEstimateIDs(ctx context.Context, tenantID string, estimateUUIDs []string) ([]string, error) {
	q := gen.New(r.db)
	out := make([]string, 0, len(estimateUUIDs))
	for i := range estimateUUIDs { // bounded by len(estimateUUIDs)
		id, err := q.GetEstimateIDByUUID(ctx, gen.GetEstimateIDByUUIDParams{TenantID: tenantID, ID: estimateUUIDs[i]})
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("unknown estimate %q", estimateUUIDs[i])
		}
		if err != nil {
			return nil, fmt.Errorf("resolve estimate uuid: %w", err)
		}
		out = append(out, id)
	}
	return out, nil
}

// ResolveClientID resolves a client uuid to its row id (uuid), scoped to
// the tenant. Returns ("", nil) when no client matches (caller 400s).
func (r *EstimatesRepo) ResolveClientID(ctx context.Context, tenantID string, clientUUID string) (string, error) {
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
func (r *EstimatesRepo) ResolvePayerID(ctx context.Context, tenantID string, payerUUID string) (string, error) {
	id, err := gen.New(r.db).GetPayerIDByUUID(ctx, gen.GetPayerIDByUUIDParams{TenantID: tenantID, ID: payerUUID})
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("resolve payer uuid: %w", err)
	}
	return id, nil
}
