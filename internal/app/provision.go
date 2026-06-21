package app

import (
	"context"

	"github.com/dknathalage/tallyo/internal/auth"
	"github.com/dknathalage/tallyo/internal/tenantdb"
)

// provisionProfile returns the signup profile provisioner: it opens (and lazily
// migrates) the new tenant's DB via the registry and creates its default
// business_profile there. Used to wire the signup handler in the composition
// root, keeping the cross-DB orchestration out of the auth slice.
func provisionProfile(reg *tenantdb.Registry) auth.ProfileProvisioner {
	return func(ctx context.Context, tenantID int64, in auth.SignupInput) error {
		tdb, err := reg.ForTenantID(tenantID)
		if err != nil {
			return err
		}
		return auth.ProvisionBusinessProfile(ctx, tdb, tenantID, in)
	}
}
