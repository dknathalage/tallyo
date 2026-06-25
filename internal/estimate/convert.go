package estimate

// Convert (accepted estimate → draft invoice, co-transacted with the estimate's
// converted-link flip) and Duplicate (copy into a fresh draft). Split out of
// repository.go to keep that file to core CRUD.

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/dknathalage/tallyo/internal/audit"
	"github.com/dknathalage/tallyo/internal/billing"
	"github.com/dknathalage/tallyo/internal/db"
	"github.com/dknathalage/tallyo/internal/db/gen"
	"github.com/dknathalage/tallyo/internal/ids"
	"github.com/dknathalage/tallyo/internal/numbering"
)

// ErrNotAccepted is returned when converting an estimate that is not in the
// 'accepted' state.
var ErrNotAccepted = errors.New("only accepted estimates can be converted")

// ErrAlreadyConverted is returned when converting an estimate that already has
// a linked invoice.
var ErrAlreadyConverted = errors.New("estimate already converted")

// Duplicate creates a new draft estimate copying the source's client, plan
// manager, tax, notes, snapshots and line items, resetting the date to today,
// clearing valid-until, and assigning a fresh number.
func (r *EstimatesRepo) Duplicate(ctx context.Context, tenantID, id string) (*Estimate, error) {
	src, err := r.Get(ctx, tenantID, id)
	if err != nil {
		return nil, fmt.Errorf("duplicate estimate: %w", err)
	}
	if src == nil {
		return nil, errors.New("duplicate estimate: source not found")
	}
	var clientID string
	if src.ClientID != nil {
		clientID = *src.ClientID
	}
	in := EstimateInput{
		ClientID:         clientID,
		PayerID:          src.PayerID,
		Status:           "draft",
		IssueDate:        time.Now().UTC().Format("2006-01-02"),
		ValidUntil:       "",
		Tax:              src.Tax,
		Notes:            src.Notes,
		BusinessSnapshot: src.BusinessSnapshot,
		ClientSnapshot:   src.ClientSnapshot,
		PayerSnapshot:    src.PayerSnapshot,
	}
	items := lineItemsToInput(src.LineItems)

	var newID string
	err = numbering.WithRetry(ctx, 10, func() error {
		return r.createTx(ctx, tenantID, in, items, &newID)
	})
	if err != nil {
		return nil, fmt.Errorf("duplicate estimate: %w", err)
	}
	return r.Get(ctx, tenantID, newID)
}

// Convert turns an accepted estimate into a draft invoice (copying header and
// items, with valid_until becoming the invoice due date), links the estimate to
// the new invoice and flips it to 'converted'. Returns (nil, nil) when the
// estimate is missing, ErrNotAccepted unless status is 'accepted', and
// ErrAlreadyConverted when a linked invoice already exists.
func (r *EstimatesRepo) Convert(ctx context.Context, tenantID, estimateID string) (*ConvertResult, error) {
	est, err := r.Get(ctx, tenantID, estimateID)
	if err != nil {
		return nil, fmt.Errorf("convert estimate: %w", err)
	}
	if est == nil {
		return nil, nil
	}
	if est.ConvertedInvoiceID != nil {
		return nil, ErrAlreadyConverted
	}
	if est.Status != "accepted" {
		return nil, ErrNotAccepted
	}

	var invID string
	var invNum, invUUID string
	err = numbering.WithRetry(ctx, 10, func() error {
		return r.convertTx(ctx, tenantID, est, &invID, &invNum, &invUUID)
	})
	if err != nil {
		return nil, fmt.Errorf("convert estimate: %w", err)
	}
	return &ConvertResult{InvoiceID: invID, InvoiceUUID: invUUID, InvoiceNumber: invNum, EstimateNumber: est.Number}, nil
}

// convertTx runs a single convert attempt inside one transaction.
func (r *EstimatesRepo) convertTx(ctx context.Context, tenantID string, est *Estimate, invID *string, invNum, invUUID *string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	q := gen.New(tx)
	// A converted estimate becomes a real invoice, so it keeps the "INV-" prefix.
	num, err := billing.NextNumber(ctx, q, tenantID, "INV-")
	if err != nil {
		return err
	}
	inv, err := q.CreateInvoice(ctx, buildInvoiceFromEstimate(tenantID, est, num))
	if err != nil {
		return err
	}
	if err := copyEstimateItemsToInvoice(ctx, q, tenantID, inv.ID, est.LineItems); err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if err := q.SetEstimateConverted(ctx, gen.SetEstimateConvertedParams{
		ConvertedInvoiceID: sql.NullString{String: inv.ID, Valid: true}, UpdatedAt: now, TenantID: tenantID, ID: est.ID,
	}); err != nil {
		return err
	}
	if err := audit.Log(ctx, tx, audit.Entry{
		EntityType: "estimate", EntityID: est.ID, Action: "convert",
		Changes: audit.Changes(map[string]any{"invoiceId": inv.ID, "invoiceNumber": num}),
	}); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	*invID = inv.ID
	*invNum = num
	*invUUID = inv.ID
	return nil
}

// buildInvoiceFromEstimate maps an estimate header onto invoice create params.
func buildInvoiceFromEstimate(tenantID string, est *Estimate, num string) gen.CreateInvoiceParams {
	var clientID string
	if est.ClientID != nil {
		clientID = *est.ClientID
	}
	now := time.Now().UTC().Format(time.RFC3339)
	return gen.CreateInvoiceParams{
		ID:               ids.New(),
		TenantID:         tenantID,
		Number:           num,
		ClientID:         clientID,
		PayerID:          db.NullStr(est.PayerID),
		Status:           "draft",
		IssueDate:        est.IssueDate,
		DueDate:          est.ValidUntil,
		Subtotal:         est.Subtotal,
		Tax:              est.Tax,
		Total:            est.Total,
		Notes:            db.NzMaybe(est.Notes),
		BusinessSnapshot: db.NzMaybe(est.BusinessSnapshot),
		ClientSnapshot:   db.NzMaybe(est.ClientSnapshot),
		PayerSnapshot:    db.NzMaybe(est.PayerSnapshot),
		CreatedAt:        now,
		UpdatedAt:        now,
	}
}

// copyEstimateItemsToInvoice writes each estimate line item as an invoice line.
func copyEstimateItemsToInvoice(ctx context.Context, q *gen.Queries, tenantID, invoiceID string, items []*billing.LineItem) error {
	for i := range items { // bounded by len(items)
		it := items[i]
		_, err := q.CreateLineItem(ctx, gen.CreateLineItemParams{
			ID:              ids.New(),
			TenantID:        tenantID,
			SessionID:       sql.NullString{}, // estimate-converted lines are not session items
			InvoiceID:       sql.NullString{String: invoiceID, Valid: true},
			CatalogueItemID: db.NullStr(it.CatalogueItemID),
			Code:            db.NzMaybe(it.Code),
			Description:     it.Description,
			ServiceDate:     db.NzMaybe(it.ServiceDate),
			Unit:            db.NzMaybe(it.Unit),
			Quantity:        it.Quantity,
			UnitPrice:       it.UnitPrice,
			Taxable:         db.B2i(it.Taxable),
			LineTotal:       it.LineTotal,
			SortOrder:       sql.NullInt64{Int64: it.SortOrder, Valid: true},
		})
		if err != nil {
			return fmt.Errorf("copy estimate item %d: %w", i, err)
		}
	}
	return nil
}

// lineItemsToInput converts stored line items back into writable inputs.
func lineItemsToInput(items []*billing.LineItem) []billing.LineItemInput {
	out := make([]billing.LineItemInput, 0, len(items))
	for i := range items { // bounded by len(items)
		it := items[i]
		out = append(out, billing.LineItemInput{
			CatalogueItemID: it.CatalogueItemID,
			Code:            it.Code,
			Description:     it.Description,
			ServiceDate:     it.ServiceDate,
			Unit:            it.Unit,
			Quantity:        it.Quantity,
			UnitPrice:       it.UnitPrice,
			Taxable:         it.Taxable,
			SortOrder:       it.SortOrder,
		})
	}
	return out
}
