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

// Tenant status values (spec §3.1). A suspended tenant blocks login for all of
// its users.
const (
	StatusActive    = "active"
	StatusSuspended = "suspended"
)

// TenantsRepo reads and writes the tenants table. Tenants are the top of the
// isolation hierarchy and are therefore NOT themselves tenant-scoped.
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

// Status returns a tenant's status string. Returns ("", false, nil) when no such
// tenant exists. Used by the login + auth-guard suspended-tenant check.
func (r *TenantsRepo) Status(ctx context.Context, tenantID int64) (status string, found bool, err error) {
	if tenantID == 0 {
		return "", false, errors.New("tenant status: tenant id required")
	}
	row, qerr := gen.New(r.db).GetTenant(ctx, tenantID)
	if errors.Is(qerr, sql.ErrNoRows) {
		return "", false, nil
	}
	if qerr != nil {
		return "", false, fmt.Errorf("tenant status: %w", qerr)
	}
	return row.Status, true, nil
}

// GetByUUID resolves a tenant by its public UUID. Returns (nil, nil) when no
// tenant has that uuid (caller → 404). Used by the URL-tenant middleware.
func (r *TenantsRepo) GetByUUID(ctx context.Context, tenantUUID string) (*Tenant, error) {
	if tenantUUID == "" {
		return nil, errors.New("tenant by uuid: uuid required")
	}
	row, err := gen.New(r.db).GetTenantByUUID(ctx, tenantUUID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("tenant by uuid: %w", err)
	}
	return &Tenant{
		ID:        row.ID,
		UUID:      row.Uuid,
		Name:      row.Name,
		Status:    row.Status,
		CreatedAt: row.CreatedAt,
		UpdatedAt: row.UpdatedAt,
	}, nil
}

// SignupInput carries the validated fields for self-serve onboarding. Validation
// (non-empty business name, valid email, password strength, allowed zone) is the
// caller's (HTTP boundary) responsibility; this method assumes pre-validated
// input and performs the all-or-nothing provisioning.
type SignupInput struct {
	BusinessName string
	Email        string
	PasswordHash string
	OwnerName    string
	Zone         string
}

// Signup provisions a brand-new tenant in ONE transaction: the tenant (status
// active), its owner user (role "owner"), and the tenant's business_profile
// (carrying the geographic zone). Any failure rolls back the whole transaction,
// so a half-provisioned tenant can never exist (spec §3.2). Returns the created
// owner user (without the password hash).
func (r *TenantsRepo) Signup(ctx context.Context, in SignupInput) (*User, error) {
	if in.BusinessName == "" || in.Email == "" || in.PasswordHash == "" {
		return nil, errors.New("signup: business name, email and password hash are required")
	}
	zone := in.Zone
	if zone == "" {
		zone = "national"
	}
	var owner gen.User
	err := audit.WithTx(ctx, r.db, audit.Entry{Action: ""}, func(tx *sql.Tx) error {
		q := gen.New(tx)
		now := time.Now().UTC().Format(time.RFC3339)
		t, e := q.CreateTenant(ctx, gen.CreateTenantParams{
			Uuid:      uuid.NewString(),
			Name:      in.BusinessName,
			Status:    StatusActive,
			CreatedAt: now,
			UpdatedAt: now,
		})
		if e != nil {
			return fmt.Errorf("create tenant: %w", e)
		}
		u, e := q.CreateUser(ctx, gen.CreateUserParams{
			Uuid:            uuid.NewString(),
			TenantID:        t.ID,
			Email:           in.Email,
			PasswordHash:    in.PasswordHash,
			Name:            in.OwnerName,
			IsPlatformAdmin: 0,
			Role:            "owner",
			CreatedAt:       now,
			UpdatedAt:       now,
		})
		if e != nil {
			return fmt.Errorf("create owner: %w", e)
		}
		owner = u
		if e := q.UpsertBusinessProfile(ctx, gen.UpsertBusinessProfileParams{
			TenantID:        t.ID,
			Uuid:            uuid.NewString(),
			Name:            in.BusinessName,
			Email:           sql.NullString{String: in.Email, Valid: true},
			Zone:            zone,
			Metadata:        sql.NullString{String: "{}", Valid: true},
			DefaultCurrency: sql.NullString{String: "AUD", Valid: true},
			CreatedAt:       now,
			UpdatedAt:       now,
		}); e != nil {
			return fmt.Errorf("create business profile: %w", e)
		}
		return audit.Log(ctx, tx, audit.Entry{
			EntityType: "tenant",
			EntityID:   t.ID,
			Action:     "signup",
			Changes:    audit.Changes(map[string]any{"name": in.BusinessName, "ownerEmail": in.Email, "zone": zone}),
		})
	})
	if err != nil {
		return nil, fmt.Errorf("signup: %w", err)
	}
	return toUser(owner), nil
}
