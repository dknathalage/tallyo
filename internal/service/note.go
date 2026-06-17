package service

import (
	"context"
	"database/sql"

	"github.com/dknathalage/tallyo/internal/realtime"
	"github.com/dknathalage/tallyo/internal/repository"
	"github.com/dknathalage/tallyo/internal/reqctx"
)

// NoteService orchestrates per-participant journal notes and publishes change
// events after a successful commit. It resolves the caller's tenant (and, for
// authorship, user) from the request context.
type NoteService struct {
	repo *repository.NotesRepo
	hub  *realtime.Hub
}

func NewNoteService(db *sql.DB, hub *realtime.Hub) *NoteService {
	if hub == nil {
		panic("NewNoteService: nil hub")
	}
	return &NoteService{repo: repository.NewNotes(db), hub: hub}
}

// ListParticipant returns a participant's notes, optionally restricted to the
// [from, to] service-date window (both empty → all notes).
func (s *NoteService) ListParticipant(ctx context.Context, participantID int64, from, to string) ([]*repository.Note, error) {
	tenantID := reqctx.MustTenant(ctx)
	return s.repo.ListParticipant(ctx, tenantID, participantID, from, to)
}

// Get returns a note by id, or (nil, nil) when absent.
func (s *NoteService) Get(ctx context.Context, id int64) (*repository.Note, error) {
	tenantID := reqctx.MustTenant(ctx)
	return s.repo.Get(ctx, tenantID, id)
}

// Create inserts a note attributed to the authenticated user, then broadcasts
// after the commit succeeds.
func (s *NoteService) Create(ctx context.Context, in repository.NoteInput) (*repository.Note, error) {
	tenantID := reqctx.MustTenant(ctx)
	var author *int64
	if uid, ok := reqctx.UserFrom(ctx); ok {
		author = &uid
	}
	n, err := s.repo.Create(ctx, tenantID, author, in)
	if err != nil {
		return nil, err
	}
	s.hub.Broadcast(realtime.Event{TenantID: tenantID, Entity: "note", ID: n.ID, Action: "create"})
	return n, nil
}

// Update mutates a note, then broadcasts on success. A nil result means the row
// was not found, in which case no event is published.
func (s *NoteService) Update(ctx context.Context, id int64, in repository.NoteInput) (*repository.Note, error) {
	tenantID := reqctx.MustTenant(ctx)
	n, err := s.repo.Update(ctx, tenantID, id, in)
	if err != nil {
		return nil, err
	}
	if n == nil {
		return nil, nil
	}
	s.hub.Broadcast(realtime.Event{TenantID: tenantID, Entity: "note", ID: id, Action: "update"})
	return n, nil
}

// Delete removes a note, then broadcasts on success.
func (s *NoteService) Delete(ctx context.Context, id int64) error {
	tenantID := reqctx.MustTenant(ctx)
	if err := s.repo.Delete(ctx, tenantID, id); err != nil {
		return err
	}
	s.hub.Broadcast(realtime.Event{TenantID: tenantID, Entity: "note", ID: id, Action: "delete"})
	return nil
}

// Bill links each note to an invoice (the soft billing flag), then broadcasts a
// single bulk event. An empty id list is a no-op.
func (s *NoteService) Bill(ctx context.Context, invoiceID int64, noteIDs []int64) error {
	tenantID := reqctx.MustTenant(ctx)
	if len(noteIDs) == 0 {
		return nil
	}
	id := invoiceID
	if err := s.repo.MarkBilled(ctx, tenantID, &id, noteIDs); err != nil {
		return err
	}
	s.hub.Broadcast(realtime.Event{TenantID: tenantID, Entity: "note", ID: 0, Action: "bill"})
	return nil
}
