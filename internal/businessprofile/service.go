package businessprofile

import (
	"context"
	"github.com/dknathalage/tallyo/internal/db"

	"github.com/dknathalage/tallyo/internal/reqctx"
)

// Service orchestrates business-profile reads/writes.
type Service struct {
	repo *BusinessProfileRepo
}

// NewService constructs the business-profile service.
func NewService(db db.Executor) *Service {
	return &Service{repo: NewBusinessProfile(db)}
}

// Get returns the business profile, or nil if unset.
func (s *Service) Get(ctx context.Context) (*BusinessProfile, error) {
	tenantID := reqctx.MustTenant(ctx)
	return s.repo.Get(ctx, tenantID)
}

// Save upserts the profile.
func (s *Service) Save(ctx context.Context, in BusinessProfileInput) error {
	if err := in.Validate(); err != nil {
		return err
	}
	tenantID := reqctx.MustTenant(ctx)
	if err := s.repo.Save(ctx, tenantID, in); err != nil {
		return err
	}
	return nil
}
