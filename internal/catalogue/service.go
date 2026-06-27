package catalogue

import (
	"context"

	"github.com/dknathalage/tallyo/internal/apperr"
	"github.com/dknathalage/tallyo/internal/db"
	"github.com/dknathalage/tallyo/internal/listquery"
	"github.com/dknathalage/tallyo/internal/reqctx"
)

// Service orchestrates per-tenant catalogue reads/writes.
type Service struct {
	repo *Repo
}

// NewService constructs a Service.
func NewService(database db.Executor) *Service {
	return &Service{repo: NewRepo(database)}
}

func (s *Service) List(ctx context.Context) ([]*CatalogueItem, error) {
	return s.repo.List(ctx, reqctx.MustTenant(ctx))
}

// Query returns a page of catalogue items. Rows is always non-nil so it
// serializes as [] not null.
func (s *Service) Query(ctx context.Context, c listquery.Clause) (listquery.Result[*CatalogueItem], error) {
	tenantID := reqctx.MustTenant(ctx)
	rows, total, err := s.repo.Query(ctx, tenantID, c)
	if err != nil {
		return listquery.Result[*CatalogueItem]{}, err
	}
	if rows == nil {
		rows = []*CatalogueItem{}
	}
	return listquery.Result[*CatalogueItem]{Rows: rows, Total: total}, nil
}

func (s *Service) Search(ctx context.Context, q string) ([]*CatalogueItem, error) {
	return s.repo.Search(ctx, reqctx.MustTenant(ctx), q)
}

func (s *Service) Get(ctx context.Context, uuid string) (*CatalogueItem, error) {
	item, err := s.repo.Get(ctx, reqctx.MustTenant(ctx), uuid)
	if err != nil {
		return nil, err
	}
	if item == nil {
		return nil, apperr.ErrNotFound
	}
	return item, nil
}

// Create inserts a catalogue item.
func (s *Service) Create(ctx context.Context, in CatalogueItemInput) (*CatalogueItem, error) {
	if err := in.Validate(); err != nil {
		return nil, err
	}
	tenantID := reqctx.MustTenant(ctx)
	item, err := s.repo.Create(ctx, tenantID, in)
	if err != nil {
		return nil, err
	}
	return item, nil
}

// Update mutates (copy-on-write) a catalogue item.
func (s *Service) Update(ctx context.Context, uuid string, in CatalogueItemInput) (*CatalogueItem, error) {
	if err := in.Validate(); err != nil {
		return nil, err
	}
	tenantID := reqctx.MustTenant(ctx)
	item, err := s.repo.Update(ctx, tenantID, uuid, in)
	if err != nil {
		return nil, err
	}
	return item, nil
}

// Delete tombstones a catalogue item.
func (s *Service) Delete(ctx context.Context, uuid string) error {
	tenantID := reqctx.MustTenant(ctx)
	if err := s.repo.Delete(ctx, tenantID, uuid); err != nil {
		return err
	}
	return nil
}

// ResolveLogicalIDs resolves catalogue version-row uuids to logical_ids for the
// tenant (order preserved). An unknown uuid surfaces as an error so the caller
// can 400.
func (s *Service) ResolveLogicalIDs(ctx context.Context, uuids []string) ([]string, error) {
	return s.repo.ResolveCatalogueLogicalIDs(ctx, reqctx.MustTenant(ctx), uuids)
}

// BulkDelete tombstones multiple items.
func (s *Service) BulkDelete(ctx context.Context, logicalIDs []string) error {
	tenantID := reqctx.MustTenant(ctx)
	if err := s.repo.BulkDelete(ctx, tenantID, logicalIDs); err != nil {
		return err
	}
	return nil
}

// Inspect previews an uploaded file (owner/admin import, step 1).
func (s *Service) Inspect(data []byte, fileType, sheetName string, headerRow int) (*InspectResult, error) {
	return s.repo.Inspect(data, fileType, sheetName, headerRow)
}

// ImportMapped applies a column mapping and upserts by code (owner/admin import,
// step 2).
func (s *Service) ImportMapped(ctx context.Context, data []byte, fileType, sheetName string, headerRow int, mapping map[string]string) (*ImportSummary, error) {
	tenantID := reqctx.MustTenant(ctx)
	summary, err := s.repo.ImportMapped(ctx, tenantID, data, fileType, sheetName, headerRow, mapping)
	if err != nil {
		return nil, err
	}
	return summary, nil
}
