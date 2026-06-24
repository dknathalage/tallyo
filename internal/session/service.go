package session

import (
	"context"
	"errors"
	"fmt"
	"github.com/dknathalage/tallyo/internal/db"

	"github.com/dknathalage/tallyo/internal/billing"
	"github.com/dknathalage/tallyo/internal/realtime"
	"github.com/dknathalage/tallyo/internal/reqctx"
)

// ErrSessionBilled is returned when an operation is refused because the session's
// items are already on an invoice (status past 'recorded').
var ErrSessionBilled = errors.New("session: cannot delete a billed session")

// InvoiceChecker is the narrow interface the session service requires to verify
// that an invoice exists before linking sessions to it. It breaks the
// session→invoice import cycle: the session package declares this interface; the
// caller (main.go) injects a concrete *invoice.InvoicesRepo which satisfies it.
type InvoiceChecker interface {
	Exists(ctx context.Context, tenantID, invoiceID string) (bool, error)
}

// Service orchestrates the session lifecycle (record→draft→bill) and
// publishes change events after a successful commit. It resolves the caller's
// tenant (and, for authorship, user) from the request context.
type Service struct {
	repo      *SessionsRepo
	invoices  InvoiceChecker
	validator *billing.LineValidator
	hub       *realtime.Hub
}

// NewService constructs the session service. A nil hub is a programmer error.
// invoices is the InvoiceChecker used to verify the invoice in MarkDrafted. The
// session service builds its own billing.LineValidator (catalogue unit_price
// pricing) from the same db the invoice service uses — no extra wiring needed.
func NewService(db db.Executor, hub *realtime.Hub, invoices InvoiceChecker) *Service {
	if hub == nil {
		panic("session.NewService: nil hub")
	}
	return &Service{repo: NewSessions(db), invoices: invoices, validator: billing.NewLineValidator(db), hub: hub}
}

// ListUnbilledForClient returns a client's recorded-but-unbilled sessions for
// the given tenant. Used by the draft-invoice Smart to gather billable work.
func (s *Service) ListUnbilledForClient(ctx context.Context, tenantID, clientID string) ([]*Session, error) {
	return s.repo.ListRecordedUnbilled(ctx, tenantID, clientID)
}

// Suggestion is a billing prompt: a client's recorded-but-unbilled sessions
// grouped together, ready to draft onto a single invoice.
type Suggestion struct {
	ClientID string   `json:"clientId"`
	IDs      []string `json:"ids"`
	From     string   `json:"from"`
	To       string   `json:"to"`
	Count    int      `json:"count"`
}

// ListClient returns a client's sessions, optionally restricted to the
// [from, to] service-date window (both empty → all sessions).
func (s *Service) ListClient(ctx context.Context, clientID string, from, to string) ([]*Session, error) {
	tenantID := reqctx.MustTenant(ctx)
	return s.repo.ListClient(ctx, tenantID, clientID, from, to)
}

// List returns all the tenant's sessions. When status is non-empty the result is
// restricted to sessions in that lifecycle status.
func (s *Service) List(ctx context.Context, status string) ([]*Session, error) {
	tenantID := reqctx.MustTenant(ctx)
	if status != "" {
		return s.repo.ListByStatus(ctx, tenantID, status)
	}
	return s.repo.List(ctx, tenantID)
}

// Get returns a session by row id, or (nil, nil) when absent. This is the
// internal/cross-slice read (agent SessionReader, the service's own pricing path);
// the public HTTP path addresses sessions by uuid via GetByUUID.
func (s *Service) Get(ctx context.Context, id string) (*Session, error) {
	tenantID := reqctx.MustTenant(ctx)
	return s.repo.Get(ctx, tenantID, id)
}

// GetByUUID returns a session by uuid, or (nil, nil) when absent. Public HTTP read.
func (s *Service) GetByUUID(ctx context.Context, sessionUUID string) (*Session, error) {
	tenantID := reqctx.MustTenant(ctx)
	return s.repo.GetByUUID(ctx, tenantID, sessionUUID)
}

// ResolveClient resolves a client uuid to its row id (uuid) for the
// tenant (inbound clientId resolution on session create/update). Returns
// ("", nil) when the uuid is unknown so the handler can 400.
func (s *Service) ResolveClient(ctx context.Context, clientUUID string) (string, error) {
	tenantID := reqctx.MustTenant(ctx)
	return s.repo.ResolveClientID(ctx, tenantID, clientUUID)
}

// ListByClientUUID returns the tenant's sessions for one client,
// resolving the client uuid to its row id (uuid). An unknown client uuid
// yields an empty (non-nil) slice — the filter simply matches nothing.
func (s *Service) ListByClientUUID(ctx context.Context, clientUUID, status string) ([]*Session, error) {
	tenantID := reqctx.MustTenant(ctx)
	pid, err := s.repo.ResolveClientID(ctx, tenantID, clientUUID)
	if err != nil {
		return nil, err
	}
	if pid == "" {
		return []*Session{}, nil
	}
	sessions, err := s.repo.ListClient(ctx, tenantID, pid, "", "")
	if err != nil {
		return nil, err
	}
	if status == "" {
		return sessions, nil
	}
	filtered := make([]*Session, 0, len(sessions))
	for i := range sessions { // bounded by len(sessions)
		if sessions[i].Status == status {
			filtered = append(filtered, sessions[i])
		}
	}
	return filtered, nil
}

// Create inserts a session attributed to the authenticated user, then broadcasts
// after the commit succeeds.
func (s *Service) Create(ctx context.Context, in SessionInput) (*Session, error) {
	tenantID := reqctx.MustTenant(ctx)
	var author *string
	if uid, ok := reqctx.UserFrom(ctx); ok {
		author = &uid
	}
	sh, err := s.repo.Create(ctx, tenantID, author, in)
	if err != nil {
		return nil, err
	}
	s.hub.Broadcast(realtime.Event{TenantID: tenantID, Entity: "session", UUID: sh.ID, Action: "create"})
	return sh, nil
}

// Update mutates a session, then broadcasts on success. A nil result means the row
// was not found, in which case no event is published. When the service date
// changes, the session's UNBILLED items are re-stamped to the new date and
// re-priced against that date's catalogue (G3/G4).
func (s *Service) Update(ctx context.Context, sessionUUID string, in SessionInput) (*Session, error) {
	tenantID := reqctx.MustTenant(ctx)
	prev, err := s.repo.GetByUUID(ctx, tenantID, sessionUUID)
	if err != nil {
		return nil, err
	}
	sh, err := s.repo.Update(ctx, tenantID, sessionUUID, in)
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
	s.hub.Broadcast(realtime.Event{TenantID: tenantID, Entity: "session", UUID: sh.ID, Action: "update"})
	return sh, nil
}

// repriceItemsForDate re-stamps every unbilled item of the session to the session's
// (new) service date and re-prices it against that date's catalogue. Bounded by
// the number of items on the session.
func (s *Service) repriceItemsForDate(ctx context.Context, tenantID string, sh *Session) error {
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
		priced, err := s.priceItem(ctx, tenantID, sh.ClientID, in)
		if err != nil {
			return err
		}
		if _, err := s.repo.UpdateItem(ctx, tenantID, it.ID, priced); err != nil {
			return err
		}
	}
	return nil
}

// UpdateStatus advances a session's lifecycle status by uuid, then broadcasts on
// success. The SSE event carries the row's id (uuid), resolved first.
func (s *Service) UpdateStatus(ctx context.Context, sessionUUID, status string) error {
	tenantID := reqctx.MustTenant(ctx)
	sh, err := s.repo.GetByUUID(ctx, tenantID, sessionUUID)
	if err != nil {
		return err
	}
	if sh == nil {
		return nil
	}
	if err := s.repo.UpdateStatus(ctx, tenantID, sessionUUID, status); err != nil {
		return err
	}
	s.hub.Broadcast(realtime.Event{TenantID: tenantID, Entity: "session", UUID: sh.ID, Action: "update"})
	return nil
}

// Delete removes a session by uuid (its items cascade), then broadcasts on success.
// A billed session — status past 'recorded' (drafted/sent/paid) — cannot be
// deleted: its items live on an invoice. Returns ErrSessionBilled in that case.
// The SSE event carries the row's id (uuid), resolved first.
func (s *Service) Delete(ctx context.Context, sessionUUID string) error {
	tenantID := reqctx.MustTenant(ctx)
	sh, err := s.repo.GetByUUID(ctx, tenantID, sessionUUID)
	if err != nil {
		return err
	}
	if sh == nil {
		return nil
	}
	if sh.Status != "scheduled" && sh.Status != "recorded" {
		return ErrSessionBilled
	}
	if err := s.repo.Delete(ctx, tenantID, sessionUUID); err != nil {
		return err
	}
	s.hub.Broadcast(realtime.Event{TenantID: tenantID, Entity: "session", UUID: sh.ID, Action: "delete"})
	return nil
}

// ToRecord returns the tenant's scheduled sessions still awaiting a record.
func (s *Service) ToRecord(ctx context.Context) ([]*Session, error) {
	tenantID := reqctx.MustTenant(ctx)
	return s.repo.ListScheduled(ctx, tenantID)
}

// Suggestions groups each client's recorded-but-unbilled sessions into a
// billing prompt, resolving the concrete session ids per client.
func (s *Service) Suggestions(ctx context.Context) ([]Suggestion, error) {
	tenantID := reqctx.MustTenant(ctx)
	aggs, err := s.repo.UnbilledByClient(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	out := make([]Suggestion, 0, len(aggs))
	for i := range aggs { // bounded by len(aggs)
		sessions, e := s.repo.ListRecordedUnbilled(ctx, tenantID, aggs[i].ClientID)
		if e != nil {
			return nil, e
		}
		ids := make([]string, 0, len(sessions))
		for j := range sessions { // bounded by len(sessions)
			ids = append(ids, sessions[j].ID)
		}
		out = append(out, Suggestion{
			ClientID: aggs[i].ClientID,
			IDs:      ids,
			From:     aggs[i].From,
			To:       aggs[i].To,
			Count:    int(aggs[i].Count),
		})
	}
	return out, nil
}

// MarkDrafted links the given recorded sessions to an invoice (status 'drafted'),
// then broadcasts a single bulk event. An empty id list is a no-op. The invoice
// MUST belong to the caller's tenant — verified tenant-scoped first to prevent
// cross-tenant linkage.
func (s *Service) MarkDrafted(ctx context.Context, invoiceID string, sessionIDs []string) error {
	tenantID := reqctx.MustTenant(ctx)
	if len(sessionIDs) == 0 {
		return nil
	}
	if invoiceID == "" {
		return fmt.Errorf("mark drafted: invoice id required")
	}
	exists, err := s.invoices.Exists(ctx, tenantID, invoiceID)
	if err != nil {
		return fmt.Errorf("mark drafted: verify invoice: %w", err)
	}
	if !exists {
		return fmt.Errorf("mark drafted: invoice %s not found for tenant", invoiceID)
	}
	for i := range sessionIDs { // bounded by len(sessionIDs)
		if err := s.repo.SetInvoice(ctx, tenantID, sessionIDs[i], invoiceID, "drafted"); err != nil {
			return err
		}
	}
	s.hub.Broadcast(realtime.Event{TenantID: tenantID, Entity: "session", UUID: "", Action: "bill"})
	return nil
}

// SetStatusForInvoice advances every session linked to an invoice to status (the
// invoice→session cascade on 'sent'/'paid'). It satisfies invoice.SessionLinker;
// tenantID is supplied by the caller (the invoice service's request scope).
func (s *Service) SetStatusForInvoice(ctx context.Context, tenantID, invoiceID string, status string) error {
	return s.repo.SetStatusForInvoice(ctx, tenantID, invoiceID, status)
}

// ClearForInvoice reverts every session linked to an invoice back to 'recorded'
// with a NULL invoice_id (invoice delete). It satisfies invoice.SessionLinker.
func (s *Service) ClearForInvoice(ctx context.Context, tenantID, invoiceID string) error {
	return s.repo.ClearForInvoice(ctx, tenantID, invoiceID)
}

// ListItems returns a session's line items (billed + unbilled).
func (s *Service) ListItems(ctx context.Context, sessionID string) ([]*billing.LineItem, error) {
	tenantID := reqctx.MustTenant(ctx)
	return s.repo.ListItems(ctx, tenantID, sessionID)
}

// AddItem prices then inserts one item on a session (invoice_id NULL), then
// broadcasts. Returns (nil, nil) when the session is absent. A blank ServiceDate
// defaults to the session's date so pricing keys off the right catalogue.
func (s *Service) AddItem(ctx context.Context, sessionID string, in billing.LineItemInput) (*billing.LineItem, error) {
	tenantID := reqctx.MustTenant(ctx)
	sh, err := s.repo.Get(ctx, tenantID, sessionID)
	if err != nil {
		return nil, err
	}
	if sh == nil {
		return nil, nil
	}
	if in.ServiceDate == "" {
		in.ServiceDate = sh.ServiceDate
	}
	priced, err := s.priceItem(ctx, tenantID, sh.ClientID, in)
	if err != nil {
		return nil, err
	}
	item, err := s.repo.CreateItem(ctx, tenantID, sessionID, priced)
	if err != nil {
		return nil, err
	}
	s.hub.Broadcast(realtime.Event{TenantID: tenantID, Entity: "session", UUID: sh.ID, Action: "update"})
	return item, nil
}

// resolveSession resolves a session uuid to its row id (uuid) for the tenant. Returns
// ("", nil) when no such session exists so HTTP item handlers can 404.
func (s *Service) resolveSession(ctx context.Context, tenantID string, sessionUUID string) (string, error) {
	return s.repo.ResolveID(ctx, tenantID, sessionUUID)
}

// ResolveSessionID resolves a session uuid to its row id (uuid) for the acting tenant.
// Returns ("", nil) when no such session exists (the Divide handler 404s). Exposed
// so the handler can bridge the uuid path to the DivideSession contract.
func (s *Service) ResolveSessionID(ctx context.Context, sessionUUID string) (string, error) {
	tenantID := reqctx.MustTenant(ctx)
	return s.repo.ResolveID(ctx, tenantID, sessionUUID)
}

// ListItemsBySessionUUID returns a session's line items, resolving the session uuid to
// its row id first. Returns (nil, nil) when the session is absent (handler 404s).
func (s *Service) ListItemsBySessionUUID(ctx context.Context, sessionUUID string) ([]*billing.LineItem, error) {
	tenantID := reqctx.MustTenant(ctx)
	sessionID, err := s.resolveSession(ctx, tenantID, sessionUUID)
	if err != nil {
		return nil, err
	}
	if sessionID == "" {
		return nil, nil
	}
	return s.repo.ListItems(ctx, tenantID, sessionID)
}

// AddItemBySessionUUID prices then inserts one item on the session named by uuid,
// then broadcasts. Returns (nil, nil) when the session is absent.
func (s *Service) AddItemBySessionUUID(ctx context.Context, sessionUUID string, in billing.LineItemInput) (*billing.LineItem, error) {
	tenantID := reqctx.MustTenant(ctx)
	sh, err := s.repo.GetByUUID(ctx, tenantID, sessionUUID)
	if err != nil {
		return nil, err
	}
	if sh == nil {
		return nil, nil
	}
	if in.ServiceDate == "" {
		in.ServiceDate = sh.ServiceDate
	}
	priced, err := s.priceItem(ctx, tenantID, sh.ClientID, in)
	if err != nil {
		return nil, err
	}
	item, err := s.repo.CreateItem(ctx, tenantID, sh.ID, priced)
	if err != nil {
		return nil, err
	}
	s.hub.Broadcast(realtime.Event{TenantID: tenantID, Entity: "session", UUID: sh.ID, Action: "update"})
	return item, nil
}

// UpdateItemBySessionUUID prices then rewrites one UNBILLED item addressed by uuid,
// scoped to the session named by uuid, then broadcasts. Returns (nil, nil) when the
// session or item is absent (or the item is already billed).
func (s *Service) UpdateItemBySessionUUID(ctx context.Context, sessionUUID, itemUUID string, in billing.LineItemInput) (*billing.LineItem, error) {
	tenantID := reqctx.MustTenant(ctx)
	sh, err := s.repo.GetByUUID(ctx, tenantID, sessionUUID)
	if err != nil {
		return nil, err
	}
	if sh == nil {
		return nil, nil
	}
	if in.ServiceDate == "" {
		in.ServiceDate = sh.ServiceDate
	}
	priced, err := s.priceItem(ctx, tenantID, sh.ClientID, in)
	if err != nil {
		return nil, err
	}
	item, err := s.repo.UpdateItemByUUID(ctx, tenantID, sh.ID, itemUUID, priced)
	if err != nil {
		return nil, err
	}
	if item == nil {
		return nil, nil
	}
	s.hub.Broadcast(realtime.Event{TenantID: tenantID, Entity: "session", UUID: sh.ID, Action: "update"})
	return item, nil
}

// DeleteItemBySessionUUID removes one UNBILLED item addressed by uuid, scoped to the
// session named by uuid, then broadcasts. A missing session is a no-op.
func (s *Service) DeleteItemBySessionUUID(ctx context.Context, sessionUUID, itemUUID string) error {
	tenantID := reqctx.MustTenant(ctx)
	sessionID, err := s.resolveSession(ctx, tenantID, sessionUUID)
	if err != nil {
		return err
	}
	if sessionID == "" {
		return nil
	}
	if err := s.repo.DeleteItemByUUID(ctx, tenantID, sessionID, itemUUID); err != nil {
		return err
	}
	// The event names the changed session; sessionUUID is its public id.
	s.hub.Broadcast(realtime.Event{TenantID: tenantID, Entity: "session", UUID: sessionUUID, Action: "update"})
	return nil
}

// ClearUnbilledItems removes all of a session's unbilled items (used to make a
// re-divide idempotent). Broadcasts on success. Resolves the session's uuid first
// so the post-commit event carries the public id (uuid).
func (s *Service) ClearUnbilledItems(ctx context.Context, sessionID string) error {
	tenantID := reqctx.MustTenant(ctx)
	sh, err := s.repo.Get(ctx, tenantID, sessionID)
	if err != nil {
		return err
	}
	if sh == nil {
		return nil
	}
	if err := s.repo.DeleteUnbilledItems(ctx, tenantID, sessionID); err != nil {
		return err
	}
	s.hub.Broadcast(realtime.Event{TenantID: tenantID, Entity: "session", UUID: sh.ID, Action: "update"})
	return nil
}

// priceItem resolves catalogue-authoritative pricing for one input line via the
// shared LineValidator (G3: pinned by ServiceDate). Returns the normalised,
// priced line.
func (s *Service) priceItem(ctx context.Context, tenantID, clientID string, in billing.LineItemInput) (billing.LineItemInput, error) {
	res, err := s.validator.ValidateFilling(ctx, tenantID, clientID, []billing.LineItemInput{in})
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
		ItemID:             it.ItemID,
		CustomItemID:       it.CustomItemUUID,
		PriceListVersionID: it.PriceListVersionID,
		Code:               it.Code,
		Description:        it.Description,
		ServiceDate:        it.ServiceDate,
		Unit:               it.Unit,
		StartTime:          it.StartTime,
		EndTime:            it.EndTime,
		Quantity:           it.Quantity,
		UnitPrice:          it.UnitPrice,
		Taxable:            it.Taxable,
		SortOrder:          it.SortOrder,
	}
}
