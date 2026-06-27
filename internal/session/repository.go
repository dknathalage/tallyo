// Package session is the session vertical slice: domain types, the audited
// repository over the sessions table, the service (with SSE broadcast), and the
// HTTP handler. It depends only on platform packages (db/gen, audit, reqctx,
// httpx), never on other domain slices.
package session

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/dknathalage/tallyo/internal/audit"
	"github.com/dknathalage/tallyo/internal/db"
	"github.com/dknathalage/tallyo/internal/db/gen"
	"github.com/dknathalage/tallyo/internal/ids"
)

// SessionsRepo reads and writes the sessions table (tenant-scoped) with audited
// mutations.
type SessionsRepo struct {
	db db.Executor
}

// NewSessions constructs a repository. A nil db is a programmer error.
func NewSessions(db db.Executor) *SessionsRepo {
	if db == nil {
		panic("session: NewSessions requires a non-nil *sql.DB")
	}
	return &SessionsRepo{db: db}
}

// Create inserts a session and writes one audit row, atomically. authorUserID is
// the user the session is attributed to (nil when unknown). Status defaults to
// 'recorded' when the input leaves it empty. The service validates the input
// (client id + service date) before this runs; the tenant/client guards here are
// the module-boundary invariant checks.
func (r *SessionsRepo) Create(ctx context.Context, tenantID string, authorUserID *string, in SessionInput) (*Session, error) {
	if tenantID == "" {
		return nil, errors.New("create session: tenant id required")
	}
	if in.ClientID == "" {
		return nil, errors.New("create session: client id required")
	}
	if !validISODate(in.ServiceDate) {
		return nil, errors.New("create session: service date must be a valid YYYY-MM-DD date")
	}
	tags, err := encodeTags(in.Tags)
	if err != nil {
		return nil, err
	}
	status := in.Status
	if status == "" {
		status = "recorded"
	}

	var newID string
	err = audit.WithTx(ctx, r.db, audit.Entry{Action: ""}, func(tx *sql.Tx) error {
		now := time.Now().UTC().Format(time.RFC3339)
		s, e := gen.New(tx).CreateSession(ctx, gen.CreateSessionParams{
			ID:           ids.New(),
			TenantID:     tenantID,
			ClientID:     in.ClientID,
			ServiceDate:  in.ServiceDate,
			Note:         in.Note,
			Tags:         tags,
			Status:       status,
			AuthorUserID: db.NullStr(authorUserID),
			CreatedAt:    now,
			UpdatedAt:    now,
		})
		if e != nil {
			return fmt.Errorf("insert: %w", e)
		}
		newID = s.ID
		return audit.Log(ctx, tx, audit.Entry{
			EntityType: "session", EntityID: s.ID, Action: "create",
			Changes: audit.Changes(map[string]any{"clientId": in.ClientID, "serviceDate": in.ServiceDate}),
		})
	})
	if err != nil {
		return nil, fmt.Errorf("create session: %w", err)
	}
	return r.Get(ctx, tenantID, newID)
}

// Get returns the tenant's session by row id, or (nil, nil) when absent. This is
// the internal/cross-slice read (agent SessionReader, the service's own pricing
// path); the public HTTP path addresses sessions by uuid via GetByUUID.
func (r *SessionsRepo) Get(ctx context.Context, tenantID, id string) (*Session, error) {
	row, err := gen.New(r.db).GetSessionByID(ctx, gen.GetSessionByIDParams{TenantID: tenantID, ID: id})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get session: %w", err)
	}
	return mapSession(sessionFieldsFromByID(row))
}

// GetByUUID returns the tenant's session by uuid, or (nil, nil) when absent (or
// owned by another tenant — the query is tenant-scoped). This is the public
// HTTP read.
func (r *SessionsRepo) GetByUUID(ctx context.Context, tenantID string, sessionUUID string) (*Session, error) {
	row, err := gen.New(r.db).GetSession(ctx, gen.GetSessionParams{TenantID: tenantID, ID: sessionUUID})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get session by uuid: %w", err)
	}
	return mapSession(sessionFieldsFromGet(row))
}

// ResolveID resolves a session uuid to its row id (uuid) for the tenant. Returns
// ("", nil) when no such session exists (so callers can 404 without an error).
func (r *SessionsRepo) ResolveID(ctx context.Context, tenantID string, sessionUUID string) (string, error) {
	id, err := gen.New(r.db).GetSessionIDByUUID(ctx, gen.GetSessionIDByUUIDParams{TenantID: tenantID, ID: sessionUUID})
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("resolve session uuid: %w", err)
	}
	return id, nil
}

// ResolveClientID resolves a client uuid to its row id (uuid) for the
// tenant (used by the ?client= session filter and inbound clientId
// resolution). Returns ("", nil) when absent.
func (r *SessionsRepo) ResolveClientID(ctx context.Context, tenantID string, clientUUID string) (string, error) {
	id, err := gen.New(r.db).GetClientIDByUUID(ctx, gen.GetClientIDByUUIDParams{TenantID: tenantID, ID: clientUUID})
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("resolve client uuid: %w", err)
	}
	return id, nil
}

// Update rewrites a session's editable fields by uuid and writes one audit row,
// atomically. Returns (nil, nil) when the session does not exist for the tenant.
// The audit EntityID keeps the row id (uuid), recovered from the RETURNING row.
// The service validates the service date before this runs.
func (r *SessionsRepo) Update(ctx context.Context, tenantID string, sessionUUID string, in SessionInput) (*Session, error) {
	if !validISODate(in.ServiceDate) {
		return nil, errors.New("update session: service date must be a valid YYYY-MM-DD date")
	}
	tags, err := encodeTags(in.Tags)
	if err != nil {
		return nil, err
	}
	status := in.Status
	if status == "" {
		status = "recorded"
	}

	var missing bool
	err = audit.WithTx(ctx, r.db, audit.Entry{Action: ""}, func(tx *sql.Tx) error {
		now := time.Now().UTC().Format(time.RFC3339)
		row, e := gen.New(tx).UpdateSession(ctx, gen.UpdateSessionParams{
			ServiceDate: in.ServiceDate,
			Note:        in.Note,
			Tags:        tags,
			Status:      status,
			UpdatedAt:   now,
			TenantID:    tenantID,
			ID:          sessionUUID,
		})
		if errors.Is(e, sql.ErrNoRows) {
			missing = true
			return e
		}
		if e != nil {
			return fmt.Errorf("update: %w", e)
		}
		return audit.Log(ctx, tx, audit.Entry{EntityType: "session", EntityID: row.ID, Action: "update"})
	})
	if missing {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("update session: %w", err)
	}
	return r.GetByUUID(ctx, tenantID, sessionUUID)
}

// UpdateStatus sets a session's lifecycle status by uuid and writes one audit row.
// The audit EntityID keeps the row id (uuid), resolved in-tx; a missing row is a no-op.
func (r *SessionsRepo) UpdateStatus(ctx context.Context, tenantID string, sessionUUID, status string) error {
	if status == "" {
		return errors.New("update session status: status required")
	}
	return audit.WithTx(ctx, r.db, audit.Entry{Action: ""}, func(tx *sql.Tx) error {
		q := gen.New(tx)
		id, e := q.GetSessionIDByUUID(ctx, gen.GetSessionIDByUUIDParams{TenantID: tenantID, ID: sessionUUID})
		if errors.Is(e, sql.ErrNoRows) {
			return nil // missing row → silent no-op
		}
		if e != nil {
			return fmt.Errorf("resolve session: %w", e)
		}
		now := time.Now().UTC().Format(time.RFC3339)
		if err := q.UpdateSessionStatus(ctx, gen.UpdateSessionStatusParams{
			Status: status, UpdatedAt: now, TenantID: tenantID, ID: sessionUUID,
		}); err != nil {
			return fmt.Errorf("update status: %w", err)
		}
		return audit.Log(ctx, tx, audit.Entry{
			EntityType: "session", EntityID: id, Action: "status",
			Changes: audit.Changes(map[string]any{"status": status}),
		})
	})
}

// Delete removes a session by uuid and writes one audit row, atomically. The audit
// EntityID keeps the row id (uuid), resolved in-tx; a missing row is a no-op.
func (r *SessionsRepo) Delete(ctx context.Context, tenantID string, sessionUUID string) error {
	return audit.WithTx(ctx, r.db, audit.Entry{Action: ""}, func(tx *sql.Tx) error {
		q := gen.New(tx)
		id, e := q.GetSessionIDByUUID(ctx, gen.GetSessionIDByUUIDParams{TenantID: tenantID, ID: sessionUUID})
		if errors.Is(e, sql.ErrNoRows) {
			return nil // missing row → silent no-op
		}
		if e != nil {
			return fmt.Errorf("resolve session: %w", e)
		}
		if err := q.DeleteSession(ctx, gen.DeleteSessionParams{TenantID: tenantID, ID: sessionUUID}); err != nil {
			return fmt.Errorf("delete: %w", err)
		}
		return audit.Log(ctx, tx, audit.Entry{EntityType: "session", EntityID: id, Action: "delete"})
	})
}
