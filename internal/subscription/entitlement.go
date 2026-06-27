// Package subscription holds SaaS billing: the entitlement gate (re-exported from
// internal/entitlement), the Stripe integration (Checkout + Customer Portal), and
// the webhook that syncs Stripe subscription state back onto the control-DB
// tenants table. It is distinct from internal/billing, which computes invoice
// line-item math.
package subscription

import "github.com/dknathalage/tallyo/internal/entitlement"

// Status constants re-exported from internal/entitlement so callers within the
// billing domain use subscription.Status*. The canonical definitions (and the
// gate rule) live in the leaf entitlement package to avoid an httpx import cycle.
const (
	StatusNone     = entitlement.StatusNone
	StatusTrialing = entitlement.StatusTrialing
	StatusActive   = entitlement.StatusActive
	StatusPastDue  = entitlement.StatusPastDue
	StatusCanceled = entitlement.StatusCanceled
)

// Entitled re-exports entitlement.Entitled.
var Entitled = entitlement.Entitled
