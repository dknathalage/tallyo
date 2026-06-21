package invoice

import (
	"context"
	"github.com/dknathalage/tallyo/internal/db"

	"github.com/dknathalage/tallyo/internal/billing"
	"github.com/dknathalage/tallyo/internal/listquery"
	"github.com/dknathalage/tallyo/internal/realtime"
	"github.com/dknathalage/tallyo/internal/reqctx"
)

// ShiftLinker is the narrow interface the invoice service requires to cascade
// status changes to linked shifts. It breaks the invoice↔shift import cycle:
// the invoice package declares this interface; the caller (main.go) injects a
// concrete *repository.ShiftsRepo which satisfies it.
type ShiftLinker interface {
	SetStatusForInvoice(ctx context.Context, tenantID, invoiceID int64, status string) error
	ClearForInvoice(ctx context.Context, tenantID, invoiceID int64) error
	// MarkDrafted links the given recorded shifts to invoiceID and advances them
	// to status 'drafted'. Called by DraftFromShifts AFTER the invoice + its
	// linked lines are committed, so the shift→invoice reference is satisfiable.
	MarkDrafted(ctx context.Context, invoiceID int64, shiftIDs []int64) error
}

// Service orchestrates invoice reads/writes and publishes change events
// after a successful commit. Line items pass through the NDIS validation engine
// (validator) on create/update before reaching the repository.
type Service struct {
	repo      *InvoicesRepo
	shifts    ShiftLinker
	validator *billing.LineValidator
	hub       *realtime.Hub
}

// NewService constructs the invoice service. A nil hub is a programmer error.
// shifts may be nil (shift cascade is skipped when nil).
func NewService(db, control db.Executor, hub *realtime.Hub, shifts ShiftLinker) *Service {
	if hub == nil {
		panic("invoice.NewService: nil hub")
	}
	return &Service{
		repo:      NewInvoices(db),
		shifts:    shifts,
		validator: billing.NewLineValidator(db, control),
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

func (s *Service) ListParticipantInvoices(ctx context.Context, participantID int64) ([]*Invoice, error) {
	tenantID := reqctx.MustTenant(ctx)
	return s.repo.ListParticipantInvoices(ctx, tenantID, participantID)
}

func (s *Service) Get(ctx context.Context, id int64) (*Invoice, error) {
	tenantID := reqctx.MustTenant(ctx)
	return s.repo.Get(ctx, tenantID, id)
}

func (s *Service) ParticipantStats(ctx context.Context, participantID int64) (*ParticipantStats, error) {
	tenantID := reqctx.MustTenant(ctx)
	return s.repo.ParticipantStats(ctx, tenantID, participantID)
}

// Create inserts an invoice + line items, then broadcasts on success.
//
// Every line passes through the NDIS validation engine (price-cap, plan-window,
// gst-free defaulting, snapshotting) first; tax is COMPUTED from the validated
// lines and overrides any client-supplied value (see validation.go tax note).
// A validation failure returns a *ValidationError with field-level detail.
func (s *Service) Create(ctx context.Context, in InvoiceInput, items []billing.LineItemInput) (*Invoice, error) {
	tenantID := reqctx.MustTenant(ctx)
	res, err := s.validator.Validate(ctx, tenantID, in.ParticipantID, items)
	if err != nil {
		return nil, err
	}
	in.Tax = res.Tax
	inv, err := s.repo.Create(ctx, tenantID, in, res.Items)
	if err != nil {
		return nil, err
	}
	s.hub.Broadcast(realtime.Event{TenantID: tenantID, Entity: "invoice", ID: inv.ID, Action: "create"})
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
	res, err := s.validator.ValidateFilling(ctx, tenantID, in.ParticipantID, items)
	if err != nil {
		return nil, err
	}
	in.Tax = res.Tax
	inv, err := s.repo.Create(ctx, tenantID, in, res.Items)
	if err != nil {
		return nil, err
	}
	s.hub.Broadcast(realtime.Event{TenantID: tenantID, Entity: "invoice", ID: inv.ID, Action: "create"})
	return inv, nil
}

// DraftFromShifts drafts a new invoice from N recorded, unbilled shifts — pure
// deterministic linking, no model, no re-pricing (the items are already priced
// on each shift). Shifts must share one participant and each carry at least one
// item (G5). The invoice and its linked lines commit atomically; only AFTER that
// commit are the shifts advanced to 'drafted' (via the ShiftLinker, a separate
// tx), so the shift→invoice reference and MarkDrafted's existence check hold.
func (s *Service) DraftFromShifts(ctx context.Context, shiftIDs []int64) (*Invoice, error) {
	tenantID := reqctx.MustTenant(ctx)
	participantID, facts, err := s.repo.validateDraftShifts(ctx, tenantID, shiftIDs)
	if err != nil {
		return nil, err
	}
	inv, err := s.repo.DraftFromShifts(ctx, tenantID, participantID, facts)
	if err != nil {
		return nil, err
	}
	if s.shifts != nil {
		if err := s.shifts.MarkDrafted(ctx, inv.ID, shiftIDs); err != nil {
			return nil, err
		}
	}
	s.hub.Broadcast(realtime.Event{TenantID: tenantID, Entity: "invoice", ID: inv.ID, Action: "create"})
	s.hub.Broadcast(realtime.Event{TenantID: tenantID, Entity: "shift", ID: 0, Action: "bill"})
	return inv, nil
}

// Update rewrites an invoice. A nil result means the row was not found, in which
// case no event is published.
func (s *Service) Update(ctx context.Context, id int64, in InvoiceInput, items []billing.LineItemInput) (*Invoice, error) {
	tenantID := reqctx.MustTenant(ctx)
	res, err := s.validator.Validate(ctx, tenantID, in.ParticipantID, items)
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
	s.hub.Broadcast(realtime.Event{TenantID: tenantID, Entity: "invoice", ID: id, Action: "update"})
	return inv, nil
}

// UpdateStatus sets the invoice status, then broadcasts on success. When the
// invoice advances to a terminal billing status ('sent'/'paid'), the shifts
// attached to it advance in lockstep (recorded→drafted→sent→paid lifecycle).
func (s *Service) UpdateStatus(ctx context.Context, id int64, status string) error {
	tenantID := reqctx.MustTenant(ctx)
	if err := s.repo.UpdateStatus(ctx, tenantID, id, status); err != nil {
		return err
	}
	cascade := status == "sent" || status == "paid"
	if cascade && s.shifts != nil {
		if err := s.shifts.SetStatusForInvoice(ctx, tenantID, id, status); err != nil {
			return err
		}
	}
	s.hub.Broadcast(realtime.Event{TenantID: tenantID, Entity: "invoice", ID: id, Action: "status"})
	if cascade {
		s.hub.Broadcast(realtime.Event{TenantID: tenantID, Entity: "shift", ID: 0, Action: "status"})
	}
	return nil
}

// Delete removes an invoice, then broadcasts on success. Before deleting, any
// shifts attached to the invoice are reverted to 'recorded' with a NULL
// invoice_id, so the work returns to the unbilled pool rather than being orphaned
// at status 'drafted' by the FK's ON DELETE SET NULL.
func (s *Service) Delete(ctx context.Context, id int64) error {
	tenantID := reqctx.MustTenant(ctx)
	if s.shifts != nil {
		if err := s.shifts.ClearForInvoice(ctx, tenantID, id); err != nil {
			return err
		}
	}
	if err := s.repo.Delete(ctx, tenantID, id); err != nil {
		return err
	}
	s.hub.Broadcast(realtime.Event{TenantID: tenantID, Entity: "invoice", ID: id, Action: "delete"})
	s.hub.Broadcast(realtime.Event{TenantID: tenantID, Entity: "shift", ID: 0, Action: "status"})
	return nil
}

// BulkDelete removes several invoices, then broadcasts a single bulk event.
// Like Delete, each invoice's shifts are first reverted to 'recorded' (NULL
// invoice_id) so bulk-deleted work returns to the unbilled pool rather than
// being orphaned at status 'drafted'.
func (s *Service) BulkDelete(ctx context.Context, ids []int64) error {
	tenantID := reqctx.MustTenant(ctx)
	if s.shifts != nil {
		for i := range ids { // bounded by len(ids)
			if err := s.shifts.ClearForInvoice(ctx, tenantID, ids[i]); err != nil {
				return err
			}
		}
	}
	if err := s.repo.BulkDelete(ctx, tenantID, ids); err != nil {
		return err
	}
	s.hub.Broadcast(realtime.Event{TenantID: tenantID, Entity: "invoice", ID: 0, Action: "bulk_delete"})
	return nil
}

// BulkUpdateStatus sets several invoices' status, then broadcasts a bulk event.
func (s *Service) BulkUpdateStatus(ctx context.Context, ids []int64, status string) error {
	tenantID := reqctx.MustTenant(ctx)
	if err := s.repo.BulkUpdateStatus(ctx, tenantID, ids, status); err != nil {
		return err
	}
	s.hub.Broadcast(realtime.Event{TenantID: tenantID, Entity: "invoice", ID: 0, Action: "bulk_status"})
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
		s.hub.Broadcast(realtime.Event{TenantID: tenantID, Entity: "invoice", ID: 0, Action: "overdue_sweep"})
	}
	return rows, nil
}
