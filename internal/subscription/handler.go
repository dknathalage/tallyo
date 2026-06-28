package subscription

import (
	"context"
	"net/http"

	"github.com/dknathalage/tallyo/internal/auth"
	"github.com/dknathalage/tallyo/internal/httpx"
	"github.com/dknathalage/tallyo/internal/reqctx"
	"github.com/go-chi/chi/v5"
)

// TenantReader resolves the full tenant row (with subscription fields) by id.
// Satisfied by *auth.TenantsRepo.
type TenantReader interface {
	GetByUUID(ctx context.Context, tenantUUID string) (*auth.Tenant, error)
}

// Handler serves the tenant-facing billing routes (checkout/portal/status) and
// the tenant-agnostic Stripe webhook. It is nil when BILLING_ENABLED is off, so
// the routes are simply not mounted.
type Handler struct {
	client  *Client
	store   *Store
	tenants TenantReader
}

// NewHandler constructs the billing handler. Nil dependencies are programmer errors.
func NewHandler(client *Client, store *Store, tenants TenantReader) *Handler {
	if client == nil || store == nil || tenants == nil {
		panic("subscription: NewHandler nil dep")
	}
	return &Handler{client: client, store: store, tenants: tenants}
}

// Routes registers the tenant-scoped billing endpoints on a router that already
// has ResolveTenant applied. Checkout/Portal are owner-only; status is readable
// by any tenant member. These MUST be mounted outside the RequireSubscription
// group so a lapsed tenant can still pay.
func (h *Handler) Routes(r chi.Router) {
	r.With(httpx.RequireRole("owner")).Post("/billing/checkout", h.Checkout)
	r.With(httpx.RequireRole("owner")).Get("/billing/portal", h.Portal)
	r.Get("/billing", h.Status)
}

// Checkout creates a Stripe Checkout Session for the current tenant and returns
// its redirect URL. Owner-only.
func (h *Handler) Checkout(w http.ResponseWriter, r *http.Request) {
	tenantID := reqctx.MustTenant(r.Context())
	tenantUUID := chi.URLParam(r, "tenantUUID")
	email, _ := reqctx.EmailFrom(r.Context())
	base := baseURL(r) + "/" + tenantUUID + "/settings/billing"

	url, err := h.client.CreateCheckoutSession(r.Context(), CheckoutInput{
		TenantID:   tenantID,
		Email:      email,
		SuccessURL: base + "?checkout=success",
		CancelURL:  base + "?checkout=cancel",
		Plan:       r.URL.Query().Get("plan"),
	})
	if err != nil {
		httpx.LoggerFrom(r.Context()).Error("checkout session", "err", err)
		httpx.WriteError(w, http.StatusBadGateway, "could not start checkout")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"url": url})
}

// Portal creates a Stripe Customer Portal session for the current tenant. Owner-only.
// Returns 409 if the tenant has no Stripe customer yet (never completed Checkout).
func (h *Handler) Portal(w http.ResponseWriter, r *http.Request) {
	tenantID := reqctx.MustTenant(r.Context())
	tenantUUID := chi.URLParam(r, "tenantUUID")
	t, err := h.tenants.GetByUUID(r.Context(), tenantID)
	if err != nil || t == nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if t.StripeCustomerID == "" {
		httpx.WriteError(w, http.StatusConflict, "no active subscription to manage")
		return
	}
	returnURL := baseURL(r) + "/" + tenantUUID + "/settings/billing"
	url, err := h.client.CreatePortalSession(r.Context(), t.StripeCustomerID, returnURL)
	if err != nil {
		httpx.LoggerFrom(r.Context()).Error("portal session", "err", err)
		httpx.WriteError(w, http.StatusBadGateway, "could not open billing portal")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"url": url})
}

// Status returns the current subscription state for the UI. Readable by any member.
func (h *Handler) Status(w http.ResponseWriter, r *http.Request) {
	tenantID := reqctx.MustTenant(r.Context())
	t, err := h.tenants.GetByUUID(r.Context(), tenantID)
	if err != nil || t == nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"status":           t.SubscriptionStatus,
		"trialEnd":         t.TrialEnd,
		"currentPeriodEnd": t.CurrentPeriodEnd,
		"entitled":         Entitled(t.SubscriptionStatus),
	})
}

// baseURL reconstructs the request's external origin, honoring the
// X-Forwarded-Proto header set by Cloud Run / proxies.
func baseURL(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}
