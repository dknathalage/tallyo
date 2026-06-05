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
// or already used. Callers should treat all three identically.
var ErrInviteInvalid = errors.New("invite invalid or expired")

// ErrEmailTaken is returned by Accept when a user with the invite's email
// already exists.
var ErrEmailTaken = errors.New("email already registered")

// Invite is the domain view of a row in the invites table.
type Invite struct {
	ID        int64  `json:"id"`
	Token     string `json:"token"`
	Email     string `json:"email"`
	Role      string `json:"role"`
	ExpiresAt string `json:"expiresAt"`
	Used      bool   `json:"used"`
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

// Create inserts an invite and writes one audit row, atomically. ttl is the
// lifetime from now until the invite expires.
func (r *InvitesRepo) Create(ctx context.Context, email, role string, createdBy int64, ttl time.Duration) (*Invite, error) {
	if email == "" {
		return nil, errors.New("create invite: email is required")
	}
	token, err := newToken()
	if err != nil {
		return nil, err
	}
	expires := time.Now().UTC().Add(ttl).Format(time.RFC3339)

	var created gen.Invite
	err = audit.WithTx(ctx, r.db, audit.Entry{Action: ""}, func(tx *sql.Tx) error {
		row, e := gen.New(tx).CreateInvite(ctx, gen.CreateInviteParams{
			Token:     token,
			Email:     email,
			Role:      role,
			CreatedBy: createdBy,
			ExpiresAt: expires,
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

// GetByToken returns the invite, or (nil, nil) when none matches.
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
	if inv == nil || inv.Used {
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

// MarkUsed records the invite as consumed and writes one audit row, atomically.
func (r *InvitesRepo) MarkUsed(ctx context.Context, token string) error {
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
		Action:     "used",
		Changes:    audit.Changes(map[string]any{"token": token}),
	}, func(tx *sql.Tx) error {
		now := time.Now().UTC().Format(time.RFC3339)
		if err := gen.New(tx).MarkInviteUsed(ctx, gen.MarkInviteUsedParams{
			UsedAt: nz(now),
			Token:  token,
		}); err != nil {
			return fmt.Errorf("update: %w", err)
		}
		return nil
	})
}

// Accept consumes an invite atomically: in ONE transaction it re-validates the
// invite, creates the user, marks the invite used, and writes an audit row.
// Returns ErrInviteInvalid if the token is unknown/expired/used, or ErrEmailTaken
// if a user with the invite's email already exists.
func (r *InvitesRepo) Accept(ctx context.Context, token, passwordHash string) (*User, error) {
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
			Uuid:         uuid.NewString(),
			Email:        inv.Email,
			PasswordHash: passwordHash,
			Role:         inv.Role,
			CreatedAt:    now,
			UpdatedAt:    now,
		})
		if e != nil {
			if isUniqueViolation(e) {
				return ErrEmailTaken
			}
			return fmt.Errorf("create user: %w", e)
		}
		created = u
		if e := q.MarkInviteUsed(ctx, gen.MarkInviteUsedParams{UsedAt: nz(now), Token: token}); e != nil {
			return fmt.Errorf("mark used: %w", e)
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
// domain Invite or ErrInviteInvalid when unknown, used, or expired.
func validateInviteTx(ctx context.Context, q *gen.Queries, token string) (*Invite, error) {
	row, err := q.GetInviteByToken(ctx, token)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrInviteInvalid
	}
	if err != nil {
		return nil, fmt.Errorf("get invite: %w", err)
	}
	inv := toInvite(row)
	if inv.Used {
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

// isUniqueViolation reports whether err is a SQLite unique-constraint failure.
func isUniqueViolation(err error) bool {
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "unique") || strings.Contains(s, "constraint")
}

// toInvite maps a generated row to the domain Invite.
func toInvite(row gen.Invite) *Invite {
	return &Invite{
		ID:        row.ID,
		Token:     row.Token,
		Email:     row.Email,
		Role:      row.Role,
		ExpiresAt: row.ExpiresAt,
		Used:      row.UsedAt.Valid,
	}
}
