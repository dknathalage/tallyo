package auth

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

// User is the domain view of a row in the users table. It deliberately omits
// the password hash so callers never receive credential material.
type User struct {
	ID          int64  `json:"id"`
	UUID        string `json:"uuid"`
	Email       string `json:"email"`
	Role        string `json:"role"`
	LastLoginAt string `json:"lastLoginAt"`
}

// UsersRepo reads and writes the users table with audited mutations.
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

// Count returns the number of users.
func (r *UsersRepo) Count(ctx context.Context) (int64, error) {
	n, err := gen.New(r.db).CountUsers(ctx)
	if err != nil {
		return 0, fmt.Errorf("count users: %w", err)
	}
	return n, nil
}

// Create inserts a user and writes one audit row, atomically.
func (r *UsersRepo) Create(ctx context.Context, email, hash, role string) (*User, error) {
	if email == "" {
		return nil, errors.New("create user: email is required")
	}
	if hash == "" {
		return nil, errors.New("create user: password hash is required")
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("create user: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	now := time.Now().UTC().Format(time.RFC3339)
	row, err := gen.New(tx).CreateUser(ctx, gen.CreateUserParams{
		Uuid:         uuid.NewString(),
		Email:        email,
		PasswordHash: hash,
		Role:         role,
		CreatedAt:    now,
		UpdatedAt:    now,
	})
	if err != nil {
		return nil, fmt.Errorf("create user: insert: %w", err)
	}

	changes, err := json.Marshal(map[string]string{"email": email, "role": role})
	if err != nil {
		return nil, fmt.Errorf("create user: marshal changes: %w", err)
	}
	if err := audit.Log(ctx, tx, audit.Entry{
		EntityType: "user",
		EntityID:   row.ID,
		Action:     "create",
		Changes:    string(changes),
	}); err != nil {
		return nil, fmt.Errorf("create user: audit: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("create user: commit: %w", err)
	}
	return toUser(row), nil
}

// GetByEmail returns the user, or (nil, nil) when none matches.
func (r *UsersRepo) GetByEmail(ctx context.Context, email string) (*User, error) {
	row, err := gen.New(r.db).GetUserByEmail(ctx, email)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get user by email: %w", err)
	}
	return toUser(row), nil
}

// GetByID returns the user, or (nil, nil) when none matches.
func (r *UsersRepo) GetByID(ctx context.Context, id int64) (*User, error) {
	row, err := gen.New(r.db).GetUserByID(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get user by id: %w", err)
	}
	return toUser(row), nil
}

// GetCredentials returns the id and password hash for an email. This is the
// only method that exposes the hash; it is used by the login flow. found is
// false (with a nil error) when no user matches the email.
func (r *UsersRepo) GetCredentials(ctx context.Context, email string) (id int64, hash string, found bool, err error) {
	row, qerr := gen.New(r.db).GetUserByEmail(ctx, email)
	if errors.Is(qerr, sql.ErrNoRows) {
		return 0, "", false, nil
	}
	if qerr != nil {
		return 0, "", false, fmt.Errorf("get credentials: %w", qerr)
	}
	return row.ID, row.PasswordHash, true, nil
}

// List returns all users ordered by id.
func (r *UsersRepo) List(ctx context.Context) ([]*User, error) {
	rows, err := gen.New(r.db).ListUsers(ctx)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	out := make([]*User, 0, len(rows))
	for i := range rows {
		out = append(out, toUser(rows[i]))
	}
	return out, nil
}

// Delete removes a user and writes one audit row, atomically.
func (r *UsersRepo) Delete(ctx context.Context, id int64) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("delete user: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if err := gen.New(tx).DeleteUser(ctx, id); err != nil {
		return fmt.Errorf("delete user: delete: %w", err)
	}
	if err := audit.Log(ctx, tx, audit.Entry{
		EntityType: "user",
		EntityID:   id,
		Action:     "delete",
	}); err != nil {
		return fmt.Errorf("delete user: audit: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("delete user: commit: %w", err)
	}
	return nil
}

// TouchLastLogin records the current time as the user's last login. Not audited.
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

// toUser maps a generated row to the domain User, dropping the password hash.
func toUser(row gen.User) *User {
	return &User{
		ID:          row.ID,
		UUID:        row.Uuid,
		Email:       row.Email,
		Role:        row.Role,
		LastLoginAt: row.LastLoginAt.String,
	}
}
