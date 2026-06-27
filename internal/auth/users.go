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

// User is the domain view of a row in the users table. The firebase_uid links
// the row to a Firebase identity but is server-side only (json:"-") so it never
// crosses the API.
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
	FirebaseUID     string `json:"-"` // Firebase identity link (never serialized)
}

// UsersRepo reads and writes the users table with audited mutations. Tenant
// scoping (spec §3.1): per-tenant reads take a tenantID; the Firebase uid links
// each row to its identity (resolved per request from the bearer token).
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

// Create inserts a user into a tenant and writes one audit row, atomically. The
// user is linked to a Firebase identity via firebaseUID (no password material).
func (r *UsersRepo) Create(ctx context.Context, tenantID string, email, firebaseUID, name, role string, isPlatformAdmin bool) (*User, error) {
	if tenantID == "" {
		return nil, errors.New("create user: tenant id required")
	}
	if email == "" {
		return nil, errors.New("create user: email is required")
	}
	if firebaseUID == "" {
		return nil, errors.New("create user: firebase uid is required")
	}

	var created gen.User
	err := audit.WithTx(ctx, r.db, audit.Entry{Action: ""}, func(tx *sql.Tx) error {
		now := time.Now().UTC().Format(time.RFC3339)
		u, e := gen.New(tx).CreateUser(ctx, gen.CreateUserParams{
			ID:              ids.New(),
			TenantID:        tenantID,
			Email:           email,
			FirebaseUid:     firebaseUID,
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

// GetByFirebaseUID returns a tenant's user by its Firebase uid (used by the
// auth-guard middleware to resolve membership from the token), or (nil, nil)
// when none matches.
func (r *UsersRepo) GetByFirebaseUID(ctx context.Context, tenantID, firebaseUID string) (*User, error) {
	row, err := gen.New(r.db).GetUserByFirebaseUID(ctx, gen.GetUserByFirebaseUIDParams{TenantID: tenantID, FirebaseUid: firebaseUID})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get user by firebase uid: %w", err)
	}
	return toUser(row), nil
}

// EmailTenant identifies one tenant in which a user is a member. Returned by
// TenantsForFirebaseUID to power the "pick tenant" UX.
//
// Public-id contract (spec: "int PK never crosses the API"): the tenant is
// identified by its uuid, serialized as "id". The int TenantID is server-side
// only (json:"-").
type EmailTenant struct {
	TenantID   string `json:"-"`  // tenant uuid (server-side, not serialized)
	TenantUUID string `json:"id"` // public tenant identifier (uuid)
	TenantName string `json:"tenantName"`
	Role       string `json:"role"`
}

// TenantsForFirebaseUID lists the tenants in which a Firebase identity is a
// member, with the per-tenant role. Powers GET /api/auth/session.
func (r *UsersRepo) TenantsForFirebaseUID(ctx context.Context, firebaseUID string) ([]EmailTenant, error) {
	rows, err := gen.New(r.db).ListTenantsByFirebaseUID(ctx, firebaseUID)
	if err != nil {
		return nil, fmt.Errorf("list tenants by firebase uid: %w", err)
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

// toUser maps a generated row to the domain User. TenantID is the tenant uuid
// (the public tenant identifier); FirebaseUID is server-side only.
func toUser(row gen.User) *User {
	return &User{
		ID:              row.ID,
		TenantID:        row.TenantID,
		Email:           row.Email,
		Name:            row.Name,
		Role:            row.Role,
		IsPlatformAdmin: row.IsPlatformAdmin == 1,
		LastLoginAt:     row.LastLoginAt.String,
		FirebaseUID:     row.FirebaseUid,
	}
}
