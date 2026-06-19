package shift

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/dknathalage/tallyo/internal/billing"
	"github.com/dknathalage/tallyo/internal/realtime"
	"github.com/dknathalage/tallyo/internal/reqctx"
)

// ErrShiftBilled is returned when an operation is refused because the shift's
// items are already on an invoice (status past 'recorded').
var ErrShiftBilled = errors.New("shift: cannot delete a billed shift")

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
	repo      *ShiftsRepo
	invoices  InvoiceChecker
	validator *billing.LineValidator
	hub       *realtime.Hub
}

// NewService constructs the shift service. A nil hub is a programmer error.
// invoices is the InvoiceChecker used to verify the invoice in MarkDrafted. The
// shift service builds its own billing.LineValidator (catalogue-authoritative
// pricing) from the same db the invoice service uses — no extra wiring needed.
func NewService(db *sql.DB, hub *realtime.Hub, invoices InvoiceChecker) *Service {
	if hub == nil {
		panic("shift.NewService: nil hub")
	}
	return &Service{repo: NewShifts(db), invoices: invoices, validator: billing.NewLineValidator(db), hub: hub}
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
// was not found, in which case no event is published. When the service date
// changes, the shift's UNBILLED items are re-stamped to the new date and
// re-priced against that date's catalogue (G3/G4).
func (s *Service) Update(ctx context.Context, id int64, in ShiftInput) (*Shift, error) {
	tenantID := reqctx.MustTenant(ctx)
	prev, err := s.repo.Get(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	sh, err := s.repo.Update(ctx, tenantID, id, in)
	if err != nil {
		return nil, err
	}
	if sh == nil {
		return nil, nil
	}
	if prev != nil && prev.ServiceDate != sh.ServiceDate {
		if err := s.repriceItemsForDate(ctx, tenantID, sh); err != nil {
			return nil, err
		}
	}
	s.hub.Broadcast(realtime.Event{TenantID: tenantID, Entity: "shift", ID: id, Action: "update"})
	return sh, nil
}

// repriceItemsForDate re-stamps every unbilled item of the shift to the shift's
// (new) service date and re-prices it against that date's catalogue. Bounded by
// the number of items on the shift.
func (s *Service) repriceItemsForDate(ctx context.Context, tenantID int64, sh *Shift) error {
	items, err := s.repo.ListItems(ctx, tenantID, sh.ID)
	if err != nil {
		return err
	}
	for i := range items { // bounded by len(items)
		it := items[i]
		if it.InvoiceID != nil {
			continue // billed items are frozen
		}
		in := itemToInput(it)
		in.ServiceDate = sh.ServiceDate
		priced, err := s.priceItem(ctx, tenantID, sh.ParticipantID, in)
		if err != nil {
			return err
		}
		if _, err := s.repo.UpdateItem(ctx, tenantID, it.ID, priced); err != nil {
			return err
		}
	}
	return nil
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

// Delete removes a shift (its items cascade), then broadcasts on success. A
// billed shift — status past 'recorded' (drafted/sent/paid) — cannot be deleted:
// its items live on an invoice. Returns ErrShiftBilled in that case.
func (s *Service) Delete(ctx context.Context, id int64) error {
	tenantID := reqctx.MustTenant(ctx)
	sh, err := s.repo.Get(ctx, tenantID, id)
	if err != nil {
		return err
	}
	if sh == nil {
		return nil
	}
	if sh.Status != "scheduled" && sh.Status != "recorded" {
		return ErrShiftBilled
	}
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

// SetStatusForInvoice advances every shift linked to an invoice to status (the
// invoice→shift cascade on 'sent'/'paid'). It satisfies invoice.ShiftLinker;
// tenantID is supplied by the caller (the invoice service's request scope).
func (s *Service) SetStatusForInvoice(ctx context.Context, tenantID, invoiceID int64, status string) error {
	return s.repo.SetStatusForInvoice(ctx, tenantID, invoiceID, status)
}

// ClearForInvoice reverts every shift linked to an invoice back to 'recorded'
// with a NULL invoice_id (invoice delete). It satisfies invoice.ShiftLinker.
func (s *Service) ClearForInvoice(ctx context.Context, tenantID, invoiceID int64) error {
	return s.repo.ClearForInvoice(ctx, tenantID, invoiceID)
}

// ListItems returns a shift's line items (billed + unbilled).
func (s *Service) ListItems(ctx context.Context, shiftID int64) ([]*billing.LineItem, error) {
	tenantID := reqctx.MustTenant(ctx)
	return s.repo.ListItems(ctx, tenantID, shiftID)
}

// AddItem prices then inserts one item on a shift (invoice_id NULL), then
// broadcasts. Returns (nil, nil) when the shift is absent. A blank ServiceDate
// defaults to the shift's date so pricing keys off the right catalogue.
func (s *Service) AddItem(ctx context.Context, shiftID int64, in billing.LineItemInput) (*billing.LineItem, error) {
	tenantID := reqctx.MustTenant(ctx)
	sh, err := s.repo.Get(ctx, tenantID, shiftID)
	if err != nil {
		return nil, err
	}
	if sh == nil {
		return nil, nil
	}
	if in.ServiceDate == "" {
		in.ServiceDate = sh.ServiceDate
	}
	priced, err := s.priceItem(ctx, tenantID, sh.ParticipantID, in)
	if err != nil {
		return nil, err
	}
	item, err := s.repo.CreateItem(ctx, tenantID, shiftID, priced)
	if err != nil {
		return nil, err
	}
	s.hub.Broadcast(realtime.Event{TenantID: tenantID, Entity: "shift", ID: shiftID, Action: "update"})
	return item, nil
}

// UpdateItem prices then rewrites one UNBILLED item, then broadcasts. Returns
// (nil, nil) when the shift or item is absent (or the item is already billed).
func (s *Service) UpdateItem(ctx context.Context, shiftID, itemID int64, in billing.LineItemInput) (*billing.LineItem, error) {
	tenantID := reqctx.MustTenant(ctx)
	sh, err := s.repo.Get(ctx, tenantID, shiftID)
	if err != nil {
		return nil, err
	}
	if sh == nil {
		return nil, nil
	}
	if in.ServiceDate == "" {
		in.ServiceDate = sh.ServiceDate
	}
	priced, err := s.priceItem(ctx, tenantID, sh.ParticipantID, in)
	if err != nil {
		return nil, err
	}
	item, err := s.repo.UpdateItem(ctx, tenantID, itemID, priced)
	if err != nil {
		return nil, err
	}
	if item == nil {
		return nil, nil
	}
	s.hub.Broadcast(realtime.Event{TenantID: tenantID, Entity: "shift", ID: shiftID, Action: "update"})
	return item, nil
}

// DeleteItem removes one UNBILLED item, then broadcasts.
func (s *Service) DeleteItem(ctx context.Context, shiftID, itemID int64) error {
	tenantID := reqctx.MustTenant(ctx)
	if err := s.repo.DeleteItem(ctx, tenantID, itemID); err != nil {
		return err
	}
	s.hub.Broadcast(realtime.Event{TenantID: tenantID, Entity: "shift", ID: shiftID, Action: "update"})
	return nil
}

// ClearUnbilledItems removes all of a shift's unbilled items (used to make a
// re-divide idempotent). Broadcasts on success.
func (s *Service) ClearUnbilledItems(ctx context.Context, shiftID int64) error {
	tenantID := reqctx.MustTenant(ctx)
	if err := s.repo.DeleteUnbilledItems(ctx, tenantID, shiftID); err != nil {
		return err
	}
	s.hub.Broadcast(realtime.Event{TenantID: tenantID, Entity: "shift", ID: shiftID, Action: "update"})
	return nil
}

// priceItem resolves catalogue-authoritative pricing for one input line via the
// shared LineValidator (G3: pinned by ServiceDate). Returns the normalised,
// priced line.
func (s *Service) priceItem(ctx context.Context, tenantID, participantID int64, in billing.LineItemInput) (billing.LineItemInput, error) {
	res, err := s.validator.ValidateFilling(ctx, tenantID, participantID, []billing.LineItemInput{in})
	if err != nil {
		return billing.LineItemInput{}, fmt.Errorf("price item: %w", err)
	}
	if len(res.Items) != 1 {
		return billing.LineItemInput{}, fmt.Errorf("price item: expected 1 priced line, got %d", len(res.Items))
	}
	return res.Items[0], nil
}

// itemToInput projects a stored line item back to its writable input shape.
func itemToInput(it *billing.LineItem) billing.LineItemInput {
	return billing.LineItemInput{
		SupportItemID:    it.SupportItemID,
		CustomItemID:     it.CustomItemID,
		CatalogVersionID: it.CatalogVersionID,
		Code:             it.Code,
		Description:      it.Description,
		ServiceDate:      it.ServiceDate,
		Unit:             it.Unit,
		StartTime:        it.StartTime,
		EndTime:          it.EndTime,
		Quantity:         it.Quantity,
		UnitPrice:        it.UnitPrice,
		GstFree:          it.GstFree,
		SortOrder:        it.SortOrder,
	}
}
