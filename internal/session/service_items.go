package session

import (
	"context"
	"fmt"

	"github.com/dknathalage/tallyo/internal/billing"
	"github.com/dknathalage/tallyo/internal/reqctx"
)

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
	s.events.Updated(tenantID, sh.ID)
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
	s.events.Updated(tenantID, sh.ID)
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
	s.events.Updated(tenantID, sh.ID)
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
	s.events.Updated(tenantID, sessionUUID)
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
	s.events.Updated(tenantID, sh.ID)
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
