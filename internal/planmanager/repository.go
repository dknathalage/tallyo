// Package planmanager is the plan-manager vertical slice: domain types, the
// audited repository over the plan_managers table, the service (with SSE
// broadcast), and the HTTP handler. It depends only on platform packages
// (db/gen, audit, reqctx, realtime, httpx), never on other domain slices.
package planmanager

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/dknathalage/tallyo/internal/db"
	"time"

	"github.com/dknathalage/tallyo/internal/audit"
	"github.com/dknathalage/tallyo/internal/db/gen"
	"github.com/google/uuid"
)

// PlanManager is the domain view of a row in the plan_managers table. All
// nullable columns are unwrapped to plain strings ("" when absent).
type PlanManager struct {
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

// PlanManagerInput is the writable subset of a plan manager.
type PlanManagerInput struct {
	Name     string `json:"name"`
	Email    string `json:"email"`
	Phone    string `json:"phone"`
	Address  string `json:"address"`
	Metadata string `json:"metadata"`
}

// PlanManagersRepo reads and writes the plan_managers table (tenant-scoped) with
// audited mutations.
type PlanManagersRepo struct {
	db *sql.DB
}

// NewPlanManagers constructs a repository. A nil db is a programmer error.
func NewPlanManagers(db *sql.DB) *PlanManagersRepo {
	if db == nil {
		panic("planmanager: NewPlanManagers requires a non-nil *sql.DB")
	}
	return &PlanManagersRepo{db: db}
}

// List returns the tenant's plan managers ordered by name. When search is
// non-empty it filters to name or email matches (LIKE).
func (r *PlanManagersRepo) List(ctx context.Context, tenantID int64, search string) ([]*PlanManager, error) {
	q := gen.New(r.db)
	if search == "" {
		rows, err := q.ListPlanManagers(ctx, tenantID)
		if err != nil {
			return nil, fmt.Errorf("list plan managers: %w", err)
		}
		return mapPlanManagers(rows), nil
	}
	like := "%" + search + "%"
	rows, err := q.SearchPlanManagers(ctx, gen.SearchPlanManagersParams{
		TenantID: tenantID,
		Name:     like,
		Email:    db.Nz(like),
	})
	if err != nil {
		return nil, fmt.Errorf("search plan managers: %w", err)
	}
	return mapPlanManagers(rows), nil
}

// Get returns the tenant's plan manager, or (nil, nil) when none matches.
func (r *PlanManagersRepo) Get(ctx context.Context, tenantID, id int64) (*PlanManager, error) {
	row, err := gen.New(r.db).GetPlanManager(ctx, gen.GetPlanManagerParams{TenantID: tenantID, ID: id})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get plan manager: %w", err)
	}
	return toPlanManager(row), nil
}

// Create inserts a plan manager and writes one audit row, atomically.
func (r *PlanManagersRepo) Create(ctx context.Context, tenantID int64, in PlanManagerInput) (*PlanManager, error) {
	if tenantID == 0 {
		return nil, errors.New("create plan manager: tenant id required")
	}
	if in.Name == "" {
		return nil, errors.New("create plan manager: name is required")
	}
	metadata := in.Metadata
	if metadata == "" {
		metadata = "{}"
	}

	var created gen.PlanManager
	err := audit.WithTx(ctx, r.db, audit.Entry{Action: ""}, func(tx *sql.Tx) error {
		now := time.Now().UTC().Format(time.RFC3339)
		p, e := gen.New(tx).CreatePlanManager(ctx, gen.CreatePlanManagerParams{
			Uuid:      uuid.NewString(),
			TenantID:  tenantID,
			Name:      in.Name,
			Email:     db.Nz(in.Email),
			Phone:     db.Nz(in.Phone),
			Address:   db.Nz(in.Address),
			Metadata:  db.Nz(metadata),
			CreatedAt: now,
			UpdatedAt: now,
		})
		if e != nil {
			return fmt.Errorf("insert: %w", e)
		}
		created = p
		return audit.Log(ctx, tx, audit.Entry{
			EntityType: "plan_manager",
			EntityID:   p.ID,
			Action:     "create",
			Changes:    audit.Changes(map[string]any{"name": in.Name, "email": in.Email}),
		})
	})
	if err != nil {
		return nil, fmt.Errorf("create plan manager: %w", err)
	}
	return toPlanManager(created), nil
}

// Update writes the plan manager's fields and one audit row, atomically.
// Returns (nil, nil) when the row does not exist so the caller can 404.
func (r *PlanManagersRepo) Update(ctx context.Context, tenantID, id int64, in PlanManagerInput) (*PlanManager, error) {
	if in.Name == "" {
		return nil, errors.New("update plan manager: name is required")
	}
	metadata := in.Metadata
	if metadata == "" {
		metadata = "{}"
	}

	var updated gen.PlanManager
	var missing bool
	err := audit.WithTx(ctx, r.db, audit.Entry{
		EntityType: "plan_manager",
		EntityID:   id,
		Action:     "update",
		Changes:    audit.Changes(map[string]any{"name": in.Name}),
	}, func(tx *sql.Tx) error {
		now := time.Now().UTC().Format(time.RFC3339)
		p, e := gen.New(tx).UpdatePlanManager(ctx, gen.UpdatePlanManagerParams{
			Name:      in.Name,
			Email:     db.Nz(in.Email),
			Phone:     db.Nz(in.Phone),
			Address:   db.Nz(in.Address),
			Metadata:  db.Nz(metadata),
			UpdatedAt: now,
			TenantID:  tenantID,
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
		return nil, fmt.Errorf("update plan manager: %w", err)
	}
	return toPlanManager(updated), nil
}

// Delete removes a plan manager and writes one audit row, atomically.
func (r *PlanManagersRepo) Delete(ctx context.Context, tenantID, id int64) error {
	return audit.WithTx(ctx, r.db, audit.Entry{
		EntityType: "plan_manager",
		EntityID:   id,
		Action:     "delete",
	}, func(tx *sql.Tx) error {
		if err := gen.New(tx).DeletePlanManager(ctx, gen.DeletePlanManagerParams{TenantID: tenantID, ID: id}); err != nil {
			return fmt.Errorf("delete: %w", err)
		}
		return nil
	})
}

// BulkDelete removes several plan managers and writes one audit row, atomically.
// An empty id list is a no-op.
func (r *PlanManagersRepo) BulkDelete(ctx context.Context, tenantID int64, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	return audit.WithTx(ctx, r.db, audit.Entry{Action: ""}, func(tx *sql.Tx) error {
		q := gen.New(tx)
		for _, id := range ids { // bounded by len(ids)
			if err := q.DeletePlanManager(ctx, gen.DeletePlanManagerParams{TenantID: tenantID, ID: id}); err != nil {
				return fmt.Errorf("delete %d: %w", id, err)
			}
		}
		return audit.Log(ctx, tx, audit.Entry{
			EntityType: "plan_manager",
			EntityID:   0,
			Action:     "bulk_delete",
			Changes:    audit.Changes(map[string]any{"ids": ids}),
		})
	})
}

// mapPlanManagers converts a slice of generated rows to a non-nil domain slice.
func mapPlanManagers(rows []gen.PlanManager) []*PlanManager {
	out := make([]*PlanManager, 0, len(rows))
	for i := range rows {
		out = append(out, toPlanManager(rows[i]))
	}
	return out
}

// toPlanManager maps a generated row to the domain PlanManager.
func toPlanManager(row gen.PlanManager) *PlanManager {
	return &PlanManager{
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
