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

// User is the domain view of a row in the users table. It deliberately omits
// the password hash so callers never receive credential material.
type User struct {
	ID              int64  `json:"id"`
	UUID            string `json:"uuid"`
	TenantID        int64  `json:"tenantId"`
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
func (r *UsersRepo) Count(ctx context.Context, tenantID int64) (int64, error) {
	n, err := gen.New(r.db).CountUsers(ctx, tenantID)
	if err != nil {
		return 0, fmt.Errorf("count users: %w", err)
	}
	return n, nil
}

// Create inserts a user into a tenant and writes one audit row, atomically.
func (r *UsersRepo) Create(ctx context.Context, tenantID int64, email, hash, name, role string, isPlatformAdmin bool) (*User, error) {
	if tenantID == 0 {
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
			Uuid:            uuid.NewString(),
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
func (r *UsersRepo) GetByEmail(ctx context.Context, tenantID int64, email string) (*User, error) {
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
func (r *UsersRepo) GetByID(ctx context.Context, tenantID, id int64) (*User, error) {
	row, err := gen.New(r.db).GetUserByID(ctx, gen.GetUserByIDParams{TenantID: tenantID, ID: id})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get user by id: %w", err)
	}
	return toUser(row), nil
}

// GetCredentialsGlobal returns the id, tenant id and password hash for an email,
// resolved across all tenants for the login flow (J5). This is the only method
// that exposes the hash. found is false (with a nil error) when no user matches.
func (r *UsersRepo) GetCredentialsGlobal(ctx context.Context, email string) (id, tenantID int64, hash string, found bool, err error) {
	row, qerr := gen.New(r.db).GetUserByEmailGlobal(ctx, email)
	if errors.Is(qerr, sql.ErrNoRows) {
		return 0, 0, "", false, nil
	}
	if qerr != nil {
		return 0, 0, "", false, fmt.Errorf("get credentials: %w", qerr)
	}
	return row.ID, row.TenantID, row.PasswordHash, true, nil
}

// List returns a tenant's users ordered by id.
func (r *UsersRepo) List(ctx context.Context, tenantID int64) ([]*User, error) {
	rows, err := gen.New(r.db).ListUsers(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	out := make([]*User, 0, len(rows))
	for i := range rows {
		out = append(out, toUser(rows[i]))
	}
	return out, nil
}

// Delete removes a tenant's user and writes one audit row, atomically.
func (r *UsersRepo) Delete(ctx context.Context, tenantID, id int64) error {
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
func (r *UsersRepo) TouchLastLogin(ctx context.Context, id int64) error {
	now := time.Now().UTC().Format(time.RFC3339)
	if err := gen.New(r.db).TouchLastLogin(ctx, gen.TouchLastLoginParams{
		LastLoginAt: nz(now),
		ID:          id,
	}); err != nil {
		return fmt.Errorf("touch last login: %w", err)
	}
	return nil
}

// nz wraps a string into a valid sql.NullString.
func nz(s string) sql.NullString {
	return sql.NullString{String: s, Valid: true}
}

// bi maps a Go bool to the SQLite 0/1 integer convention.
func bi(b bool) int64 {
	if b {
		return 1
	}
	return 0
}

// toUser maps a generated row to the domain User, dropping the password hash.
func toUser(row gen.User) *User {
	return &User{
		ID:              row.ID,
		UUID:            row.Uuid,
		TenantID:        row.TenantID,
		Email:           row.Email,
		Name:            row.Name,
		Role:            row.Role,
		IsPlatformAdmin: row.IsPlatformAdmin == 1,
		LastLoginAt:     row.LastLoginAt.String,
	}
}
