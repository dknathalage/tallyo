package shift

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/dknathalage/tallyo/internal/realtime"
	"github.com/dknathalage/tallyo/internal/reqctx"
)

// InvoiceChecker is the narrow interface the shift service requires to verify
// that an invoice exists before linking shifts to it. It breaks the
// shift→invoice import cycle: the shift package declares this interface; the
// caller (main.go) injects a concrete *invoice.InvoicesRepo which satisfies it.
type InvoiceChecker interface {
	Exists(ctx context.Context, tenantID, invoiceID int64) (bool, error)
}

// Service orchestrates the shift lifecycle (record→draft→bill) and
// publishes change events after a successful commit. It resolves the caller's
// tenant (and, for authorship, user) from the request context.
type Service struct {
	repo     *ShiftsRepo
	invoices InvoiceChecker
	hub      *realtime.Hub
}

// NewService constructs the shift service. A nil hub is a programmer error.
// invoices is the InvoiceChecker used to verify the invoice in MarkDrafted.
func NewService(db *sql.DB, hub *realtime.Hub, invoices InvoiceChecker) *Service {
	if hub == nil {
		panic("shift.NewService: nil hub")
	}
	return &Service{repo: NewShifts(db), invoices: invoices, hub: hub}
}

// Suggestion is a billing prompt: a participant's recorded-but-unbilled shifts
// grouped together, ready to draft onto a single invoice.
type Suggestion struct {
	ParticipantID int64   `json:"participantId"`
	IDs           []int64 `json:"ids"`
	From          string  `json:"from"`
	To            string  `json:"to"`
	Count         int     `json:"count"`
}

// ListParticipant returns a participant's shifts, optionally restricted to the
// [from, to] service-date window (both empty → all shifts).
func (s *Service) ListParticipant(ctx context.Context, participantID int64, from, to string) ([]*Shift, error) {
	tenantID := reqctx.MustTenant(ctx)
	return s.repo.ListParticipant(ctx, tenantID, participantID, from, to)
}

// List returns all the tenant's shifts. When status is non-empty the result is
// restricted to shifts in that lifecycle status.
func (s *Service) List(ctx context.Context, status string) ([]*Shift, error) {
	tenantID := reqctx.MustTenant(ctx)
	if status != "" {
		return s.repo.ListByStatus(ctx, tenantID, status)
	}
	return s.repo.List(ctx, tenantID)
}

// Get returns a shift by id, or (nil, nil) when absent.
func (s *Service) Get(ctx context.Context, id int64) (*Shift, error) {
	tenantID := reqctx.MustTenant(ctx)
	return s.repo.Get(ctx, tenantID, id)
}

// Create inserts a shift attributed to the authenticated user, then broadcasts
// after the commit succeeds.
func (s *Service) Create(ctx context.Context, in ShiftInput) (*Shift, error) {
	tenantID := reqctx.MustTenant(ctx)
	var author *int64
	if uid, ok := reqctx.UserFrom(ctx); ok {
		author = &uid
	}
	sh, err := s.repo.Create(ctx, tenantID, author, in)
	if err != nil {
		return nil, err
	}
	s.hub.Broadcast(realtime.Event{TenantID: tenantID, Entity: "shift", ID: sh.ID, Action: "create"})
	return sh, nil
}

// Update mutates a shift, then broadcasts on success. A nil result means the row
// was not found, in which case no event is published.
func (s *Service) Update(ctx context.Context, id int64, in ShiftInput) (*Shift, error) {
	tenantID := reqctx.MustTenant(ctx)
	sh, err := s.repo.Update(ctx, tenantID, id, in)
	if err != nil {
		return nil, err
	}
	if sh == nil {
		return nil, nil
	}
	s.hub.Broadcast(realtime.Event{TenantID: tenantID, Entity: "shift", ID: id, Action: "update"})
	return sh, nil
}

// UpdateStatus advances a shift's lifecycle status, then broadcasts on success.
func (s *Service) UpdateStatus(ctx context.Context, id int64, status string) error {
	tenantID := reqctx.MustTenant(ctx)
	if err := s.repo.UpdateStatus(ctx, tenantID, id, status); err != nil {
		return err
	}
	s.hub.Broadcast(realtime.Event{TenantID: tenantID, Entity: "shift", ID: id, Action: "update"})
	return nil
}

// Delete removes a shift, then broadcasts on success.
func (s *Service) Delete(ctx context.Context, id int64) error {
	tenantID := reqctx.MustTenant(ctx)
	if err := s.repo.Delete(ctx, tenantID, id); err != nil {
		return err
	}
	s.hub.Broadcast(realtime.Event{TenantID: tenantID, Entity: "shift", ID: id, Action: "delete"})
	return nil
}

// ToRecord returns the tenant's scheduled shifts still awaiting a record.
func (s *Service) ToRecord(ctx context.Context) ([]*Shift, error) {
	tenantID := reqctx.MustTenant(ctx)
	return s.repo.ListScheduled(ctx, tenantID)
}

// Suggestions groups each participant's recorded-but-unbilled shifts into a
// billing prompt, resolving the concrete shift ids per participant.
func (s *Service) Suggestions(ctx context.Context) ([]Suggestion, error) {
	tenantID := reqctx.MustTenant(ctx)
	aggs, err := s.repo.UnbilledByParticipant(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	out := make([]Suggestion, 0, len(aggs))
	for i := range aggs { // bounded by len(aggs)
		shifts, e := s.repo.ListRecordedUnbilled(ctx, tenantID, aggs[i].ParticipantID)
		if e != nil {
			return nil, e
		}
		ids := make([]int64, 0, len(shifts))
		for j := range shifts { // bounded by len(shifts)
			ids = append(ids, shifts[j].ID)
		}
		out = append(out, Suggestion{
			ParticipantID: aggs[i].ParticipantID,
			IDs:           ids,
			From:          aggs[i].From,
			To:            aggs[i].To,
			Count:         int(aggs[i].Count),
		})
	}
	return out, nil
}

// MarkDrafted links the given recorded shifts to an invoice (status 'drafted'),
// then broadcasts a single bulk event. An empty id list is a no-op. The invoice
// MUST belong to the caller's tenant — verified tenant-scoped first to prevent
// cross-tenant linkage.
func (s *Service) MarkDrafted(ctx context.Context, invoiceID int64, shiftIDs []int64) error {
	tenantID := reqctx.MustTenant(ctx)
	if len(shiftIDs) == 0 {
		return nil
	}
	if invoiceID <= 0 {
		return fmt.Errorf("mark drafted: invoice id required")
	}
	exists, err := s.invoices.Exists(ctx, tenantID, invoiceID)
	if err != nil {
		return fmt.Errorf("mark drafted: verify invoice: %w", err)
	}
	if !exists {
		return fmt.Errorf("mark drafted: invoice %d not found for tenant", invoiceID)
	}
	for i := range shiftIDs { // bounded by len(shiftIDs)
		if err := s.repo.SetInvoice(ctx, tenantID, shiftIDs[i], invoiceID, "drafted"); err != nil {
			return err
		}
	}
	s.hub.Broadcast(realtime.Event{TenantID: tenantID, Entity: "shift", ID: 0, Action: "bill"})
	return nil
}
