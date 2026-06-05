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

// Payer is the domain view of a row in the payers table. All nullable columns
// are unwrapped to plain strings ("" when absent).
type Payer struct {
	ID        int64  `json:"id"`
	UUID      string `json:"uuid"`
	Name      string `json:"name"`
	Email     string `json:"email"`
	Phone     string `json:"phone"`
	Address   string `json:"address"`
	Metadata  string `json:"metadata"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

// PayerInput is the writable subset of a payer.
type PayerInput struct {
	Name     string `json:"name"`
	Email    string `json:"email"`
	Phone    string `json:"phone"`
	Address  string `json:"address"`
	Metadata string `json:"metadata"`
}

// PayersRepo reads and writes the payers table with audited mutations.
type PayersRepo struct {
	db *sql.DB
}

// NewPayers constructs a repository. A nil db is a programmer error.
func NewPayers(db *sql.DB) *PayersRepo {
	if db == nil {
		panic("repository: NewPayers requires a non-nil *sql.DB")
	}
	return &PayersRepo{db: db}
}

// List returns all payers ordered by name. When search is non-empty it filters
// to payers whose name or email matches the term (case-insensitive LIKE).
func (r *PayersRepo) List(ctx context.Context, search string) ([]*Payer, error) {
	q := gen.New(r.db)
	if search == "" {
		rows, err := q.ListPayers(ctx)
		if err != nil {
			return nil, fmt.Errorf("list payers: %w", err)
		}
		return mapPayers(rows), nil
	}
	like := "%" + search + "%"
	rows, err := q.SearchPayers(ctx, gen.SearchPayersParams{Name: like, Email: nz(like)})
	if err != nil {
		return nil, fmt.Errorf("search payers: %w", err)
	}
	return mapPayers(rows), nil
}

// Get returns the payer, or (nil, nil) when none matches.
func (r *PayersRepo) Get(ctx context.Context, id int64) (*Payer, error) {
	row, err := gen.New(r.db).GetPayer(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get payer: %w", err)
	}
	return toPayer(row), nil
}

// Create inserts a payer and writes one audit row, atomically.
func (r *PayersRepo) Create(ctx context.Context, in PayerInput) (*Payer, error) {
	if in.Name == "" {
		return nil, errors.New("create payer: name is required")
	}
	metadata := in.Metadata
	if metadata == "" {
		metadata = "{}"
	}

	var created gen.Payer
	err := audit.WithTx(ctx, r.db, audit.Entry{Action: ""}, func(tx *sql.Tx) error {
		now := time.Now().UTC().Format(time.RFC3339)
		p, e := gen.New(tx).CreatePayer(ctx, gen.CreatePayerParams{
			Uuid:      uuid.NewString(),
			Name:      in.Name,
			Email:     nz(in.Email),
			Phone:     nz(in.Phone),
			Address:   nz(in.Address),
			Metadata:  nz(metadata),
			CreatedAt: now,
			UpdatedAt: now,
		})
		if e != nil {
			return fmt.Errorf("insert: %w", e)
		}
		created = p
		return audit.Log(ctx, tx, audit.Entry{
			EntityType: "payer",
			EntityID:   p.ID,
			Action:     "create",
			Changes:    audit.Changes(map[string]any{"name": in.Name, "email": in.Email}),
		})
	})
	if err != nil {
		return nil, fmt.Errorf("create payer: %w", err)
	}
	return toPayer(created), nil
}

// Update writes the payer's fields and one audit row, atomically. Returns
// (nil, nil) when the payer does not exist so the caller can 404.
func (r *PayersRepo) Update(ctx context.Context, id int64, in PayerInput) (*Payer, error) {
	if in.Name == "" {
		return nil, errors.New("update payer: name is required")
	}
	metadata := in.Metadata
	if metadata == "" {
		metadata = "{}"
	}

	var updated gen.Payer
	var missing bool
	err := audit.WithTx(ctx, r.db, audit.Entry{
		EntityType: "payer",
		EntityID:   id,
		Action:     "update",
		Changes:    audit.Changes(map[string]any{"name": in.Name}),
	}, func(tx *sql.Tx) error {
		now := time.Now().UTC().Format(time.RFC3339)
		p, e := gen.New(tx).UpdatePayer(ctx, gen.UpdatePayerParams{
			Name:      in.Name,
			Email:     nz(in.Email),
			Phone:     nz(in.Phone),
			Address:   nz(in.Address),
			Metadata:  nz(metadata),
			UpdatedAt: now,
			ID:        id,
		})
		if errors.Is(e, sql.ErrNoRows) {
			missing = true
			return e
		}
		if e != nil {
			return fmt.Errorf("update: %w", e)
		}
		updated = p
		return nil
	})
	if missing {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("update payer: %w", err)
	}
	return toPayer(updated), nil
}

// Delete removes a payer and writes one audit row, atomically.
func (r *PayersRepo) Delete(ctx context.Context, id int64) error {
	return audit.WithTx(ctx, r.db, audit.Entry{
		EntityType: "payer",
		EntityID:   id,
		Action:     "delete",
	}, func(tx *sql.Tx) error {
		if err := gen.New(tx).DeletePayer(ctx, id); err != nil {
			return fmt.Errorf("delete: %w", err)
		}
		return nil
	})
}

// BulkDelete removes several payers and writes one audit row, atomically. An
// empty id list is a no-op.
func (r *PayersRepo) BulkDelete(ctx context.Context, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	return audit.WithTx(ctx, r.db, audit.Entry{Action: ""}, func(tx *sql.Tx) error {
		q := gen.New(tx)
		for _, id := range ids { // bounded by len(ids)
			if err := q.DeletePayer(ctx, id); err != nil {
				return fmt.Errorf("delete %d: %w", id, err)
			}
		}
		return audit.Log(ctx, tx, audit.Entry{
			EntityType: "payer",
			EntityID:   0,
			Action:     "bulk_delete",
			Changes:    audit.Changes(map[string]any{"ids": ids}),
		})
	})
}

// mapPayers converts a slice of generated rows to a non-nil domain slice.
func mapPayers(rows []gen.Payer) []*Payer {
	out := make([]*Payer, 0, len(rows))
	for i := range rows {
		out = append(out, toPayer(rows[i]))
	}
	return out
}

// toPayer maps a generated row to the domain Payer, unwrapping NullStrings.
func toPayer(row gen.Payer) *Payer {
	return &Payer{
		ID:        row.ID,
		UUID:      row.Uuid,
		Name:      row.Name,
		Email:     row.Email.String,
		Phone:     row.Phone.String,
		Address:   row.Address.String,
		Metadata:  row.Metadata.String,
		CreatedAt: row.CreatedAt,
		UpdatedAt: row.UpdatedAt,
	}
}
