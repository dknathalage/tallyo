package subscription

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/dknathalage/tallyo/internal/httpx"
	stripe "github.com/stripe/stripe-go/v82"
	"github.com/stripe/stripe-go/v82/webhook"
)

// maxWebhookBody caps the webhook body read. Stripe events are small; this guards
// against a malicious oversized body before signature verification.
const maxWebhookBody = 1 << 20 // 1 MiB

// Webhook is the tenant-agnostic Stripe webhook endpoint. It is mounted OUTSIDE
// auth and any body-consuming middleware (it needs the raw body for signature
// verification). It always acks (200) handled, ignored, and unresolvable events
// so Stripe does not retry into a storm; it returns 400 only for a bad signature
// and 500 only for a genuine internal (DB) error worth retrying.
func (h *Handler) Webhook(w http.ResponseWriter, r *http.Request) {
	log := httpx.LoggerFrom(r.Context())
	body, err := io.ReadAll(io.LimitReader(r.Body, maxWebhookBody))
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "could not read body")
		return
	}
	// IgnoreAPIVersionMismatch: we read only a few stable fields (status, customer,
	// metadata, trial_end), so a dashboard/SDK API-version skew must not reject
	// otherwise-valid, correctly-signed events.
	event, err := webhook.ConstructEventWithOptions(
		body, r.Header.Get("Stripe-Signature"), h.client.WebhookSecret(),
		webhook.ConstructEventOptions{IgnoreAPIVersionMismatch: true},
	)
	if err != nil {
		log.Warn("stripe webhook: bad signature", "err", err)
		httpx.WriteError(w, http.StatusBadRequest, "invalid signature")
		return
	}

	syncedAt := time.Unix(event.Created, 0).UTC().Format(time.RFC3339)
	upd, ok, err := h.updateForEvent(r.Context(), event, syncedAt)
	if err != nil {
		log.Error("stripe webhook: handler error", "type", event.Type, "err", err)
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if !ok {
		// Unhandled event type or unresolvable tenant — ack so Stripe stops retrying.
		w.WriteHeader(http.StatusOK)
		return
	}
	if _, err := h.store.Apply(r.Context(), upd); err != nil {
		log.Error("stripe webhook: apply", "type", event.Type, "err", err)
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.WriteHeader(http.StatusOK)
}

// updateForEvent maps a Stripe event to a store Update. ok is false for event
// types we don't handle or whose tenant can't be resolved (caller acks anyway).
func (h *Handler) updateForEvent(ctx context.Context, event stripe.Event, syncedAt string) (upd Update, ok bool, err error) {
	switch event.Type {
	case "checkout.session.completed":
		var cs stripe.CheckoutSession
		if err := json.Unmarshal(event.Data.Raw, &cs); err != nil {
			return Update{}, false, err
		}
		if cs.ClientReferenceID == "" {
			return Update{}, false, nil // can't map to a tenant
		}
		u := Update{
			TenantID:         cs.ClientReferenceID,
			StripeCustomerID: customerID(cs.Customer),
			Status:           StatusTrialing, // card-required trial always starts trialing
			SyncedAt:         syncedAt,
		}
		if cs.Subscription != nil {
			u.StripeSubscriptionID = cs.Subscription.ID
		}
		return u, true, nil

	case "customer.subscription.updated", "customer.subscription.deleted":
		var sub stripe.Subscription
		if err := json.Unmarshal(event.Data.Raw, &sub); err != nil {
			return Update{}, false, err
		}
		tenantID, err := h.resolveTenant(ctx, &sub)
		if err != nil {
			return Update{}, false, err
		}
		if tenantID == "" {
			return Update{}, false, nil // unresolvable — ack and move on
		}
		status := string(sub.Status)
		if event.Type == "customer.subscription.deleted" {
			status = StatusCanceled
		}
		u := Update{
			TenantID:             tenantID,
			StripeCustomerID:     customerID(sub.Customer),
			StripeSubscriptionID: sub.ID,
			Status:               status,
			SyncedAt:             syncedAt,
		}
		if sub.TrialEnd > 0 {
			u.TrialEnd = time.Unix(sub.TrialEnd, 0).UTC().Format(time.RFC3339)
		}
		// ponytail: current_period_end is display-only and moved to the item level
		// in this API version; left empty for now, wire from sub.Items if the UI
		// needs the renewal date.
		return u, true, nil

	default:
		return Update{}, false, nil // unhandled type — ack
	}
}

// resolveTenant maps a subscription back to a tenant: first by the linked Stripe
// customer, then (self-heal, if a subscription event arrived before the checkout
// completion that links the customer) by the tenant_id stamped in the
// subscription metadata.
func (h *Handler) resolveTenant(ctx context.Context, sub *stripe.Subscription) (string, error) {
	if cid := customerID(sub.Customer); cid != "" {
		tenantID, found, err := h.store.GetTenantByStripeCustomer(ctx, cid)
		if err != nil {
			return "", err
		}
		if found {
			return tenantID, nil
		}
	}
	if sub.Metadata != nil {
		if t := sub.Metadata["tenant_id"]; t != "" {
			return t, nil
		}
	}
	return "", nil
}

func customerID(c *stripe.Customer) string {
	if c == nil {
		return ""
	}
	return c.ID
}
