package app

import (
	"context"
	"database/sql"

	"github.com/dknathalage/tallyo/internal/auth"
)

// provisionProfile returns the signup profile provisioner: it creates the new
// tenant's default business_profile in the shared DB. Wired in the composition
// root so the cross-slice orchestration stays out of the auth slice.
func provisionProfile(database *sql.DB) auth.ProfileProvisioner {
	return func(ctx context.Context, tenantID int64, in auth.SignupInput) error {
		return auth.ProvisionBusinessProfile(ctx, database, tenantID, in)
	}
}
