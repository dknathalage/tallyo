package invoice

import (
	"context"
	"github.com/dknathalage/tallyo/internal/db"

	"github.com/dknathalage/tallyo/internal/billing"
	"github.com/dknathalage/tallyo/internal/listquery"
	"github.com/dknathalage/tallyo/internal/realtime"
	"github.com/dknathalage/tallyo/internal/reqctx"
)

// SessionLinker is the narrow interface the invoice service requires to cascade
// status changes to linked sessions. It breaks the invoice↔session import cycle:
// the invoice package declares this interface; the caller (main.go) injects a
// concrete *repository.SessionsRepo which satisfies it.
type SessionLinker interface {
	SetStatusForInvoice(ctx context.Context, tenantID, invoiceID int64, status string) error
	ClearForInvoice(ctx context.Context, tenantID, invoiceID int64) error
	// MarkDrafted links the given recorded sessions to invoiceID and advances them
	// to status 'drafted'. Called by DraftFromSessions AFTER the invoice + its
	// linked lines are committed, so the session→invoice reference is satisfiable.
	MarkDrafted(ctx context.Context, invoiceID int64, sessionIDs []int64) error
}

// Service orchestrates invoice reads/writes and publishes change events
// after a successful commit. Line items pass through the line validation engine
// (validator) on create/update before reaching the repository.
type Service struct {
	repo      *InvoicesRepo
	sessions  SessionLinker
	validator *billing.LineValidator
	hub       *realtime.Hub
}

// NewService constructs the invoice service. A nil hub is a programmer error.
// sessions may be nil (session cascade is skipped when nil).
func NewService(db db.Executor, hub *realtime.Hub, sessions SessionLinker) *Service {
	if hub == nil {
		panic("invoice.NewService: nil hub")
	}
	return &Service{
		repo:      NewInvoices(db),
		sessions:  sessions,
		validator: billing.NewLineValidator(db),
		hub:       hub,
	}
}

func (s *Service) List(ctx context.Context) ([]*Invoice, error) {
	tenantID := reqctx.MustTenant(ctx)
	return s.repo.List(ctx, tenantID)
}

// Query returns a page of invoices for the given listquery clause. Rows is
// non-nil so it serializes as [] not null.
func (s *Service) Query(ctx context.Context, c listquery.Clause) (listquery.Result[*Invoice], error) {
	tenantID := reqctx.MustTenant(ctx)
	rows, total, err := s.repo.Query(ctx, tenantID, c)
	if err != nil {
		return listquery.Result[*Invoice]{}, err
	}
	if rows == nil {
		rows = []*Invoice{}
	}
	return listquery.Result[*Invoice]{Rows: rows, Total: total}, nil
}

func (s *Service) ListByStatus(ctx context.Context, status string) ([]*Invoice, error) {
	tenantID := reqctx.MustTenant(ctx)
	return s.repo.ListByStatus(ctx, tenantID, status)
}

func (s *Service) ListClientInvoices(ctx context.Context, clientID int64) ([]*Invoice, error) {
	tenantID := reqctx.MustTenant(ctx)
	return s.repo.ListClientInvoices(ctx, tenantID, clientID)
}

func (s *Service) Get(ctx context.Context, id int64) (*Invoice, error) {
	tenantID := reqctx.MustTenant(ctx)
	return s.repo.Get(ctx, tenantID, id)
}

// GetByUUID returns an invoice by uuid, or (nil, nil) when absent. Public HTTP read.
func (s *Service) GetByUUID(ctx context.Context, invoiceUUID string) (*Invoice, error) {
	tenantID := reqctx.MustTenant(ctx)
	return s.repo.GetByUUID(ctx, tenantID, invoiceUUID)
}

// ResolveClient translates a client uuid into its int FK for the
// tenant. Returns (0, nil) when no client matches (caller 400s).
func (s *Service) ResolveClient(ctx context.Context, clientUUID string) (int64, error) {
	tenantID := reqctx.MustTenant(ctx)
	return s.repo.ResolveClientID(ctx, tenantID, clientUUID)
}

// ResolvePayer translates a payer uuid into its int FK for the
// tenant. Returns (0, nil) when no payer matches (caller 400s).
func (s *Service) ResolvePayer(ctx context.Context, payerUUID string) (int64, error) {
	tenantID := reqctx.MustTenant(ctx)
	return s.repo.ResolvePayerID(ctx, tenantID, payerUUID)
}

// ResolveSessionIDs translates a list of session uuids into their int PKs for the
// tenant (preserving order). An unknown uuid surfaces as an error so the caller
// can 400 — draft-from-sessions must not silently drop a session.
func (s *Service) ResolveSessionIDs(ctx context.Context, sessionUUIDs []string) ([]int64, error) {
	tenantID := reqctx.MustTenant(ctx)
	return s.repo.ResolveSessionIDs(ctx, tenantID, sessionUUIDs)
}

// ResolveInvoiceIDs translates a list of invoice uuids into their int PKs for
// the tenant (preserving order). An unknown uuid surfaces as an error so the
// caller can 400 — bulk operations must not silently drop a member.
func (s *Service) ResolveInvoiceIDs(ctx context.Context, invoiceUUIDs []string) ([]int64, error) {
	tenantID := reqctx.MustTenant(ctx)
	return s.repo.ResolveInvoiceIDs(ctx, tenantID, invoiceUUIDs)
}

// ClientStats resolves the client uuid to its int PK (tenant-scoped)
// then aggregates that client's invoices. Returns (nil, nil) when no
// client matches the uuid so the handler can 404.
func (s *Service) ClientStats(ctx context.Context, clientUUID string) (*ClientStats, error) {
	tenantID := reqctx.MustTenant(ctx)
	clientID, err := s.repo.ResolveClientID(ctx, tenantID, clientUUID)
	if err != nil {
		return nil, err
	}
	if clientID == 0 {
		return nil, nil
	}
	return s.repo.ClientStats(ctx, tenantID, clientID)
}

// Create inserts an invoice + line items, then broadcasts on success.
//
// Every line passes through the line validation engine (price-cap, plan-window,
// taxable resolution, snapshotting) first; tax is COMPUTED from the validated
// lines and overrides any client-supplied value (see validation.go tax note).
// A validation failure returns a *ValidationError with field-level detail.
func (s *Service) Create(ctx context.Context, in InvoiceInput, items []billing.LineItemInput) (*Invoice, error) {
	tenantID := reqctx.MustTenant(ctx)
	res, err := s.validator.Validate(ctx, tenantID, in.ClientID, items)
	if err != nil {
		return nil, err
	}
	in.Tax = res.Tax
	inv, err := s.repo.Create(ctx, tenantID, in, res.Items)
	if err != nil {
		return nil, err
	}
	s.hub.Broadcast(realtime.Event{TenantID: tenantID, Entity: "invoice", UUID: inv.UUID, Action: "create"})
	return inv, nil
}

// CreateWithCatalogPricing is Create in catalogue-authoritative pricing mode:
// every support-item line's unit price is resolved from the catalogue (the
// tenant-zone cap) rather than trusted from the caller. Used by the AI agent's
// create_invoice tool so the model owns only the code, service date and
// quantity — never the price. A quotable item with no published cap still
// requires a caller-supplied price (a *ValidationError otherwise).
func (s *Service) CreateWithCatalogPricing(ctx context.Context, in InvoiceInput, items []billing.LineItemInput) (*Invoice, error) {
	tenantID := reqctx.MustTenant(ctx)
	res, err := s.validator.ValidateFilling(ctx, tenantID, in.ClientID, items)
	if err != nil {
		return nil, err
	}
	in.Tax = res.Tax
	inv, err := s.repo.Create(ctx, tenantID, in, res.Items)
	if err != nil {
		return nil, err
	}
	s.hub.Broadcast(realtime.Event{TenantID: tenantID, Entity: "invoice", UUID: inv.UUID, Action: "create"})
	return inv, nil
}

// DraftFromSessions drafts a new invoice from N recorded, unbilled sessions — pure
// deterministic linking, no model, no re-pricing (the items are already priced
// on each session). Sessions must share one client and each carry at least one
// item (G5). The invoice and its linked lines commit atomically; only AFTER that
// commit are the sessions advanced to 'drafted' (via the SessionLinker, a separate
// tx), so the session→invoice reference and MarkDrafted's existence check hold.
func (s *Service) DraftFromSessions(ctx context.Context, sessionIDs []int64) (*Invoice, error) {
	tenantID := reqctx.MustTenant(ctx)
	clientID, facts, err := s.repo.validateDraftSessions(ctx, tenantID, sessionIDs)
	if err != nil {
		return nil, err
	}
	inv, err := s.repo.DraftFromSessions(ctx, tenantID, clientID, facts)
	if err != nil {
		return nil, err
	}
	if s.sessions != nil {
		if err := s.sessions.MarkDrafted(ctx, inv.ID, sessionIDs); err != nil {
			return nil, err
		}
	}
	s.hub.Broadcast(realtime.Event{TenantID: tenantID, Entity: "invoice", UUID: inv.UUID, Action: "create"})
	s.hub.Broadcast(realtime.Event{TenantID: tenantID, Entity: "session", UUID: "", Action: "bill"})
	return inv, nil
}

// Update rewrites an invoice. A nil result means the row was not found, in which
// case no event is published.
func (s *Service) Update(ctx context.Context, id int64, in InvoiceInput, items []billing.LineItemInput) (*Invoice, error) {
	tenantID := reqctx.MustTenant(ctx)
	res, err := s.validator.Validate(ctx, tenantID, in.ClientID, items)
	if err != nil {
		return nil, err
	}
	in.Tax = res.Tax
	inv, err := s.repo.Update(ctx, tenantID, id, in, res.Items)
	if err != nil {
		return nil, err
	}
	if inv == nil {
		return nil, nil
	}
	s.hub.Broadcast(realtime.Event{TenantID: tenantID, Entity: "invoice", UUID: inv.UUID, Action: "update"})
	return inv, nil
}

// UpdateByUUID resolves the invoice uuid → int PK, then rewrites the invoice.
// Returns (nil, nil) when no invoice matches the uuid so the handler can 404.
func (s *Service) UpdateByUUID(ctx context.Context, invoiceUUID string, in InvoiceInput, items []billing.LineItemInput) (*Invoice, error) {
	tenantID := reqctx.MustTenant(ctx)
	id, err := s.repo.ResolveInvoiceID(ctx, tenantID, invoiceUUID)
	if err != nil {
		return nil, err
	}
	if id == 0 {
		return nil, nil
	}
	return s.Update(ctx, id, in, items)
}

// DeleteByUUID resolves the invoice uuid → int PK, then deletes the invoice.
// A no-match uuid is a no-op (the int Delete is idempotent).
func (s *Service) DeleteByUUID(ctx context.Context, invoiceUUID string) error {
	tenantID := reqctx.MustTenant(ctx)
	id, err := s.repo.ResolveInvoiceID(ctx, tenantID, invoiceUUID)
	if err != nil {
		return err
	}
	if id == 0 {
		return nil
	}
	return s.Delete(ctx, id)
}

// UpdateStatusByUUID resolves the invoice uuid → int PK, then flips its status.
// A no-match uuid is a no-op.
func (s *Service) UpdateStatusByUUID(ctx context.Context, invoiceUUID, status string) error {
	tenantID := reqctx.MustTenant(ctx)
	id, err := s.repo.ResolveInvoiceID(ctx, tenantID, invoiceUUID)
	if err != nil {
		return err
	}
	if id == 0 {
		return nil
	}
	return s.UpdateStatus(ctx, id, status)
}

// UpdateStatus sets the invoice status, then broadcasts on success. When the
// invoice advances to a terminal billing status ('sent'/'paid'), the sessions
// attached to it advance in lockstep (recorded→drafted→sent→paid lifecycle).
func (s *Service) UpdateStatus(ctx context.Context, id int64, status string) error {
	tenantID := reqctx.MustTenant(ctx)
	inv, err := s.repo.Get(ctx, tenantID, id)
	if err != nil {
		return err
	}
	if inv == nil {
		return nil
	}
	if err := s.repo.UpdateStatus(ctx, tenantID, id, status); err != nil {
		return err
	}
	cascade := status == "sent" || status == "paid"
	if cascade && s.sessions != nil {
		if err := s.sessions.SetStatusForInvoice(ctx, tenantID, id, status); err != nil {
			return err
		}
	}
	s.hub.Broadcast(realtime.Event{TenantID: tenantID, Entity: "invoice", UUID: inv.UUID, Action: "status"})
	if cascade {
		s.hub.Broadcast(realtime.Event{TenantID: tenantID, Entity: "session", UUID: "", Action: "status"})
	}
	return nil
}

// Delete removes an invoice, then broadcasts on success. Before deleting, any
// sessions attached to the invoice are reverted to 'recorded' with a NULL
// invoice_id, so the work returns to the unbilled pool rather than being orphaned
// at status 'drafted' by the FK's ON DELETE SET NULL.
func (s *Service) Delete(ctx context.Context, id int64) error {
	tenantID := reqctx.MustTenant(ctx)
	inv, err := s.repo.Get(ctx, tenantID, id)
	if err != nil {
		return err
	}
	if inv == nil {
		return nil
	}
	if s.sessions != nil {
		if err := s.sessions.ClearForInvoice(ctx, tenantID, id); err != nil {
			return err
		}
	}
	if err := s.repo.Delete(ctx, tenantID, id); err != nil {
		return err
	}
	s.hub.Broadcast(realtime.Event{TenantID: tenantID, Entity: "invoice", UUID: inv.UUID, Action: "delete"})
	s.hub.Broadcast(realtime.Event{TenantID: tenantID, Entity: "session", UUID: "", Action: "status"})
	return nil
}

// BulkDelete removes several invoices, then broadcasts a single bulk event.
// Like Delete, each invoice's sessions are first reverted to 'recorded' (NULL
// invoice_id) so bulk-deleted work returns to the unbilled pool rather than
// being orphaned at status 'drafted'.
func (s *Service) BulkDelete(ctx context.Context, ids []int64) error {
	tenantID := reqctx.MustTenant(ctx)
	if s.sessions != nil {
		for i := range ids { // bounded by len(ids)
			if err := s.sessions.ClearForInvoice(ctx, tenantID, ids[i]); err != nil {
				return err
			}
		}
	}
	if err := s.repo.BulkDelete(ctx, tenantID, ids); err != nil {
		return err
	}
	s.hub.Broadcast(realtime.Event{TenantID: tenantID, Entity: "invoice", UUID: "", Action: "bulk_delete"})
	return nil
}

// BulkUpdateStatus sets several invoices' status, then broadcasts a bulk event.
func (s *Service) BulkUpdateStatus(ctx context.Context, ids []int64, status string) error {
	tenantID := reqctx.MustTenant(ctx)
	if err := s.repo.BulkUpdateStatus(ctx, tenantID, ids, status); err != nil {
		return err
	}
	s.hub.Broadcast(realtime.Event{TenantID: tenantID, Entity: "invoice", UUID: "", Action: "bulk_status"})
	return nil
}

// ActiveTenantIDs returns the ids of active (non-suspended) tenants. The sweep
// driver uses it to iterate tenants and skip suspended ones (spec §8).
func (s *Service) ActiveTenantIDs(ctx context.Context) ([]int64, error) {
	return s.repo.ActiveTenantIDs(ctx)
}

// MarkOverdueForTenant flips overdue invoices for ONE tenant (the per-tenant
// sweep path) and, when any flipped, broadcasts a sweep event scoped to that
// tenant so only its subscribers resync. ctx must carry the tenant (the sweep
// driver attaches it via reqctx.WithTenant).
func (s *Service) MarkOverdueForTenant(ctx context.Context, tenantID int64) ([]OverdueInvoice, error) {
	rows, err := s.repo.MarkOverdueForTenant(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	if len(rows) > 0 {
		s.hub.Broadcast(realtime.Event{TenantID: tenantID, Entity: "invoice", UUID: "", Action: "overdue_sweep"})
	}
	return rows, nil
}
