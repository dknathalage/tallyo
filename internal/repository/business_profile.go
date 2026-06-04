// Package repository provides the data-access layer over the sqlc-generated
// queries. Services use repositories; they never touch internal/db/gen directly.
package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/dknathalage/tallyo/internal/audit"
	"github.com/dknathalage/tallyo/internal/db/gen"
	"github.com/google/uuid"
)

// BusinessProfile is the domain view of the singleton business profile row.
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

// BusinessProfileRepo reads and writes the singleton business profile (id=1).
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

// Get returns the singleton profile, or (nil, nil) when none exists yet.
func (r *BusinessProfileRepo) Get(ctx context.Context) (*BusinessProfile, error) {
	row, err := gen.New(r.db).GetBusinessProfile(ctx)
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

// Save upserts the singleton profile and writes one audit row, atomically.
func (r *BusinessProfileRepo) Save(ctx context.Context, in BusinessProfileInput) error {
	if in.Name == "" {
		return errors.New("save business profile: name is required")
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("save business profile: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	id, err := existingUuid(ctx, tx)
	if err != nil {
		return fmt.Errorf("save business profile: read uuid: %w", err)
	}

	if err := gen.New(tx).UpsertBusinessProfile(ctx, buildParams(id, in)); err != nil {
		return fmt.Errorf("save business profile: upsert: %w", err)
	}

	changes, err := json.Marshal(map[string]string{"name": in.Name})
	if err != nil {
		return fmt.Errorf("save business profile: marshal changes: %w", err)
	}
	auditErr := audit.Log(ctx, tx, audit.Entry{
		EntityType: "business_profile",
		EntityID:   1,
		Action:     "update",
		Changes:    string(changes),
	})
	if auditErr != nil {
		return fmt.Errorf("save business profile: audit: %w", auditErr)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("save business profile: commit: %w", err)
	}
	return nil
}

// existingUuid returns the current profile uuid, or a freshly generated one
// when no row exists yet. Read inside the tx so the upsert preserves it.
func existingUuid(ctx context.Context, tx *sql.Tx) (string, error) {
	var id string
	err := tx.QueryRowContext(ctx, "SELECT uuid FROM business_profile WHERE id = 1").Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return uuid.NewString(), nil
	}
	if err != nil {
		return "", err
	}
	return id, nil
}

// buildParams maps input + defaults into the generated upsert params.
func buildParams(id string, in BusinessProfileInput) gen.UpsertBusinessProfileParams {
	currency := in.DefaultCurrency
	if currency == "" {
		currency = "USD"
	}
	metadata := in.Metadata
	if metadata == "" {
		metadata = "{}"
	}
	now := time.Now().UTC().Format(time.RFC3339)
	return gen.UpsertBusinessProfileParams{
		Uuid:            id,
		Name:            in.Name,
		Email:           nz(in.Email),
		Phone:           nz(in.Phone),
		Address:         nz(in.Address),
		Logo:            nz(in.Logo),
		Metadata:        nz(metadata),
		DefaultCurrency: nz(currency),
		CreatedAt:       nz(now),
		UpdatedAt:       nz(now),
	}
}

// nz wraps a string into a valid sql.NullString.
func nz(s string) sql.NullString {
	return sql.NullString{String: s, Valid: true}
}
