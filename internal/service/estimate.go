package service

import (
	"context"
	"database/sql"

	"github.com/dknathalage/tallyo/internal/realtime"
	"github.com/dknathalage/tallyo/internal/repository"
)

// EstimateService orchestrates estimate reads/writes and publishes change events
// after a successful commit. Unlike invoices it has no overdue sweep, but it adds
// a Convert action that turns an accepted estimate into an invoice.
type EstimateService struct {
	repo *repository.EstimatesRepo
	hub  *realtime.Hub
}

func NewEstimateService(db *sql.DB, hub *realtime.Hub) *EstimateService {
	if hub == nil {
		panic("NewEstimateService: nil hub")
	}
	return &EstimateService{repo: repository.NewEstimates(db), hub: hub}
}

func (s *EstimateService) List(ctx context.Context) ([]*repository.Estimate, error) {
	return s.repo.List(ctx)
}

func (s *EstimateService) ListByStatus(ctx context.Context, status string) ([]*repository.Estimate, error) {
	return s.repo.ListByStatus(ctx, status)
}

func (s *EstimateService) ListClientEstimates(ctx context.Context, clientID int64) ([]*repository.Estimate, error) {
	return s.repo.ListClientEstimates(ctx, clientID)
}

func (s *EstimateService) Get(ctx context.Context, id int64) (*repository.Estimate, error) {
	return s.repo.Get(ctx, id)
}

// Create inserts an estimate + line items, then broadcasts on success.
func (s *EstimateService) Create(ctx context.Context, in repository.EstimateInput, items []repository.LineItemInput) (*repository.Estimate, error) {
	est, err := s.repo.Create(ctx, in, items)
	if err != nil {
		return nil, err
	}
	s.hub.Broadcast(realtime.Event{Entity: "estimate", ID: est.ID, Action: "create"})
	return est, nil
}

// Update rewrites an estimate. A nil result means the row was not found, in which
// case no event is published.
func (s *EstimateService) Update(ctx context.Context, id int64, in repository.EstimateInput, items []repository.LineItemInput) (*repository.Estimate, error) {
	est, err := s.repo.Update(ctx, id, in, items)
	if err != nil {
		return nil, err
	}
	if est == nil {
		return nil, nil
	}
	s.hub.Broadcast(realtime.Event{Entity: "estimate", ID: id, Action: "update"})
	return est, nil
}

// UpdateStatus sets the estimate status, then broadcasts on success.
func (s *EstimateService) UpdateStatus(ctx context.Context, id int64, status string) error {
	if err := s.repo.UpdateStatus(ctx, id, status); err != nil {
		return err
	}
	s.hub.Broadcast(realtime.Event{Entity: "estimate", ID: id, Action: "status"})
	return nil
}

// Delete removes an estimate, then broadcasts on success.
func (s *EstimateService) Delete(ctx context.Context, id int64) error {
	if err := s.repo.Delete(ctx, id); err != nil {
		return err
	}
	s.hub.Broadcast(realtime.Event{Entity: "estimate", ID: id, Action: "delete"})
	return nil
}

// Duplicate copies an estimate, then broadcasts a create for the new id.
func (s *EstimateService) Duplicate(ctx context.Context, id int64) (*repository.Estimate, error) {
	est, err := s.repo.Duplicate(ctx, id)
	if err != nil {
		return nil, err
	}
	s.hub.Broadcast(realtime.Event{Entity: "estimate", ID: est.ID, Action: "create"})
	return est, nil
}

// BulkDelete removes several estimates, then broadcasts a single bulk event.
func (s *EstimateService) BulkDelete(ctx context.Context, ids []int64) error {
	if err := s.repo.BulkDelete(ctx, ids); err != nil {
		return err
	}
	s.hub.Broadcast(realtime.Event{Entity: "estimate", ID: 0, Action: "bulk_delete"})
	return nil
}

// BulkUpdateStatus sets several estimates' status, then broadcasts a bulk event.
func (s *EstimateService) BulkUpdateStatus(ctx context.Context, ids []int64, status string) error {
	if err := s.repo.BulkUpdateStatus(ctx, ids, status); err != nil {
		return err
	}
	s.hub.Broadcast(realtime.Event{Entity: "estimate", ID: 0, Action: "bulk_status"})
	return nil
}

// Convert turns an accepted estimate into an invoice. On success it broadcasts an
// estimate "convert" event and an invoice "create" event for the new invoice, then
// returns the result. ErrNotAccepted/ErrAlreadyConverted are propagated unchanged.
func (s *EstimateService) Convert(ctx context.Context, id int64) (*repository.ConvertResult, error) {
	res, err := s.repo.Convert(ctx, id)
	if err != nil {
		return nil, err
	}
	if res == nil {
		return nil, nil
	}
	s.hub.Broadcast(realtime.Event{Entity: "estimate", ID: id, Action: "convert"})
	s.hub.Broadcast(realtime.Event{Entity: "invoice", ID: res.InvoiceID, Action: "create"})
	return res, nil
}
