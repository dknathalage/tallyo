// Package subscription holds SaaS billing: the entitlement gate, the Stripe
// integration (Checkout + Customer Portal), and the webhook that syncs Stripe
// subscription state back onto the control-DB tenants table. It is distinct from
// internal/billing, which computes invoice line-item math.
package subscription

// Subscription status values. trialing/active/past_due mirror Stripe; "none" is
// the local pre-Checkout state (tenant signed up but never paid).
const (
	StatusNone     = "none"
	StatusTrialing = "trialing"
	StatusActive   = "active"
	StatusPastDue  = "past_due" // in dunning — kept entitled with a grace banner
	StatusCanceled = "canceled"
)

// Entitled reports whether a tenant in the given subscription status may perform
// write actions. trialing, active, and past_due (grace) are entitled; none and
// canceled are not. Stripe owns the trial clock, so there is no time math here —
// the webhook flips the status and that is the only input.
func Entitled(status string) bool {
	switch status {
	case StatusActive, StatusTrialing, StatusPastDue:
		return true
	default:
		return false
	}
}
