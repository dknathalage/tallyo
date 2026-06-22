package businessprofile

import (
	"context"
	"github.com/dknathalage/tallyo/internal/db"

	"github.com/dknathalage/tallyo/internal/realtime"
	"github.com/dknathalage/tallyo/internal/reqctx"
)

// Service orchestrates business-profile reads/writes and publishes change events
// after a successful commit.
type Service struct {
	repo *BusinessProfileRepo
	hub  *realtime.Hub
}

// NewService constructs the business-profile service. A nil hub is a programmer error.
func NewService(db db.Executor, hub *realtime.Hub) *Service {
	if hub == nil {
		panic("businessprofile.NewService: nil hub")
	}
	return &Service{repo: NewBusinessProfile(db), hub: hub}
}

// Get returns the business profile, or nil if unset.
func (s *Service) Get(ctx context.Context) (*BusinessProfile, error) {
	tenantID := reqctx.MustTenant(ctx)
	return s.repo.Get(ctx, tenantID)
}

// Save upserts the profile, then broadcasts AFTER the commit succeeds.
func (s *Service) Save(ctx context.Context, in BusinessProfileInput) error {
	tenantID := reqctx.MustTenant(ctx)
	if err := s.repo.Save(ctx, tenantID, in); err != nil {
		return err
	}
	s.hub.Broadcast(realtime.Event{TenantID: tenantID, Entity: "business_profile", UUID: "", Action: "update"})
	return nil
}
