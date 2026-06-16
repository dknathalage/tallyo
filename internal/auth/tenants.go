package auth

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

// Tenant is the domain view of a row in the tenants table.
type Tenant struct {
	ID        int64  `json:"id"`
	UUID      string `json:"uuid"`
	Name      string `json:"name"`
	Status    string `json:"status"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

// TenantsRepo reads and writes the tenants table. Tenants are the top of the
// isolation hierarchy and are therefore NOT themselves tenant-scoped.
//
// TODO(J5): full signup (tenant + owner provisioning, roles, suspended guard)
// is owned by J5; this repo provides the minimal Create/Count that first-run
// setup needs to compile and function.
type TenantsRepo struct {
	db *sql.DB
}

// NewTenants constructs a repository. A nil db is a programmer error.
func NewTenants(db *sql.DB) *TenantsRepo {
	if db == nil {
		panic("auth: NewTenants requires a non-nil *sql.DB")
	}
	return &TenantsRepo{db: db}
}

// Count returns the total number of tenants. Used by first-run setup to decide
// whether an owner/tenant already exists.
func (r *TenantsRepo) Count(ctx context.Context) (int64, error) {
	rows, err := gen.New(r.db).ListTenants(ctx)
	if err != nil {
		return 0, fmt.Errorf("count tenants: %w", err)
	}
	return int64(len(rows)), nil
}

// Create inserts a tenant (status "active") and writes one audit row, atomically.
func (r *TenantsRepo) Create(ctx context.Context, name string) (*Tenant, error) {
	if name == "" {
		return nil, errors.New("create tenant: name is required")
	}
	var created gen.Tenant
	err := audit.WithTx(ctx, r.db, audit.Entry{Action: ""}, func(tx *sql.Tx) error {
		now := time.Now().UTC().Format(time.RFC3339)
		t, e := gen.New(tx).CreateTenant(ctx, gen.CreateTenantParams{
			Uuid:      uuid.NewString(),
			Name:      name,
			Status:    "active",
			CreatedAt: now,
			UpdatedAt: now,
		})
		if e != nil {
			return fmt.Errorf("insert: %w", e)
		}
		created = t
		return audit.Log(ctx, tx, audit.Entry{
			EntityType: "tenant",
			EntityID:   t.ID,
			Action:     "create",
			Changes:    audit.Changes(map[string]any{"name": name}),
		})
	})
	if err != nil {
		return nil, fmt.Errorf("create tenant: %w", err)
	}
	return &Tenant{
		ID:        created.ID,
		UUID:      created.Uuid,
		Name:      created.Name,
		Status:    created.Status,
		CreatedAt: created.CreatedAt,
		UpdatedAt: created.UpdatedAt,
	}, nil
}
