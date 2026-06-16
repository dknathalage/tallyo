package service

import (
	"context"
	"database/sql"

	"github.com/dknathalage/tallyo/internal/realtime"
	"github.com/dknathalage/tallyo/internal/repository"
	"github.com/dknathalage/tallyo/internal/reqctx"
)

// BusinessProfileService orchestrates business-profile reads/writes and
// publishes change events after a successful commit.
type BusinessProfileService struct {
	repo *repository.BusinessProfileRepo
	hub  *realtime.Hub
}

func NewBusinessProfileService(db *sql.DB, hub *realtime.Hub) *BusinessProfileService {
	if hub == nil {
		panic("NewBusinessProfileService: nil hub")
	}
	return &BusinessProfileService{repo: repository.NewBusinessProfile(db), hub: hub}
}

// Get returns the business profile, or nil if unset.
func (s *BusinessProfileService) Get(ctx context.Context) (*repository.BusinessProfile, error) {
	tenantID := reqctx.MustTenant(ctx)
	return s.repo.Get(ctx, tenantID)
}

// Save upserts the profile, then broadcasts AFTER the commit succeeds.
func (s *BusinessProfileService) Save(ctx context.Context, in repository.BusinessProfileInput) error {
	tenantID := reqctx.MustTenant(ctx)
	if err := s.repo.Save(ctx, tenantID, in); err != nil {
		return err
	}
	s.hub.Broadcast(realtime.Event{TenantID: tenantID, Entity: "business_profile", ID: 1, Action: "update"})
	return nil
}
