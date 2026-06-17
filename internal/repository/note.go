package repository

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

// Note is the domain view of a row in the notes table — one daily journal entry
// a provider keeps for a participant. Body is free-text (UNTRUSTED when fed to
// the agent). TransportKm/SupportHours are optional structured tags. BilledID is
// the soft billing flag (nil until the note is billed onto an invoice).
type Note struct {
	ID            int64    `json:"id"`
	UUID          string   `json:"uuid"`
	ParticipantID int64    `json:"participantId"`
	ServiceDate   string   `json:"serviceDate"`
	Body          string   `json:"body"`
	TransportKm   *float64 `json:"transportKm"`
	SupportHours  *float64 `json:"supportHours"`
	AuthorUserID  *int64   `json:"authorUserId"`
	BilledID      *int64   `json:"billedInvoiceId"`
	CreatedAt     string   `json:"createdAt"`
	UpdatedAt     string   `json:"updatedAt"`
}

// NoteInput is the writable subset of a note.
type NoteInput struct {
	ParticipantID int64    `json:"participantId"`
	ServiceDate   string   `json:"serviceDate"`
	Body          string   `json:"body"`
	TransportKm   *float64 `json:"transportKm"`
	SupportHours  *float64 `json:"supportHours"`
}

// NotesRepo reads and writes the notes table (tenant-scoped) with audited
// mutations.
type NotesRepo struct {
	db *sql.DB
}

// NewNotes constructs a repository. A nil db is a programmer error.
func NewNotes(db *sql.DB) *NotesRepo {
	if db == nil {
		panic("repository: NewNotes requires a non-nil *sql.DB")
	}
	return &NotesRepo{db: db}
}

// ListParticipant returns a participant's notes. When both from and to are
// non-empty it restricts to service_date ∈ [from, to]; otherwise it returns all.
func (r *NotesRepo) ListParticipant(ctx context.Context, tenantID, participantID int64, from, to string) ([]*Note, error) {
	if tenantID == 0 || participantID == 0 {
		return nil, errors.New("list notes: tenant and participant id required")
	}
	q := gen.New(r.db)
	if from != "" && to != "" {
		rows, err := q.ListParticipantNotesRange(ctx, gen.ListParticipantNotesRangeParams{
			TenantID: tenantID, ParticipantID: participantID, ServiceDate: from, ServiceDate_2: to,
		})
		if err != nil {
			return nil, fmt.Errorf("list participant notes range: %w", err)
		}
		return toNotes(rows), nil
	}
	rows, err := q.ListParticipantNotes(ctx, gen.ListParticipantNotesParams{
		TenantID: tenantID, ParticipantID: participantID,
	})
	if err != nil {
		return nil, fmt.Errorf("list participant notes: %w", err)
	}
	return toNotes(rows), nil
}

// Get returns the tenant's note by id, or (nil, nil) when absent.
func (r *NotesRepo) Get(ctx context.Context, tenantID, id int64) (*Note, error) {
	row, err := gen.New(r.db).GetNote(ctx, gen.GetNoteParams{TenantID: tenantID, ID: id})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get note: %w", err)
	}
	return toNote(row), nil
}

// Create inserts a note and writes one audit row, atomically. authorUserID is
// the user the note is attributed to (nil when unknown).
func (r *NotesRepo) Create(ctx context.Context, tenantID int64, authorUserID *int64, in NoteInput) (*Note, error) {
	if tenantID == 0 {
		return nil, errors.New("create note: tenant id required")
	}
	if in.ParticipantID == 0 {
		return nil, errors.New("create note: participant id required")
	}
	if in.ServiceDate == "" {
		return nil, errors.New("create note: service date is required")
	}
	if in.Body == "" {
		return nil, errors.New("create note: body is required")
	}
	if err := assertNonNegative(in); err != nil {
		return nil, err
	}

	var newID int64
	err := audit.WithTx(ctx, r.db, audit.Entry{Action: ""}, func(tx *sql.Tx) error {
		now := time.Now().UTC().Format(time.RFC3339)
		n, e := gen.New(tx).CreateNote(ctx, gen.CreateNoteParams{
			Uuid:          uuid.NewString(),
			TenantID:      tenantID,
			ParticipantID: in.ParticipantID,
			ServiceDate:   in.ServiceDate,
			Body:          in.Body,
			TransportKm:   nullFloat(in.TransportKm),
			SupportHours:  nullFloat(in.SupportHours),
			AuthorUserID:  nullID(authorUserID),
			CreatedAt:     now,
			UpdatedAt:     now,
		})
		if e != nil {
			return fmt.Errorf("insert: %w", e)
		}
		newID = n.ID
		return audit.Log(ctx, tx, audit.Entry{
			EntityType: "note", EntityID: n.ID, Action: "create",
			Changes: audit.Changes(map[string]any{"participantId": in.ParticipantID, "serviceDate": in.ServiceDate}),
		})
	})
	if err != nil {
		return nil, fmt.Errorf("create note: %w", err)
	}
	return r.Get(ctx, tenantID, newID)
}

// Update rewrites a note's editable fields and writes one audit row, atomically.
// Returns (nil, nil) when the note does not exist for the tenant.
func (r *NotesRepo) Update(ctx context.Context, tenantID, id int64, in NoteInput) (*Note, error) {
	if in.ServiceDate == "" {
		return nil, errors.New("update note: service date is required")
	}
	if in.Body == "" {
		return nil, errors.New("update note: body is required")
	}
	if err := assertNonNegative(in); err != nil {
		return nil, err
	}

	var missing bool
	err := audit.WithTx(ctx, r.db, audit.Entry{
		EntityType: "note", EntityID: id, Action: "update",
	}, func(tx *sql.Tx) error {
		now := time.Now().UTC().Format(time.RFC3339)
		_, e := gen.New(tx).UpdateNote(ctx, gen.UpdateNoteParams{
			ServiceDate:  in.ServiceDate,
			Body:         in.Body,
			TransportKm:  nullFloat(in.TransportKm),
			SupportHours: nullFloat(in.SupportHours),
			UpdatedAt:    now,
			TenantID:     tenantID,
			ID:           id,
		})
		if errors.Is(e, sql.ErrNoRows) {
			missing = true
			return e
		}
		if e != nil {
			return fmt.Errorf("update: %w", e)
		}
		return nil
	})
	if missing {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("update note: %w", err)
	}
	return r.Get(ctx, tenantID, id)
}

// Delete removes a note and writes one audit row, atomically.
func (r *NotesRepo) Delete(ctx context.Context, tenantID, id int64) error {
	return audit.WithTx(ctx, r.db, audit.Entry{
		EntityType: "note", EntityID: id, Action: "delete",
	}, func(tx *sql.Tx) error {
		if err := gen.New(tx).DeleteNote(ctx, gen.DeleteNoteParams{TenantID: tenantID, ID: id}); err != nil {
			return fmt.Errorf("delete: %w", err)
		}
		return nil
	})
}

// MarkBilled sets (or clears, when invoiceID is nil) the soft billing flag on
// each note id, in one audited transaction. An empty id list is a no-op.
func (r *NotesRepo) MarkBilled(ctx context.Context, tenantID int64, invoiceID *int64, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	return audit.WithTx(ctx, r.db, audit.Entry{
		EntityType: "note", EntityID: 0, Action: "bill",
		Changes: audit.Changes(map[string]any{"invoiceId": invoiceID, "ids": ids}),
	}, func(tx *sql.Tx) error {
		q := gen.New(tx)
		now := time.Now().UTC().Format(time.RFC3339)
		for _, id := range ids { // bounded by len(ids)
			if err := q.MarkNoteBilled(ctx, gen.MarkNoteBilledParams{
				BilledInvoiceID: nullID(invoiceID), UpdatedAt: now, TenantID: tenantID, ID: id,
			}); err != nil {
				return fmt.Errorf("mark billed %d: %w", id, err)
			}
		}
		return nil
	})
}

// assertNonNegative rejects negative optional quantities at the boundary.
func assertNonNegative(in NoteInput) error {
	if in.TransportKm != nil && *in.TransportKm < 0 {
		return errors.New("note: transportKm must not be negative")
	}
	if in.SupportHours != nil && *in.SupportHours < 0 {
		return errors.New("note: supportHours must not be negative")
	}
	return nil
}

// nullFloat wraps an optional float into a sql.NullFloat64 (invalid when nil).
func nullFloat(p *float64) sql.NullFloat64 {
	if p == nil {
		return sql.NullFloat64{}
	}
	return sql.NullFloat64{Float64: *p, Valid: true}
}

// ptrFloat unwraps a sql.NullFloat64 into a *float64 (nil when invalid).
func ptrFloat(n sql.NullFloat64) *float64 {
	if !n.Valid {
		return nil
	}
	v := n.Float64
	return &v
}

func toNote(r gen.Note) *Note {
	return &Note{
		ID:            r.ID,
		UUID:          r.Uuid,
		ParticipantID: r.ParticipantID,
		ServiceDate:   r.ServiceDate,
		Body:          r.Body,
		TransportKm:   ptrFloat(r.TransportKm),
		SupportHours:  ptrFloat(r.SupportHours),
		AuthorUserID:  ptrID(r.AuthorUserID),
		BilledID:      ptrID(r.BilledInvoiceID),
		CreatedAt:     r.CreatedAt,
		UpdatedAt:     r.UpdatedAt,
	}
}

func toNotes(rows []gen.Note) []*Note {
	out := make([]*Note, 0, len(rows))
	for i := range rows { // bounded by len(rows)
		out = append(out, toNote(rows[i]))
	}
	return out
}
