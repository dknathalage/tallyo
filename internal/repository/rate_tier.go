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

// RateTier is the domain view of a row in the rate_tiers table.
type RateTier struct {
	ID          int64  `json:"id"`
	UUID        string `json:"uuid"`
	Name        string `json:"name"`
	Description string `json:"description"`
	SortOrder   int64  `json:"sortOrder"`
	CreatedAt   string `json:"createdAt"`
	UpdatedAt   string `json:"updatedAt"`
}

// RateTierInput is the writable subset of a rate tier.
type RateTierInput struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	SortOrder   int64  `json:"sortOrder"`
}

// ErrLastTier is returned when a delete would remove the only remaining tier.
var ErrLastTier = errors.New("cannot delete the last rate tier")

// RateTiersRepo reads and writes the rate_tiers table with audited mutations.
type RateTiersRepo struct {
	db *sql.DB
}

// NewRateTiers constructs a repository. A nil db is a programmer error.
func NewRateTiers(db *sql.DB) *RateTiersRepo {
	if db == nil {
		panic("repository: NewRateTiers requires a non-nil *sql.DB")
	}
	return &RateTiersRepo{db: db}
}

// List returns all rate tiers ordered by sort_order then name.
func (r *RateTiersRepo) List(ctx context.Context) ([]*RateTier, error) {
	rows, err := gen.New(r.db).ListRateTiers(ctx)
	if err != nil {
		return nil, fmt.Errorf("list rate tiers: %w", err)
	}
	out := make([]*RateTier, 0, len(rows))
	for i := range rows {
		out = append(out, toRateTier(rows[i]))
	}
	return out, nil
}

// Get returns the tier, or (nil, nil) when none matches.
func (r *RateTiersRepo) Get(ctx context.Context, id int64) (*RateTier, error) {
	row, err := gen.New(r.db).GetRateTier(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get rate tier: %w", err)
	}
	return toRateTier(row), nil
}

// GetDefault returns the lowest sort_order tier, or (nil, nil) on an empty table.
func (r *RateTiersRepo) GetDefault(ctx context.Context) (*RateTier, error) {
	row, err := gen.New(r.db).GetDefaultTier(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get default tier: %w", err)
	}
	return toRateTier(row), nil
}

// Create inserts a tier and writes one audit row, atomically.
func (r *RateTiersRepo) Create(ctx context.Context, in RateTierInput) (*RateTier, error) {
	if in.Name == "" {
		return nil, errors.New("create rate tier: name is required")
	}

	var created gen.RateTier
	err := audit.WithTx(ctx, r.db, audit.Entry{Action: ""}, func(tx *sql.Tx) error {
		now := time.Now().UTC().Format(time.RFC3339)
		t, e := gen.New(tx).CreateRateTier(ctx, gen.CreateRateTierParams{
			Uuid:        uuid.NewString(),
			Name:        in.Name,
			Description: nz(in.Description),
			SortOrder:   nzInt(in.SortOrder),
			CreatedAt:   now,
			UpdatedAt:   now,
		})
		if e != nil {
			return fmt.Errorf("insert: %w", e)
		}
		created = t
		return audit.Log(ctx, tx, audit.Entry{
			EntityType: "rate_tier",
			EntityID:   t.ID,
			Action:     "create",
			Changes:    audit.Changes(map[string]any{"name": in.Name}),
		})
	})
	if err != nil {
		return nil, fmt.Errorf("create rate tier: %w", err)
	}
	return toRateTier(created), nil
}

// Update writes the tier's fields and one audit row, atomically. Returns
// (nil, nil) when the tier does not exist so the caller can 404.
func (r *RateTiersRepo) Update(ctx context.Context, id int64, in RateTierInput) (*RateTier, error) {
	if in.Name == "" {
		return nil, errors.New("update rate tier: name is required")
	}

	var updated gen.RateTier
	var missing bool
	err := audit.WithTx(ctx, r.db, audit.Entry{
		EntityType: "rate_tier",
		EntityID:   id,
		Action:     "update",
		Changes:    audit.Changes(map[string]any{"name": in.Name}),
	}, func(tx *sql.Tx) error {
		now := time.Now().UTC().Format(time.RFC3339)
		t, e := gen.New(tx).UpdateRateTier(ctx, gen.UpdateRateTierParams{
			Name:        in.Name,
			Description: nz(in.Description),
			SortOrder:   nzInt(in.SortOrder),
			UpdatedAt:   now,
			ID:          id,
		})
		if errors.Is(e, sql.ErrNoRows) {
			missing = true
			return e
		}
		if e != nil {
			return fmt.Errorf("update: %w", e)
		}
		updated = t
		return nil
	})
	if missing {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("update rate tier: %w", err)
	}
	return toRateTier(updated), nil
}

// Delete removes a tier and writes one audit row, atomically. It refuses to
// remove the only remaining tier, returning ErrLastTier. The count, delete, and
// audit all run in one tx so the guard is race-safe.
func (r *RateTiersRepo) Delete(ctx context.Context, id int64) error {
	return audit.WithTx(ctx, r.db, audit.Entry{Action: ""}, func(tx *sql.Tx) error {
		q := gen.New(tx)
		count, e := q.CountRateTiers(ctx)
		if e != nil {
			return fmt.Errorf("count: %w", e)
		}
		if count <= 1 {
			return ErrLastTier
		}
		if e := q.DeleteRateTier(ctx, id); e != nil {
			return fmt.Errorf("delete: %w", e)
		}
		return audit.Log(ctx, tx, audit.Entry{
			EntityType: "rate_tier",
			EntityID:   id,
			Action:     "delete",
		})
	})
}

// nzInt wraps an int64 into a valid sql.NullInt64.
func nzInt(n int64) sql.NullInt64 {
	return sql.NullInt64{Int64: n, Valid: true}
}

// toRateTier maps a generated row to the domain RateTier.
func toRateTier(row gen.RateTier) *RateTier {
	return &RateTier{
		ID:          row.ID,
		UUID:        row.Uuid,
		Name:        row.Name,
		Description: row.Description.String,
		SortOrder:   row.SortOrder.Int64,
		CreatedAt:   row.CreatedAt,
		UpdatedAt:   row.UpdatedAt,
	}
}
