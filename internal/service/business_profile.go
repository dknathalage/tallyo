package service

import (
	"context"
	"database/sql"

	"github.com/dknathalage/tallyo/internal/repository"
)

// BusinessProfileService is bound into the Wails frontend.
type BusinessProfileService struct {
	repo *repository.BusinessProfileRepo
}

func NewBusinessProfileService(db *sql.DB) *BusinessProfileService {
	return &BusinessProfileService{repo: repository.NewBusinessProfile(db)}
}

// Get returns the business profile, or null if unset.
func (s *BusinessProfileService) Get() (*repository.BusinessProfile, error) {
	return s.repo.Get(context.Background())
}

// Save upserts the business profile.
func (s *BusinessProfileService) Save(in repository.BusinessProfileInput) error {
	return s.repo.Save(context.Background(), in)
}
