// Package admin provides HTTP handlers for the platform-admin panel.
// Routes live under /api/admin and are gated by RequireAuth + ResolveAdminUser +
// RequirePlatformAdmin — no tenant scoping applies.
package admin

import (
	"context"
	"net/http"

	"github.com/dknathalage/tallyo/internal/audit"
	"github.com/dknathalage/tallyo/internal/auth"
	"github.com/dknathalage/tallyo/internal/httpx"
	"github.com/go-chi/chi/v5"
)

// TenantsLister lists all tenants with per-tenant user counts.
// Satisfied by *auth.TenantsRepo.
type TenantsLister interface {
	List(ctx context.Context) ([]*auth.TenantSummary, error)
}

// TenantReader resolves a tenant by its public UUID.
// Satisfied by *auth.TenantsRepo.
type TenantReader interface {
	GetByUUID(ctx context.Context, tenantUUID string) (*auth.Tenant, error)
}

// TenantSuspender suspends a tenant by UUID.
// Satisfied by *auth.TenantsRepo.
type TenantSuspender interface {
	Suspend(ctx context.Context, tenantUUID, adminUserID string) error
}

// TenantUnsuspender reverses a tenant suspension.
// Satisfied by *auth.TenantsRepo.
type TenantUnsuspender interface {
	Unsuspend(ctx context.Context, tenantUUID, adminUserID string) error
}

// TenantDeleter permanently removes a tenant.
// Satisfied by *auth.TenantsRepo.
type TenantDeleter interface {
	Delete(ctx context.Context, tenantUUID, adminUserID string) error
}

// SubscriptionSetter overrides a tenant's subscription status.
// Satisfied by *subscription.Store.
type SubscriptionSetter interface {
	SetSubscriptionStatus(ctx context.Context, tenantID, status, adminUserID string, trialEndsAt string) error
}

// AuditLister returns the recent audit trail for a tenant.
// Satisfied by *audit.Reader.
type AuditLister interface {
	ListByTenant(ctx context.Context, tenantID string) ([]audit.Record, error)
}

// TenantsRepo combines all tenant store operations needed by the admin handler.
// *auth.TenantsRepo satisfies this interface.
type TenantsRepo interface {
	TenantsLister
	TenantReader
	TenantSuspender
	TenantUnsuspender
	TenantDeleter
}

// Handler serves the platform-admin HTTP endpoints. It must be mounted under
// a RequireAuth + ResolveAdminUser + RequirePlatformAdmin middleware chain.
type Handler struct {
	tenants      TenantsRepo
	subscription SubscriptionSetter
	auditLog     AuditLister
}

// New constructs the admin handler. Nil dependencies are programmer errors.
func New(tenants TenantsRepo, subscription SubscriptionSetter, auditLog AuditLister) *Handler {
	if tenants == nil || subscription == nil || auditLog == nil {
		panic("admin: New nil dep")
	}
	return &Handler{tenants: tenants, subscription: subscription, auditLog: auditLog}
}

// Routes registers all admin endpoints on the provided router. The router must
// already have RequireAuth, ResolveAdminUser, and RequirePlatformAdmin applied.
func (h *Handler) Routes(r chi.Router) {
	r.Get("/tenants", h.List)
	r.Get("/tenants/{uuid}", h.Detail)
	r.Patch("/tenants/{uuid}/subscription", h.SetSubscription)
	r.Post("/tenants/{uuid}/suspend", h.Suspend)
	r.Post("/tenants/{uuid}/unsuspend", h.Unsuspend)
	r.Delete("/tenants/{uuid}", h.Delete)
}

// List returns all tenants with per-tenant user counts.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	tenants, err := h.tenants.List(r.Context())
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, tenants)
}

// detailResponse is the body of GET /api/admin/tenants/:uuid: the tenant row
// plus its recent audit trail (newest first, capped by the store query).
type detailResponse struct {
	Tenant *auth.Tenant   `json:"tenant"`
	Audit  []audit.Record `json:"audit"`
}

// Detail returns a single tenant by UUID together with its recent audit trail
// (the affected-tenant rows, including cross-tenant admin actions stamped via
// LogAs). The trail is bounded by the ListByTenant store query.
func (h *Handler) Detail(w http.ResponseWriter, r *http.Request) {
	uuid := chi.URLParam(r, "uuid")
	tenant, err := h.tenants.GetByUUID(r.Context(), uuid)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if tenant == nil {
		httpx.WriteError(w, http.StatusNotFound, "tenant not found")
		return
	}
	trail, err := h.auditLog.ListByTenant(r.Context(), tenant.ID)
	if err != nil {
		httpx.LoggerFrom(r.Context()).Error("list audit trail", "err", err)
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, detailResponse{Tenant: tenant, Audit: trail})
}

// setSubscriptionRequest is the request body for PATCH .../subscription.
type setSubscriptionRequest struct {
	Status      string `json:"status"`
	TrialEndsAt string `json:"trialEndsAt,omitempty"`
}

// SetSubscription overrides the subscription status for a tenant.
func (h *Handler) SetSubscription(w http.ResponseWriter, r *http.Request) {
	uuid := chi.URLParam(r, "uuid")

	u := httpx.UserFrom(r.Context())
	if u == nil {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req setSubscriptionRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Status == "" {
		httpx.WriteError(w, http.StatusUnprocessableEntity, "status is required")
		return
	}

	if err := h.subscription.SetSubscriptionStatus(r.Context(), uuid, req.Status, u.ID, req.TrialEndsAt); err != nil {
		httpx.LoggerFrom(r.Context()).Error("set subscription status", "err", err)
		httpx.WriteServiceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Suspend sets a tenant to suspended status, blocking login for all its users.
func (h *Handler) Suspend(w http.ResponseWriter, r *http.Request) {
	uuid := chi.URLParam(r, "uuid")
	u := httpx.UserFrom(r.Context())
	if u == nil {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if err := h.tenants.Suspend(r.Context(), uuid, u.ID); err != nil {
		httpx.LoggerFrom(r.Context()).Error("suspend tenant", "err", err)
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Unsuspend restores a tenant from suspended status.
func (h *Handler) Unsuspend(w http.ResponseWriter, r *http.Request) {
	uuid := chi.URLParam(r, "uuid")
	u := httpx.UserFrom(r.Context())
	if u == nil {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if err := h.tenants.Unsuspend(r.Context(), uuid, u.ID); err != nil {
		httpx.LoggerFrom(r.Context()).Error("unsuspend tenant", "err", err)
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Delete permanently removes a tenant and all its dependents (users, invites,
// audit rows in the control DB). This action is irreversible.
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	uuid := chi.URLParam(r, "uuid")
	u := httpx.UserFrom(r.Context())
	if u == nil {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if err := h.tenants.Delete(r.Context(), uuid, u.ID); err != nil {
		httpx.LoggerFrom(r.Context()).Error("delete tenant", "err", err)
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
