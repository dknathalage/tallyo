// Package repository provides the data-access layer over the sqlc-generated
// queries. Services use repositories; they never touch internal/db/gen directly.
//
// Tenant scoping is the data-layer half of multi-tenant isolation (spec §3.1):
// every method on a tenant-owned repository takes a tenantID and passes it into
// the generated query, which filters WHERE tenant_id = ?. The global NDIS
// catalogue repositories (CatalogRepo) are NOT tenant-scoped.
package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/dknathalage/tallyo/internal/audit"
	"github.com/dknathalage/tallyo/internal/db/gen"
	"github.com/google/uuid"
)

// BusinessProfile is the domain view of the per-tenant business profile row.
// All nullable columns are unwrapped to plain strings ("" when absent). Zone is
// the tenant's NDIS pricing zone (national | remote | very_remote).
type BusinessProfile struct {
	Name            string `json:"name"`
	Email           string `json:"email"`
	Phone           string `json:"phone"`
	Address         string `json:"address"`
	Zone            string `json:"zone"`
	Logo            string `json:"logo"`
	Metadata        string `json:"metadata"`
	DefaultCurrency string `json:"defaultCurrency"`
}

// BusinessProfileInput carries the writable fields for a save.
type BusinessProfileInput struct {
	Name            string `json:"name"`
	Email           string `json:"email"`
	Phone           string `json:"phone"`
	Address         string `json:"address"`
	Zone            string `json:"zone"`
	Logo            string `json:"logo"`
	Metadata        string `json:"metadata"`
	DefaultCurrency string `json:"defaultCurrency"`
}

// BusinessProfileRepo reads and writes the per-tenant business profile (1:1).
type BusinessProfileRepo struct {
	db *sql.DB
}

// NewBusinessProfile constructs a repository. A nil db is a programmer error.
func NewBusinessProfile(db *sql.DB) *BusinessProfileRepo {
	if db == nil {
		panic("repository: NewBusinessProfile requires a non-nil *sql.DB")
	}
	return &BusinessProfileRepo{db: db}
}

// Get returns the tenant's profile, or (nil, nil) when none exists yet.
func (r *BusinessProfileRepo) Get(ctx context.Context, tenantID int64) (*BusinessProfile, error) {
	if tenantID == 0 {
		return nil, errors.New("get business profile: tenant id required")
	}
	row, err := gen.New(r.db).GetBusinessProfile(ctx, tenantID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get business profile: %w", err)
	}
	return &BusinessProfile{
		Name:            row.Name,
		Email:           row.Email.String,
		Phone:           row.Phone.String,
		Address:         row.Address.String,
		Zone:            row.Zone,
		Logo:            row.Logo.String,
		Metadata:        row.Metadata.String,
		DefaultCurrency: row.DefaultCurrency.String,
	}, nil
}

// Save upserts the tenant's profile and writes one audit row, atomically.
func (r *BusinessProfileRepo) Save(ctx context.Context, tenantID int64, in BusinessProfileInput) error {
	if tenantID == 0 {
		return errors.New("save business profile: tenant id required")
	}
	if in.Name == "" {
		return errors.New("save business profile: name is required")
	}

	return audit.WithTx(ctx, r.db, audit.Entry{
		EntityType: "business_profile",
		EntityID:   tenantID,
		Action:     "update",
		Changes:    audit.Changes(map[string]any{"name": in.Name}),
	}, func(tx *sql.Tx) error {
		id, err := existingUuid(ctx, tx, tenantID)
		if err != nil {
			return fmt.Errorf("read uuid: %w", err)
		}
		if err := gen.New(tx).UpsertBusinessProfile(ctx, buildParams(tenantID, id, in)); err != nil {
			return fmt.Errorf("upsert: %w", err)
		}
		return nil
	})
}

// existingUuid returns the tenant's current profile uuid, or a freshly generated
// one when no row exists yet. Read inside the tx so the upsert preserves it.
func existingUuid(ctx context.Context, tx *sql.Tx, tenantID int64) (string, error) {
	var id string
	err := tx.QueryRowContext(ctx, "SELECT uuid FROM business_profile WHERE tenant_id = ?", tenantID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return uuid.NewString(), nil
	}
	if err != nil {
		return "", err
	}
	return id, nil
}

// buildParams maps input + defaults into the generated upsert params.
func buildParams(tenantID int64, id string, in BusinessProfileInput) gen.UpsertBusinessProfileParams {
	currency := in.DefaultCurrency
	if currency == "" {
		currency = "AUD"
	}
	zone := in.Zone
	if zone == "" {
		zone = "national"
	}
	metadata := in.Metadata
	if metadata == "" {
		metadata = "{}"
	}
	now := time.Now().UTC().Format(time.RFC3339)
	return gen.UpsertBusinessProfileParams{
		TenantID:        tenantID,
		Uuid:            id,
		Name:            in.Name,
		Email:           nz(in.Email),
		Phone:           nz(in.Phone),
		Address:         nz(in.Address),
		Zone:            zone,
		Logo:            nz(in.Logo),
		Metadata:        nz(metadata),
		DefaultCurrency: nz(currency),
		CreatedAt:       now,
		UpdatedAt:       now,
	}
}

// nz wraps a string into a valid sql.NullString.
func nz(s string) sql.NullString {
	return sql.NullString{String: s, Valid: true}
}
