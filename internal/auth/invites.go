package auth

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/dknathalage/tallyo/internal/audit"
	"github.com/dknathalage/tallyo/internal/db/gen"
	"github.com/google/uuid"
)

// ErrInviteInvalid is returned by Validate when an invite is unknown, expired,
// or already accepted. Callers should treat all three identically.
var ErrInviteInvalid = errors.New("invite invalid or expired")

// ErrEmailTaken is returned by Accept when a user with the invite's email
// already exists.
var ErrEmailTaken = errors.New("email already registered")

// Invite is the domain view of a row in the invites table. Invites are
// tenant-scoped: an owner/admin invites users into their own tenant (spec §3.2).
type Invite struct {
	ID        int64  `json:"id"`
	TenantID  int64  `json:"tenantId"`
	Token     string `json:"token"`
	Email     string `json:"email"`
	Role      string `json:"role"`
	ExpiresAt string `json:"expiresAt"`
	Accepted  bool   `json:"accepted"`
}

// InvitesRepo reads and writes the invites table with audited mutations.
type InvitesRepo struct {
	db *sql.DB
}

// NewInvites constructs a repository. A nil db is a programmer error.
func NewInvites(db *sql.DB) *InvitesRepo {
	if db == nil {
		panic("auth: NewInvites requires a non-nil *sql.DB")
	}
	return &InvitesRepo{db: db}
}

// newToken returns a URL-safe random invite token. A rand failure is fatal to
// the operation and is surfaced, never ignored.
func newToken() (string, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("invite token: read random: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b[:]), nil
}

// Create inserts a tenant-scoped invite and writes one audit row, atomically.
// ttl is the lifetime from now until the invite expires.
func (r *InvitesRepo) Create(ctx context.Context, tenantID int64, email, role string, createdBy int64, ttl time.Duration) (*Invite, error) {
	if tenantID == 0 {
		return nil, errors.New("create invite: tenant id required")
	}
	if email == "" {
		return nil, errors.New("create invite: email is required")
	}
	token, err := newToken()
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	expires := time.Now().UTC().Add(ttl).Format(time.RFC3339)

	var created gen.Invite
	err = audit.WithTx(ctx, r.db, audit.Entry{Action: ""}, func(tx *sql.Tx) error {
		row, e := gen.New(tx).CreateInvite(ctx, gen.CreateInviteParams{
			Uuid:      uuid.NewString(),
			TenantID:  tenantID,
			Token:     token,
			Email:     email,
			Role:      role,
			CreatedBy: createdBy,
			ExpiresAt: expires,
			CreatedAt: now,
		})
		if e != nil {
			return fmt.Errorf("insert: %w", e)
		}
		created = row
		return audit.Log(ctx, tx, audit.Entry{
			EntityType: "invite",
			EntityID:   row.ID,
			Action:     "create",
			Changes:    audit.Changes(map[string]any{"email": email}),
		})
	})
	if err != nil {
		return nil, fmt.Errorf("create invite: %w", err)
	}
	return toInvite(created), nil
}

// GetByToken returns the invite (global lookup; token is unique), or (nil, nil)
// when none matches.
func (r *InvitesRepo) GetByToken(ctx context.Context, token string) (*Invite, error) {
	row, err := gen.New(r.db).GetInviteByToken(ctx, token)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get invite by token: %w", err)
	}
	return toInvite(row), nil
}

// Validate returns the invite when it is usable; otherwise ErrInviteInvalid.
func (r *InvitesRepo) Validate(ctx context.Context, token string) (*Invite, error) {
	inv, err := r.GetByToken(ctx, token)
	if err != nil {
		return nil, err
	}
	if inv == nil || inv.Accepted {
		return nil, ErrInviteInvalid
	}
	expires, err := time.Parse(time.RFC3339, inv.ExpiresAt)
	if err != nil {
		return nil, fmt.Errorf("validate invite: parse expires_at: %w", err)
	}
	if time.Now().After(expires) {
		return nil, ErrInviteInvalid
	}
	return inv, nil
}

// MarkAccepted records the invite as consumed and writes one audit row,
// atomically.
func (r *InvitesRepo) MarkAccepted(ctx context.Context, token string) error {
	inv, err := r.GetByToken(ctx, token)
	if err != nil {
		return err
	}
	if inv == nil {
		return ErrInviteInvalid
	}

	return audit.WithTx(ctx, r.db, audit.Entry{
		EntityType: "invite",
		EntityID:   inv.ID,
		Action:     "accepted",
		Changes:    audit.Changes(map[string]any{"token": token}),
	}, func(tx *sql.Tx) error {
		now := time.Now().UTC().Format(time.RFC3339)
		if err := gen.New(tx).MarkInviteAccepted(ctx, gen.MarkInviteAcceptedParams{
			AcceptedAt: nz(now),
			Token:      token,
		}); err != nil {
			return fmt.Errorf("update: %w", err)
		}
		return nil
	})
}

// Accept consumes an invite atomically: in ONE transaction it re-validates the
// invite, creates the user IN THE INVITE'S TENANT, marks the invite accepted,
// and writes an audit row. Returns ErrInviteInvalid if the token is
// unknown/expired/accepted, or ErrEmailTaken if a user with the invite's email
// already exists in that tenant.
func (r *InvitesRepo) Accept(ctx context.Context, token, name, passwordHash string) (*User, error) {
	if token == "" || passwordHash == "" {
		return nil, errors.New("accept invite: token and hash required")
	}
	var created gen.User
	err := audit.WithTx(ctx, r.db, audit.Entry{Action: ""}, func(tx *sql.Tx) error {
		q := gen.New(tx)
		inv, e := validateInviteTx(ctx, q, token)
		if e != nil {
			return e
		}
		now := time.Now().UTC().Format(time.RFC3339)
		u, e := q.CreateUser(ctx, gen.CreateUserParams{
			Uuid:            uuid.NewString(),
			TenantID:        inv.TenantID,
			Email:           inv.Email,
			PasswordHash:    passwordHash,
			Name:            name,
			IsPlatformAdmin: 0,
			Role:            inv.Role,
			CreatedAt:       now,
			UpdatedAt:       now,
		})
		if e != nil {
			if isUniqueViolation(e) {
				return ErrEmailTaken
			}
			return fmt.Errorf("create user: %w", e)
		}
		created = u
		if e := q.MarkInviteAccepted(ctx, gen.MarkInviteAcceptedParams{AcceptedAt: nz(now), Token: token}); e != nil {
			return fmt.Errorf("mark accepted: %w", e)
		}
		return audit.Log(ctx, tx, audit.Entry{
			EntityType: "invite",
			EntityID:   inv.ID,
			Action:     "accepted",
			Changes:    audit.Changes(map[string]any{"email": inv.Email, "userId": u.ID}),
		})
	})
	if err != nil {
		return nil, err
	}
	return toUser(created), nil
}

// validateInviteTx re-checks an invite inside an open transaction, returning the
// domain Invite or ErrInviteInvalid when unknown, accepted, or expired.
func validateInviteTx(ctx context.Context, q *gen.Queries, token string) (*Invite, error) {
	row, err := q.GetInviteByToken(ctx, token)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrInviteInvalid
	}
	if err != nil {
		return nil, fmt.Errorf("get invite: %w", err)
	}
	inv := toInvite(row)
	if inv.Accepted {
		return nil, ErrInviteInvalid
	}
	expires, err := time.Parse(time.RFC3339, inv.ExpiresAt)
	if err != nil {
		return nil, fmt.Errorf("parse expires: %w", err)
	}
	if time.Now().After(expires) {
		return nil, ErrInviteInvalid
	}
	return inv, nil
}

// isUniqueViolation reports whether err is a SQLite UNIQUE-constraint failure.
// modernc.org/sqlite surfaces these as "UNIQUE constraint failed: ...". We match
// only that phrase: matching a bare "constraint" would misclassify FK / NOT NULL
// failures (e.g. a bad tenant_id) as ErrEmailTaken in Accept().
func isUniqueViolation(err error) bool {
	return strings.Contains(strings.ToLower(err.Error()), "unique constraint")
}

// toInvite maps a generated row to the domain Invite.
func toInvite(row gen.Invite) *Invite {
	return &Invite{
		ID:        row.ID,
		TenantID:  row.TenantID,
		Token:     row.Token,
		Email:     row.Email,
		Role:      row.Role,
		ExpiresAt: row.ExpiresAt,
		Accepted:  row.AcceptedAt.Valid,
	}
}
