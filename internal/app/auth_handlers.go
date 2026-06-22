package app

import (
	"errors"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/alexedwards/scs/v2"
	"github.com/dknathalage/tallyo/internal/auth"
	"github.com/dknathalage/tallyo/internal/httpx"
	"github.com/go-chi/chi/v5"
)

// minPasswordLen is the minimum acceptable password length for signup and
// invite acceptance.
const minPasswordLen = 8

// emailRe is a deliberately permissive email shape check: one @, non-empty
// local and domain parts, and a dot in the domain. Full RFC 5322 validation is
// neither necessary nor desirable at this boundary.
var emailRe = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)

// allowedZones is the set of valid NDIS pricing zones for signup.
var allowedZones = map[string]bool{
	"national":    true,
	"remote":      true,
	"very_remote": true,
}

// ============================================================================
// AuthHandler
// ============================================================================

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

// loginRequest is the decoded login body. tenantId is optional: it is REQUIRED
// only to disambiguate when the email is registered in more than one tenant. It
// carries the tenant's public UUID (matching the 409 response's tenant id), which
// the login handler resolves to the internal tenant PK before looking up creds.
type loginRequest struct {
	Email      string `json:"email"`
	Password   string `json:"password"`
	TenantUUID string `json:"tenantId"`
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
	if err := httpx.DecodeJSON(r, &in); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request")
		return
	}
	if in.Email == "" || in.Password == "" {
		httpx.WriteError(w, http.StatusBadRequest, "email and password required")
		return
	}

	creds, found, err := h.resolveCredentials(r, in)
	if errors.Is(err, auth.ErrAmbiguousEmail) {
		h.respondTenantChoice(w, r, in.Email)
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if !found || !auth.VerifyPassword(creds.Hash, in.Password) {
		// Same message for unknown email + bad password (no user enumeration).
		// Log at warn for security visibility WITHOUT the email/password (PII).
		httpx.LoggerFrom(r.Context()).Warn("failed login attempt")
		httpx.WriteError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	// Suspended-tenant guard: a suspended tenant cannot log in (spec §3.1).
	status, ok, err := h.tenants.Status(r.Context(), creds.TenantID)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if !ok || status == auth.StatusSuspended {
		httpx.LoggerFrom(r.Context()).Warn("login blocked: tenant suspended",
			slog.Int64("tenant_id", creds.TenantID))
		httpx.WriteError(w, http.StatusForbidden, "tenant suspended")
		return
	}

	if err := h.sm.RenewToken(r.Context()); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	h.sm.Put(r.Context(), "userID", int(creds.ID))
	h.sm.Put(r.Context(), "tenantID", int(creds.TenantID))
	// Email is the durable cross-tenant identity used by ResolveTenant to
	// authorize the URL tenant per request. Normalize to match stored email.
	h.sm.Put(r.Context(), "email", strings.ToLower(strings.TrimSpace(in.Email)))
	// Last-login is best-effort: login must not fail if recording it errors.
	if err := h.users.TouchLastLogin(r.Context(), creds.ID); err != nil {
		httpx.LoggerFrom(r.Context()).Warn("touch last login failed", slog.Any("error", err))
	}
	u, err := h.users.GetByID(r.Context(), creds.TenantID, creds.ID)
	if err != nil || u == nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, u)
}

// resolveCredentials looks up the login credentials, honouring an explicit
// tenant uuid when present and otherwise relying on the fail-safe global lookup
// (which returns ErrAmbiguousEmail when the email spans multiple tenants). When a
// tenant uuid is supplied it is resolved to the internal tenant PK first; an
// unknown uuid yields a not-found result (→ 401, no user enumeration).
func (h *AuthHandler) resolveCredentials(r *http.Request, in loginRequest) (auth.Credentials, bool, error) {
	if in.TenantUUID != "" {
		t, err := h.tenants.GetByUUID(r.Context(), in.TenantUUID)
		if err != nil {
			return auth.Credentials{}, false, err
		}
		if t == nil {
			return auth.Credentials{}, false, nil
		}
		return h.users.GetCredentialsForTenant(r.Context(), t.ID, in.Email)
	}
	return h.users.GetCredentialsGlobal(r.Context(), in.Email)
}

// respondTenantChoice answers an ambiguous-email login with 409 and the list of
// tenants the email belongs to, so the client can re-submit with a tenantId.
func (h *AuthHandler) respondTenantChoice(w http.ResponseWriter, r *http.Request, email string) {
	tenants, err := h.users.TenantsForEmail(r.Context(), email)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	httpx.WriteJSON(w, http.StatusConflict, map[string]any{
		"error":          "tenant selection required",
		"tenantRequired": true,
		"tenants":        tenants,
	})
}

// Logout destroys the current session.
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	if err := h.sm.Destroy(r.Context()); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// Me returns the authenticated user placed on the context by RequireAuth.
func (h *AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	u := httpx.UserFrom(r.Context())
	if u == nil {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, u)
}

// Session returns the authenticated email and the tenants it belongs to (with
// per-tenant role). Tenant-AGNOSTIC: it powers the SPA bootstrap, the root
// redirect, and the tenant switcher before any tenant is selected.
func (h *AuthHandler) Session(w http.ResponseWriter, r *http.Request) {
	email := h.sm.GetString(r.Context(), "email")
	if email == "" {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	tenants, err := h.users.TenantsForEmail(r.Context(), email)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"email": email, "tenants": tenants})
}

// ============================================================================
// InviteHandler
// ============================================================================

// inviteTTL is the lifetime of a freshly minted invite.
const inviteTTL = 7 * 24 * time.Hour

// InviteHandler implements owner-only invite creation plus the public
// validation and acceptance routes the invitee uses.
type InviteHandler struct {
	invites *auth.InvitesRepo
	users   *auth.UsersRepo
}

// NewInviteHandler constructs the handler. Nil dependencies are programmer errors.
func NewInviteHandler(invites *auth.InvitesRepo, users *auth.UsersRepo) *InviteHandler {
	if invites == nil || users == nil {
		panic("NewInviteHandler: nil dep")
	}
	return &InviteHandler{invites: invites, users: users}
}

// Create mints an invite. Owner-only: the caller's role is checked before any
// write. The response carries the token and a relative SPA accept URL.
func (h *InviteHandler) Create(w http.ResponseWriter, r *http.Request) {
	u := httpx.UserFrom(r.Context())
	if u == nil {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if u.Role != "owner" && u.Role != "admin" {
		httpx.WriteError(w, http.StatusForbidden, "owner or admin only")
		return
	}
	var in struct {
		Email string `json:"email"`
		Role  string `json:"role"`
	}
	if err := httpx.DecodeJSON(r, &in); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request")
		return
	}
	if in.Email == "" {
		httpx.WriteError(w, http.StatusBadRequest, "email is required")
		return
	}
	role := in.Role
	if role == "" {
		role = "member"
	}
	if role != "owner" && role != "admin" && role != "member" {
		httpx.WriteError(w, http.StatusBadRequest, "invalid role")
		return
	}
	inv, err := h.invites.Create(r.Context(), u.TenantID, in.Email, role, u.ID, inviteTTL)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, map[string]string{
		"token":     inv.Token,
		"acceptUrl": "/accept-invite?token=" + inv.Token,
	})
}

// Revoke deletes a pending invite by its uuid, tenant-scoped. Owner/admin only
// (gated at the route). An unknown uuid is a no-op delete; we return 204 either
// way so revoke is idempotent.
func (h *InviteHandler) Revoke(w http.ResponseWriter, r *http.Request) {
	u := httpx.UserFrom(r.Context())
	if u == nil {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	inviteUUID, ok := httpx.ParseUUID(r, "inviteUUID")
	if !ok {
		httpx.WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	if err := h.invites.DeleteByUUID(r.Context(), u.TenantID, inviteUUID); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Validate reports whether a token is usable. It never echoes the token back so
// it cannot be confirmed/leaked via the response body.
func (h *InviteHandler) Validate(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	inv, err := h.invites.Validate(r.Context(), token)
	if errors.Is(err, auth.ErrInviteInvalid) {
		httpx.WriteError(w, http.StatusNotFound, "invite not found or expired")
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"email": inv.Email, "role": inv.Role})
}

// Accept consumes an invite atomically: it re-validates the invite, creates the
// member user, and marks the invite used in a single transaction. An already-used
// or invalid token is rejected with 409, as is an email already registered.
func (h *InviteHandler) Accept(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	var in struct {
		Name     string `json:"name"`
		Password string `json:"password"`
	}
	if err := httpx.DecodeJSON(r, &in); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request")
		return
	}
	name := strings.TrimSpace(in.Name)
	if name == "" {
		httpx.WriteError(w, http.StatusBadRequest, "name required")
		return
	}
	if len(in.Password) < minPasswordLen {
		httpx.WriteError(w, http.StatusBadRequest, "password too short")
		return
	}
	hash, err := auth.HashPassword(in.Password)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if _, err := h.invites.Accept(r.Context(), token, name, hash); err != nil {
		if errors.Is(err, auth.ErrInviteInvalid) {
			httpx.WriteError(w, http.StatusConflict, "invite invalid or already used")
			return
		}
		if errors.Is(err, auth.ErrEmailTaken) {
			httpx.WriteError(w, http.StatusConflict, "email already registered")
			return
		}
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, map[string]string{"status": "ok"})
}

// ============================================================================
// SignupHandler
// ============================================================================

// SignupHandler serves the public self-serve tenant signup flow: one request
// provisions a tenant + owner + business profile and logs the new owner in.
// It supersedes the old single-org first-run setup flow.
//
// Platform admins are NOT created here: is_platform_admin is orthogonal to the
// tenant role and is provisioned out-of-band (e.g. a future admin CLI or a
// direct DB update), never via public signup.
type SignupHandler struct {
	sm        *scs.SessionManager
	tenants   *auth.TenantsRepo
	users     *auth.UsersRepo
	provision auth.ProfileProvisioner
}

// NewSignupHandler constructs the handler. Nil dependencies are programmer errors.
// provision creates the new tenant's business_profile in its own DB (DB-per-tenant).
func NewSignupHandler(sm *scs.SessionManager, tenants *auth.TenantsRepo, users *auth.UsersRepo, provision auth.ProfileProvisioner) *SignupHandler {
	if sm == nil || tenants == nil || users == nil || provision == nil {
		panic("NewSignupHandler: nil dep")
	}
	return &SignupHandler{sm: sm, tenants: tenants, users: users, provision: provision}
}

// signupRequest is the decoded body for Signup. zone defaults to "national".
type signupRequest struct {
	BusinessName string `json:"businessName"`
	Name         string `json:"name"`
	Email        string `json:"email"`
	Password     string `json:"password"`
	Zone         string `json:"zone"`
}

// Signup provisions a new tenant and its owner atomically, then establishes the
// session so the caller lands logged in. Public route (no auth).
func (h *SignupHandler) Signup(w http.ResponseWriter, r *http.Request) {
	var req signupRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request")
		return
	}
	if status, msg := validateSignup(&req); status != 0 {
		httpx.WriteError(w, status, msg)
		return
	}

	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	owner, err := h.tenants.Signup(r.Context(), auth.SignupInput{
		BusinessName: req.BusinessName,
		Email:        req.Email,
		PasswordHash: hash,
		OwnerName:    req.Name,
		Zone:         req.Zone,
	}, h.provision)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Establish the session: renew the token (fixation defence) then store the
	// new owner's identity + tenant so the response is an authenticated session.
	if err := h.sm.RenewToken(r.Context()); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	h.sm.Put(r.Context(), "userID", int(owner.ID))
	h.sm.Put(r.Context(), "tenantID", int(owner.TenantID))
	// Email identity for ResolveTenant (owner.Email is already normalized).
	h.sm.Put(r.Context(), "email", owner.Email)
	httpx.LoggerFrom(r.Context()).Info("tenant signup",
		slog.Int64("tenant_id", owner.TenantID), slog.Int64("user_id", owner.ID))
	httpx.WriteJSON(w, http.StatusCreated, owner)
}

// validateSignup checks the request fields at the boundary. It returns a zero
// status when valid, otherwise the HTTP status and message to write. It also
// normalizes whitespace + email casing in place.
func validateSignup(req *signupRequest) (int, string) {
	req.BusinessName = strings.TrimSpace(req.BusinessName)
	req.Name = strings.TrimSpace(req.Name)
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	req.Zone = strings.TrimSpace(req.Zone)
	if req.BusinessName == "" {
		return http.StatusBadRequest, "business name required"
	}
	if !emailRe.MatchString(req.Email) {
		return http.StatusBadRequest, "valid email required"
	}
	if len(req.Password) < minPasswordLen {
		return http.StatusBadRequest, "password too short"
	}
	if req.Zone == "" {
		req.Zone = "national"
	}
	if !allowedZones[req.Zone] {
		return http.StatusBadRequest, "invalid zone"
	}
	return 0, ""
}
