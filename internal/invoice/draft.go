package invoice

// draft-from-sessions: the deterministic path that links N recorded, unbilled
// sessions' items onto a fresh draft invoice (no model, no re-pricing). Split out
// of repository.go to keep that file to core CRUD.

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/dknathalage/tallyo/internal/audit"
	"github.com/dknathalage/tallyo/internal/billing"
	"github.com/dknathalage/tallyo/internal/db/gen"
	"github.com/dknathalage/tallyo/internal/numbering"
)

// draftSessionItem holds the validated facts about one session that DraftFromSessions
// needs: its client and the number of unbilled items it carries.
type draftSessionItem struct {
	sessionID string
	clientID  string
	itemCount int64
}

// validateDraftSessions reads each session (no writes) and enforces the draft
// preconditions: the session exists for the tenant, is status 'recorded' with no
// invoice yet, carries at least one unbilled item (G5), and every session shares
// one client. Returns the shared client id and the per-session facts.
func (r *InvoicesRepo) validateDraftSessions(ctx context.Context, tenantID string, sessionIDs []string) (string, []draftSessionItem, error) {
	if len(sessionIDs) == 0 {
		return "", nil, errors.New("draft from sessions: at least one session is required")
	}
	q := gen.New(r.db)
	var clientID string
	facts := make([]draftSessionItem, 0, len(sessionIDs))
	for i := range sessionIDs { // bounded by len(sessionIDs)
		sh, err := q.GetSessionByID(ctx, gen.GetSessionByIDParams{TenantID: tenantID, ID: sessionIDs[i]})
		if errors.Is(err, sql.ErrNoRows) {
			return "", nil, fmt.Errorf("draft from sessions: session %s not found", sessionIDs[i])
		}
		if err != nil {
			return "", nil, fmt.Errorf("draft from sessions: load session %s: %w", sessionIDs[i], err)
		}
		if sh.Status != "recorded" || sh.InvoiceID.Valid {
			return "", nil, fmt.Errorf("draft from sessions: session %s is not recorded+unbilled", sessionIDs[i])
		}
		if i == 0 {
			clientID = sh.ClientID
		} else if sh.ClientID != clientID {
			return "", nil, errors.New("draft from sessions: all sessions must share one client")
		}
		n, err := q.CountSessionItems(ctx, gen.CountSessionItemsParams{TenantID: tenantID, SessionID: sql.NullString{String: sessionIDs[i], Valid: true}})
		if err != nil {
			return "", nil, fmt.Errorf("draft from sessions: count items %s: %w", sessionIDs[i], err)
		}
		if n == 0 {
			return "", nil, fmt.Errorf("draft from sessions: session %s has no items", sessionIDs[i])
		}
		facts = append(facts, draftSessionItem{sessionID: sessionIDs[i], clientID: sh.ClientID, itemCount: n})
	}
	return clientID, facts, nil
}

// DraftFromSessions creates a draft invoice header for clientID, links every
// validated session's unbilled items onto it, and persists totals computed from
// the now-linked lines — all in ONE numbering-retried transaction. The sessions
// table is NOT written here; the caller advances the sessions to 'drafted'
// afterwards (a separate, post-commit step), mirroring Delete↔ClearForInvoice.
func (r *InvoicesRepo) DraftFromSessions(ctx context.Context, tenantID, clientID string, facts []draftSessionItem) (*Invoice, error) {
	if tenantID == "" || clientID == "" {
		return nil, errors.New("draft from sessions: tenant and client id required")
	}
	in := InvoiceInput{ClientID: clientID, Status: "draft"}
	now := time.Now().UTC().Format("2006-01-02")
	in.IssueDate = now
	in.DueDate = now
	r.fillSnapshots(ctx, tenantID, &in)

	var newID string
	err := numbering.WithRetry(ctx, 10, func() error {
		return r.draftTx(ctx, tenantID, in, facts, &newID)
	})
	if err != nil {
		return nil, fmt.Errorf("draft from sessions: %w", err)
	}
	return r.Get(ctx, tenantID, newID)
}

// draftTx runs one draft attempt: allocate the number, insert a zero-total
// header, link each session's items (assigning a sort_order base), recompute
// totals from the linked lines, persist them, and audit — all in one tx.
func (r *InvoicesRepo) draftTx(ctx context.Context, tenantID string, in InvoiceInput, facts []draftSessionItem, newID *string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	q := gen.New(tx)
	num, err := NextInvoiceNumber(ctx, q, tenantID)
	if err != nil {
		return err
	}
	inv, err := q.CreateInvoice(ctx, createInvoiceParams(tenantID, in, nil, num))
	if err != nil {
		return err
	}
	var sortBase int64
	for i := range facts { // bounded by len(facts)
		if e := q.LinkSessionItemsToInvoice(ctx, gen.LinkSessionItemsToInvoiceParams{
			InvoiceID: sql.NullString{String: inv.ID, Valid: true},
			SortOrder: sql.NullInt64{Int64: sortBase, Valid: true},
			TenantID:  tenantID,
			SessionID: sql.NullString{String: facts[i].sessionID, Valid: true},
		}); e != nil {
			return fmt.Errorf("link session %s: %w", facts[i].sessionID, e)
		}
		sortBase += facts[i].itemCount
	}
	lines, err := q.ListLineItemsForInvoice(ctx, gen.ListLineItemsForInvoiceParams{
		TenantID: tenantID, InvoiceID: sql.NullString{String: inv.ID, Valid: true},
	})
	if err != nil {
		return fmt.Errorf("list linked lines: %w", err)
	}
	totals := totalsFromRows(lines)
	if _, e := q.UpdateInvoiceTotals(ctx, gen.UpdateInvoiceTotalsParams{
		Subtotal: totals.Subtotal, Tax: totals.Tax, Total: totals.Total,
		UpdatedAt: time.Now().UTC().Format(time.RFC3339), TenantID: tenantID, ID: inv.ID,
	}); e != nil {
		return fmt.Errorf("update totals: %w", e)
	}
	if e := audit.Log(ctx, tx, audit.Entry{
		EntityType: "invoice", EntityID: inv.ID, Action: "create",
		Changes: audit.Changes(map[string]any{"number": num, "draftedFromSessions": len(facts)}),
	}); e != nil {
		return e
	}
	if e := tx.Commit(); e != nil {
		return e
	}
	*newID = inv.ID
	return nil
}

// totalsFromRows sums line totals from already-priced line_items rows. Tax is 0
// (GST-free lines carry no tax; gst-bearing lines already fold tax into
// their unit price upstream — same as the human invoice path).
func totalsFromRows(rows []gen.ListLineItemsForInvoiceRow) billing.Totals {
	var subtotal float64
	for i := range rows { // bounded by len(rows)
		subtotal += billing.Round2(rows[i].LineTotal)
	}
	subtotal = billing.Round2(subtotal)
	return billing.Totals{Subtotal: subtotal, Tax: 0, Total: subtotal}
}
