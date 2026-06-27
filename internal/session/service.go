package session

import (
	"context"
	"fmt"

	"github.com/dknathalage/tallyo/internal/apperr"
	"github.com/dknathalage/tallyo/internal/billing"
	"github.com/dknathalage/tallyo/internal/db"
	"github.com/dknathalage/tallyo/internal/reqctx"
)

// ErrSessionBilled is returned when an operation is refused because the session's
// items are already on an invoice (status past 'recorded'). It wraps
// apperr.ErrConflict so httpx.WriteServiceError maps it to a 409, while callers
// (and tests) can still match it with errors.Is(err, ErrSessionBilled).
var ErrSessionBilled = fmt.Errorf("session: cannot delete a billed session: %w", apperr.ErrConflict)

// InvoiceChecker is the narrow interface the session service requires to verify
// that an invoice exists before linking sessions to it. It breaks the
// session→invoice import cycle: the session package declares this interface; the
// caller (main.go) injects a concrete *invoice.InvoicesRepo which satisfies it.
type InvoiceChecker interface {
	Exists(ctx context.Context, tenantID, invoiceID string) (bool, error)
}

// Service orchestrates the session lifecycle (record→draft→bill). It resolves
// the caller's tenant (and, for authorship, user) from the request context.
type Service struct {
	repo      *SessionsRepo
	invoices  InvoiceChecker
	validator *billing.LineValidator
}

// NewService constructs the session service.
// invoices is the InvoiceChecker used to verify the invoice in MarkDrafted. The
// session service builds its own billing.LineValidator (catalogue unit_price
// pricing) from the same db the invoice service uses — no extra wiring needed.
func NewService(db db.Executor, invoices InvoiceChecker) *Service {
	return &Service{
		repo:      NewSessions(db),
		invoices:  invoices,
		validator: billing.NewLineValidator(db),
	}
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

// Create inserts a session attributed to the authenticated user.
func (s *Service) Create(ctx context.Context, in SessionInput) (*Session, error) {
	if err := in.Validate(); err != nil {
		return nil, err
	}
	tenantID := reqctx.MustTenant(ctx)
	var author *string
	if uid, ok := reqctx.UserFrom(ctx); ok {
		author = &uid
	}
	sh, err := s.repo.Create(ctx, tenantID, author, in)
	if err != nil {
		return nil, err
	}
	return sh, nil
}

// Update mutates a session. A nil result means the row was not found. When the
// service date changes, the session's UNBILLED items are re-stamped to the new
// date and re-priced against that date's catalogue (G3/G4).
func (s *Service) Update(ctx context.Context, sessionUUID string, in SessionInput) (*Session, error) {
	if err := in.Validate(); err != nil {
		return nil, err
	}
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

// UpdateStatus advances a session's lifecycle status by uuid.
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
	return nil
}

// Delete removes a session by uuid (its items cascade). A billed session —
// status past 'recorded' (drafted/sent/paid) — cannot be deleted: its items live
// on an invoice. Returns ErrSessionBilled in that case.
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

// MarkDrafted links the given recorded sessions to an invoice (status 'drafted').
// An empty id list is a no-op. The invoice MUST belong to the caller's tenant —
// verified tenant-scoped first to prevent cross-tenant linkage.
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
