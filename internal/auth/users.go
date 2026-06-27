package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/dknathalage/tallyo/internal/db"
	"time"

	"github.com/dknathalage/tallyo/internal/audit"
	"github.com/dknathalage/tallyo/internal/db/gen"
	"github.com/dknathalage/tallyo/internal/ids"
)

// ErrAmbiguousEmail is returned by GetCredentialsGlobal when an email is
// registered in more than one tenant. Login must then require a tenant selector
// rather than authenticating into an arbitrary tenant (fail safe, spec §3.1).
var ErrAmbiguousEmail = errors.New("email registered in multiple tenants")

// User is the domain view of a row in the users table. It deliberately omits
// the password hash so callers never receive credential material.
//
// Public-id contract (spec: "int PK never crosses the API"): the serialized
// identifier is the user's uuid (json:"id") and the tenant is identified by the
// tenant uuid (json:"tenantId"). The int PKs (ID, TenantID) are server-side only
// and tagged json:"-".
type User struct {
	ID              string `json:"id"`       // public identifier (user uuid)
	TenantID        string `json:"tenantId"` // tenant uuid
	Email           string `json:"email"`
	Name            string `json:"name"`
	Role            string `json:"role"`
	IsPlatformAdmin bool   `json:"isPlatformAdmin"`
	LastLoginAt     string `json:"lastLoginAt"`
}

// UsersRepo reads and writes the users table with audited mutations. Tenant
// scoping (spec §3.1): per-tenant reads take a tenantID; the global, pre-tenant
// login lookup uses GetByEmailGlobal.
type UsersRepo struct {
	db *sql.DB
}

// NewUsers constructs a repository. A nil db is a programmer error.
func NewUsers(db *sql.DB) *UsersRepo {
	if db == nil {
		panic("auth: NewUsers requires a non-nil *sql.DB")
	}
	return &UsersRepo{db: db}
}

// Count returns the number of users in a tenant.
func (r *UsersRepo) Count(ctx context.Context, tenantID string) (int64, error) {
	n, err := gen.New(r.db).CountUsers(ctx, tenantID)
	if err != nil {
		return 0, fmt.Errorf("count users: %w", err)
	}
	return n, nil
}

// Create inserts a user into a tenant and writes one audit row, atomically.
func (r *UsersRepo) Create(ctx context.Context, tenantID string, email, hash, name, role string, isPlatformAdmin bool) (*User, error) {
	if tenantID == "" {
		return nil, errors.New("create user: tenant id required")
	}
	if email == "" {
		return nil, errors.New("create user: email is required")
	}
	if hash == "" {
		return nil, errors.New("create user: password hash is required")
	}

	var created gen.User
	err := audit.WithTx(ctx, r.db, audit.Entry{Action: ""}, func(tx *sql.Tx) error {
		now := time.Now().UTC().Format(time.RFC3339)
		u, e := gen.New(tx).CreateUser(ctx, gen.CreateUserParams{
			ID:              ids.New(),
			TenantID:        tenantID,
			Email:           email,
			PasswordHash:    hash,
			Name:            name,
			IsPlatformAdmin: bi(isPlatformAdmin),
			Role:            role,
			CreatedAt:       now,
			UpdatedAt:       now,
		})
		if e != nil {
			return fmt.Errorf("insert: %w", e)
		}
		created = u
		return audit.Log(ctx, tx, audit.Entry{
			EntityType: "user",
			EntityID:   u.ID,
			Action:     "create",
			Changes:    audit.Changes(map[string]any{"email": email, "role": role}),
		})
	})
	if err != nil {
		return nil, fmt.Errorf("create user: %w", err)
	}
	return toUser(created), nil
}

// GetByEmail returns a tenant's user by email, or (nil, nil) when none matches.
func (r *UsersRepo) GetByEmail(ctx context.Context, tenantID string, email string) (*User, error) {
	row, err := gen.New(r.db).GetUserByEmail(ctx, gen.GetUserByEmailParams{TenantID: tenantID, Email: email})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get user by email: %w", err)
	}
	return toUser(row), nil
}

// GetByEmailGlobal returns the user with the given email regardless of tenant,
// for the pre-tenant login flow (J5). Returns (nil, nil) when none matches.
func (r *UsersRepo) GetByEmailGlobal(ctx context.Context, email string) (*User, error) {
	row, err := gen.New(r.db).GetUserByEmailGlobal(ctx, email)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get user by email global: %w", err)
	}
	return toUser(row), nil
}

// GetByID returns a tenant's user by id (used by the auth-guard middleware), or
// (nil, nil) when none matches.
func (r *UsersRepo) GetByID(ctx context.Context, tenantID, id string) (*User, error) {
	row, err := gen.New(r.db).GetUserByID(ctx, gen.GetUserByIDParams{TenantID: tenantID, ID: id})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get user by id: %w", err)
	}
	return toUser(row), nil
}

// Credentials carries the fields needed to authenticate and establish a session.
// It is the only return shape that exposes the password hash; callers must never
// surface Hash to clients.
type Credentials struct {
	ID       string
	TenantID string
	Hash     string
}

// CountByEmailGlobal returns how many users across all tenants share an email.
// Login uses this to detect the AMBIGUOUS case (count > 1) and fail safe rather
// than authenticating into an arbitrary tenant.
func (r *UsersRepo) CountByEmailGlobal(ctx context.Context, email string) (int64, error) {
	n, err := gen.New(r.db).CountUsersByEmailGlobal(ctx, email)
	if err != nil {
		return 0, fmt.Errorf("count users by email: %w", err)
	}
	return n, nil
}

// EmailTenant identifies one tenant in which an email is registered. Returned by
// TenantsForEmail so an ambiguous login can prompt the user to choose a tenant.
//
// Public-id contract (spec: "int PK never crosses the API"): the tenant is
// identified by its uuid, serialized as "id". The int TenantID is server-side
// only (json:"-"); it stays on the struct because the login flow uses it to
// scope the credential lookup.
type EmailTenant struct {
	TenantID   string `json:"-"`  // tenant uuid (used by login, not serialized)
	TenantUUID string `json:"id"` // public tenant identifier (uuid)
	TenantName string `json:"tenantName"`
	Role       string `json:"role"`
}

// TenantsForEmail lists the tenants in which an email is registered, for the
// tenant-disambiguation step of login.
func (r *UsersRepo) TenantsForEmail(ctx context.Context, email string) ([]EmailTenant, error) {
	rows, err := gen.New(r.db).ListTenantsByEmail(ctx, email)
	if err != nil {
		return nil, fmt.Errorf("list tenants by email: %w", err)
	}
	out := make([]EmailTenant, 0, len(rows))
	for i := range rows {
		out = append(out, EmailTenant{
			TenantID:   rows[i].TenantID,
			TenantName: rows[i].TenantName,
			TenantUUID: rows[i].TenantUuid,
			Role:       rows[i].Role,
		})
	}
	return out, nil
}

// GetCredentialsGlobal returns the credentials for an email when EXACTLY ONE
// user across all tenants has it. found is false (nil error) when no user
// matches. When more than one tenant shares the email it returns ErrAmbiguous so
// the caller can fail safe instead of picking an arbitrary tenant.
func (r *UsersRepo) GetCredentialsGlobal(ctx context.Context, email string) (creds Credentials, found bool, err error) {
	n, err := r.CountByEmailGlobal(ctx, email)
	if err != nil {
		return Credentials{}, false, err
	}
	if n == 0 {
		return Credentials{}, false, nil
	}
	if n > 1 {
		return Credentials{}, false, ErrAmbiguousEmail
	}
	row, qerr := gen.New(r.db).GetUserByEmailGlobal(ctx, email)
	if errors.Is(qerr, sql.ErrNoRows) {
		return Credentials{}, false, nil
	}
	if qerr != nil {
		return Credentials{}, false, fmt.Errorf("get credentials: %w", qerr)
	}
	return Credentials{ID: row.ID, TenantID: row.TenantID, Hash: row.PasswordHash}, true, nil
}

// GetCredentialsForTenant returns the credentials for an (email, tenant) pair.
// Used when the login request names a tenant (disambiguation). found is false
// (nil error) when no such user exists.
func (r *UsersRepo) GetCredentialsForTenant(ctx context.Context, tenantID string, email string) (creds Credentials, found bool, err error) {
	if tenantID == "" {
		return Credentials{}, false, errors.New("get credentials for tenant: tenant id required")
	}
	row, qerr := gen.New(r.db).GetUserByEmail(ctx, gen.GetUserByEmailParams{TenantID: tenantID, Email: email})
	if errors.Is(qerr, sql.ErrNoRows) {
		return Credentials{}, false, nil
	}
	if qerr != nil {
		return Credentials{}, false, fmt.Errorf("get credentials for tenant: %w", qerr)
	}
	return Credentials{ID: row.ID, TenantID: row.TenantID, Hash: row.PasswordHash}, true, nil
}

// List returns a tenant's users ordered by id. Every row shares the tenant uuid,
// so it is resolved once and stamped onto each User (public tenant identifier).
func (r *UsersRepo) List(ctx context.Context, tenantID string) ([]*User, error) {
	rows, err := gen.New(r.db).ListUsers(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	out := make([]*User, 0, len(rows))
	for i := range rows { // bounded by len(rows)
		out = append(out, toUser(rows[i]))
	}
	return out, nil
}

// Delete removes a tenant's user and writes one audit row, atomically.
func (r *UsersRepo) Delete(ctx context.Context, tenantID, id string) error {
	return audit.WithTx(ctx, r.db, audit.Entry{
		EntityType: "user",
		EntityID:   id,
		Action:     "delete",
	}, func(tx *sql.Tx) error {
		if err := gen.New(tx).DeleteUser(ctx, gen.DeleteUserParams{TenantID: tenantID, ID: id}); err != nil {
			return fmt.Errorf("delete: %w", err)
		}
		return nil
	})
}

// TouchLastLogin records the current time as the user's last login. Not audited.
// Keyed by user id only (the user is already authenticated at this point).
func (r *UsersRepo) TouchLastLogin(ctx context.Context, id string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	if err := gen.New(r.db).TouchLastLogin(ctx, gen.TouchLastLoginParams{
		LastLoginAt: db.Nz(now),
		ID:          id,
	}); err != nil {
		return fmt.Errorf("touch last login: %w", err)
	}
	return nil
}

// bi maps a Go bool to the 0/1 integer convention used for INTEGER bool columns.
func bi(b bool) int64 {
	if b {
		return 1
	}
	return 0
}

// toUser maps a generated row to the domain User, dropping the password hash.
// TenantID is the tenant uuid (the public tenant identifier).
func toUser(row gen.User) *User {
	return &User{
		ID:              row.ID,
		TenantID:        row.TenantID,
		Email:           row.Email,
		Name:            row.Name,
		Role:            row.Role,
		IsPlatformAdmin: row.IsPlatformAdmin == 1,
		LastLoginAt:     row.LastLoginAt.String,
	}
}
