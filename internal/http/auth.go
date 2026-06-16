package httpapi

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/alexedwards/scs/v2"
	"github.com/dknathalage/tallyo/internal/auth"
)

// AuthHandler implements session login/logout and the authenticated "me" route.
type AuthHandler struct {
	sm      *scs.SessionManager
	users   *auth.UsersRepo
	tenants *auth.TenantsRepo
}

// NewAuthHandler constructs the handler. Nil dependencies are programmer errors.
func NewAuthHandler(sm *scs.SessionManager, users *auth.UsersRepo, tenants *auth.TenantsRepo) *AuthHandler {
	if sm == nil || users == nil || tenants == nil {
		panic("NewAuthHandler: nil dep")
	}
	return &AuthHandler{sm: sm, users: users, tenants: tenants}
}

// loginRequest is the decoded login body. TenantId is optional: it is REQUIRED
// only to disambiguate when the email is registered in more than one tenant.
type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	TenantID int64  `json:"tenantId"`
}

// Login verifies credentials and establishes a session. It uses a single error
// message for both unknown email and bad password to avoid user enumeration, and
// renews the session token to prevent session fixation.
//
// Multi-tenant fail-safe: email is UNIQUE only per (tenant_id, email). When an
// email exists in more than one tenant and the request did not name a tenant,
// login does NOT pick an arbitrary one — it returns 409 with the candidate
// tenants so the client can re-submit with a tenantId. A single-tenant email
// logs in directly.
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var in loginRequest
	if err := DecodeJSON(r, &in); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid request")
		return
	}
	if in.Email == "" || in.Password == "" {
		WriteError(w, http.StatusBadRequest, "email and password required")
		return
	}

	creds, found, err := h.resolveCredentials(r, in)
	if errors.Is(err, auth.ErrAmbiguousEmail) {
		h.respondTenantChoice(w, r, in.Email)
		return
	}
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if !found || !auth.VerifyPassword(creds.Hash, in.Password) {
		// Same message for unknown email + bad password (no user enumeration).
		// Log at warn for security visibility WITHOUT the email/password (PII).
		LoggerFrom(r.Context()).Warn("failed login attempt")
		WriteError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	// Suspended-tenant guard: a suspended tenant cannot log in (spec §3.1).
	status, ok, err := h.tenants.Status(r.Context(), creds.TenantID)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if !ok || status == auth.StatusSuspended {
		LoggerFrom(r.Context()).Warn("login blocked: tenant suspended",
			slog.Int64("tenant_id", creds.TenantID))
		WriteError(w, http.StatusForbidden, "tenant suspended")
		return
	}

	if err := h.sm.RenewToken(r.Context()); err != nil {
		WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	h.sm.Put(r.Context(), "userID", int(creds.ID))
	h.sm.Put(r.Context(), "tenantID", int(creds.TenantID))
	// Last-login is best-effort: login must not fail if recording it errors.
	if err := h.users.TouchLastLogin(r.Context(), creds.ID); err != nil {
		LoggerFrom(r.Context()).Warn("touch last login failed", slog.Any("error", err))
	}
	u, err := h.users.GetByID(r.Context(), creds.TenantID, creds.ID)
	if err != nil || u == nil {
		WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	WriteJSON(w, http.StatusOK, u)
}

// resolveCredentials looks up the login credentials, honouring an explicit
// tenantId when present and otherwise relying on the fail-safe global lookup
// (which returns ErrAmbiguousEmail when the email spans multiple tenants).
func (h *AuthHandler) resolveCredentials(r *http.Request, in loginRequest) (auth.Credentials, bool, error) {
	if in.TenantID != 0 {
		return h.users.GetCredentialsForTenant(r.Context(), in.TenantID, in.Email)
	}
	return h.users.GetCredentialsGlobal(r.Context(), in.Email)
}

// respondTenantChoice answers an ambiguous-email login with 409 and the list of
// tenants the email belongs to, so the client can re-submit with a tenantId.
func (h *AuthHandler) respondTenantChoice(w http.ResponseWriter, r *http.Request, email string) {
	tenants, err := h.users.TenantsForEmail(r.Context(), email)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	WriteJSON(w, http.StatusConflict, map[string]any{
		"error":          "tenant selection required",
		"tenantRequired": true,
		"tenants":        tenants,
	})
}

// Logout destroys the current session.
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	if err := h.sm.Destroy(r.Context()); err != nil {
		WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// Me returns the authenticated user placed on the context by RequireAuth.
func (h *AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	u := UserFrom(r.Context())
	if u == nil {
		WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	WriteJSON(w, http.StatusOK, u)
}
