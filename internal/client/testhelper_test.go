package client

import (
	"context"

	"github.com/dknathalage/tallyo/internal/reqctx"
)

// tctx returns a context carrying the given tenant id.
func tctx(tenantID string) context.Context {
	return reqctx.WithTenant(context.Background(), tenantID)
}
