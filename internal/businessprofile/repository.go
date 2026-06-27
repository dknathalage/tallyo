// Package businessprofile is the business-profile vertical slice: domain types,
// the audited repository over the business_profile table, the service (with SSE
// broadcast), and the HTTP handler. It depends only on platform packages
// (db/gen, audit, reqctx, httpx), never on other domain slices.
package businessprofile

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/dknathalage/tallyo/internal/db"
	"time"

	"github.com/dknathalage/tallyo/internal/apperr"
	"github.com/dknathalage/tallyo/internal/audit"
	"github.com/dknathalage/tallyo/internal/db/gen"
	"github.com/dknathalage/tallyo/internal/ids"
)

// BusinessProfile is the domain view of the per-tenant business profile row.
// All nullable columns are unwrapped to plain strings ("" when absent).
type BusinessProfile struct {
	Name            string `json:"name"`
	Email           string `json:"email"`
	Phone           string `json:"phone"`
	Address         string `json:"address"`
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
	Logo            string `json:"logo"`
	Metadata        string `json:"metadata"`
	DefaultCurrency string `json:"defaultCurrency"`
}

// Validate checks the cheap required-field rules the service enforces before the
// repository runs. A failure is returned as an *apperr.ValidationError so the
// HTTP layer responds 422 with per-field detail.
func (in BusinessProfileInput) Validate() error {
	ve := &apperr.ValidationError{}
	if in.Name == "" {
		ve.Errors = append(ve.Errors, apperr.FieldError{Line: 0, Field: "name", Message: "required"})
	}
	if len(ve.Errors) > 0 {
		return ve
	}
	return nil
}

// BusinessProfileRepo reads and writes the per-tenant business profile (1:1).
type BusinessProfileRepo struct {
	db db.Executor
}

// NewBusinessProfile constructs a repository. A nil db is a programmer error.
func NewBusinessProfile(db db.Executor) *BusinessProfileRepo {
	if db == nil {
		panic("businessprofile: NewBusinessProfile requires a non-nil *sql.DB")
	}
	return &BusinessProfileRepo{db: db}
}

// Get returns the tenant's profile, or (nil, nil) when none exists yet.
func (r *BusinessProfileRepo) Get(ctx context.Context, tenantID string) (*BusinessProfile, error) {
	if tenantID == "" {
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
		Logo:            row.Logo.String,
		Metadata:        row.Metadata.String,
		DefaultCurrency: row.DefaultCurrency.String,
	}, nil
}

// Save upserts the tenant's profile and writes one audit row, atomically.
func (r *BusinessProfileRepo) Save(ctx context.Context, tenantID string, in BusinessProfileInput) error {
	if tenantID == "" {
		return errors.New("save business profile: tenant id required")
	}

	return audit.WithTx(ctx, r.db, audit.Entry{
		EntityType: "business_profile",
		EntityID:   tenantID,
		Action:     "update",
		Changes:    audit.Changes(map[string]any{"name": in.Name}),
	}, func(tx *sql.Tx) error {
		id, err := existingUUID(ctx, tx, tenantID)
		if err != nil {
			return fmt.Errorf("read uuid: %w", err)
		}
		if err := gen.New(tx).UpsertBusinessProfile(ctx, buildParams(tenantID, id, in)); err != nil {
			return fmt.Errorf("upsert: %w", err)
		}
		return nil
	})
}

// existingUUID returns the tenant's current profile uuid, or a freshly generated
// one when no row exists yet. Read inside the tx so the upsert preserves it.
func existingUUID(ctx context.Context, tx *sql.Tx, tenantID string) (string, error) {
	var id string
	err := tx.QueryRowContext(ctx, "SELECT id FROM business_profile WHERE tenant_id = ?", tenantID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return ids.New(), nil
	}
	if err != nil {
		return "", err
	}
	return id, nil
}

// buildParams maps input + defaults into the generated upsert params.
func buildParams(tenantID string, id string, in BusinessProfileInput) gen.UpsertBusinessProfileParams {
	currency := in.DefaultCurrency
	if currency == "" {
		currency = "AUD"
	}
	metadata := in.Metadata
	if metadata == "" {
		metadata = "{}"
	}
	now := time.Now().UTC().Format(time.RFC3339)
	return gen.UpsertBusinessProfileParams{
		TenantID:        tenantID,
		ID:              id,
		Name:            in.Name,
		Email:           db.Nz(in.Email),
		Phone:           db.Nz(in.Phone),
		Address:         db.Nz(in.Address),
		Logo:            db.Nz(in.Logo),
		Metadata:        db.Nz(metadata),
		DefaultCurrency: db.Nz(currency),
		CreatedAt:       now,
		UpdatedAt:       now,
	}
}
