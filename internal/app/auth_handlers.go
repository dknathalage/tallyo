package app

import (
	"errors"
	"log/slog"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/dknathalage/tallyo/internal/auth"
	"github.com/dknathalage/tallyo/internal/httpx"
	"github.com/dknathalage/tallyo/internal/reqctx"
	"github.com/go-chi/chi/v5"
)

// emailRe is a deliberately permissive email shape check: one @, non-empty
// local and domain parts, and a dot in the domain. Full RFC 5322 validation is
// neither necessary nor desirable at this boundary.
var emailRe = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)

// ============================================================================
// AuthConfigHandler (public) — GET /api/auth/config
// ============================================================================

// AuthConfigHandler serves the public Firebase web config + enabled sign-in
// methods so the SPA can initialize the Firebase JS SDK and decide which sign-in
// buttons to render. Everything it returns is public by design (the browser API
// key is not a secret). No auth.
type AuthConfigHandler struct{}

// NewAuthConfigHandler constructs the handler. It is stateless — all values are
// read from the environment per request so a config change does not require a
// rebuild.
func NewAuthConfigHandler() *AuthConfigHandler { return &AuthConfigHandler{} }

// Config returns { firebase:{apiKey,authDomain,projectId}, methods:{...} } from
// the FIREBASE_* and AUTH_*_ENABLED env vars.
func (h *AuthConfigHandler) Config(w http.ResponseWriter, _ *http.Request) {
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"firebase": map[string]string{
			"apiKey":     os.Getenv("FIREBASE_API_KEY"),
			"authDomain": os.Getenv("FIREBASE_AUTH_DOMAIN"),
			"projectId":  os.Getenv("FIREBASE_PROJECT_ID"),
		},
		"methods": map[string]bool{
			"emailPassword": envBool("AUTH_EMAIL_PASSWORD_ENABLED"),
			"google":        envBool("AUTH_GOOGLE_ENABLED"),
			"emailLink":     envBool("AUTH_EMAIL_LINK_ENABLED"),
		},
	})
}

// envBool reports whether an env var is the string "true" (case-insensitive).
// Anything else (unset, "false", garbage) is false.
func envBool(key string) bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv(key)), "true")
}

// ============================================================================
// AuthHandler — Bearer-authed session/me routes
// ============================================================================

// AuthHandler implements the authenticated "me" and "session" routes. Auth is
// stateless: RequireAuth verifies the bearer token upstream and places the uid +
// email on the context; this handler reads them. There is no login/logout
// endpoint (the client signs in/out with the Firebase SDK directly).
type AuthHandler struct {
	users   *auth.UsersRepo
	tenants *auth.TenantsRepo
}

// NewAuthHandler constructs the handler. Nil dependencies are programmer errors.
func NewAuthHandler(users *auth.UsersRepo, tenants *auth.TenantsRepo) *AuthHandler {
	if users == nil || tenants == nil {
		panic("NewAuthHandler: nil dep")
	}
	return &AuthHandler{users: users, tenants: tenants}
}

// Me returns the authenticated per-tenant user placed on the context by
// ResolveTenant.
func (h *AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	u := httpx.UserFrom(r.Context())
	if u == nil {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, u)
}

// Session returns the authenticated email and the tenants the token's uid
// belongs to (with per-tenant role). Tenant-AGNOSTIC: it powers the SPA
// bootstrap, the root redirect, and the tenant switcher before any tenant is
// selected.
func (h *AuthHandler) Session(w http.ResponseWriter, r *http.Request) {
	uid, ok := reqctx.FirebaseUIDFrom(r.Context())
	if !ok || uid == "" {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	email, _ := reqctx.EmailFrom(r.Context())
	tenants, err := h.users.TenantsForFirebaseUID(r.Context(), uid)
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
// validation and (Bearer-authed) acceptance routes the invitee uses.
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
// it cannot be confirmed/leaked via the response body. Public route.
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
// member user linked to the bearer token's uid, and marks the invite used in a
// single transaction. Bearer-authed: the invitee must have signed in with
// Firebase first (the token supplies the uid). An already-used or invalid token
// is rejected with 409, as is a uid already registered in the tenant.
func (h *InviteHandler) Accept(w http.ResponseWriter, r *http.Request) {
	uid, ok := reqctx.FirebaseUIDFrom(r.Context())
	if !ok || uid == "" {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	token := chi.URLParam(r, "token")
	var in struct {
		Name string `json:"name"`
	}
	if err := httpx.DecodeJSON(r, &in); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request")
		return
	}
	name := strings.TrimSpace(in.Name)
	if _, err := h.invites.Accept(r.Context(), token, name, uid); err != nil {
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

// SignupHandler serves the self-serve tenant signup flow: one request provisions
// a tenant + owner + business profile for the bearer token's identity.
// Bearer-authed: the caller must already have a Firebase account (created via the
// SDK with their chosen sign-in method); signup links that uid to a new tenant.
//
// Platform admins are NOT created here: is_platform_admin is orthogonal to the
// tenant role and is provisioned out-of-band, never via public signup.
type SignupHandler struct {
	tenants   *auth.TenantsRepo
	users     *auth.UsersRepo
	provision auth.ProfileProvisioner
}

// NewSignupHandler constructs the handler. Nil dependencies are programmer errors.
// provision creates the new tenant's business_profile in its own DB (DB-per-tenant).
func NewSignupHandler(tenants *auth.TenantsRepo, users *auth.UsersRepo, provision auth.ProfileProvisioner) *SignupHandler {
	if tenants == nil || users == nil || provision == nil {
		panic("NewSignupHandler: nil dep")
	}
	return &SignupHandler{tenants: tenants, users: users, provision: provision}
}

// signupRequest is the decoded body for Signup. The email + uid (and optional
// name) come from the verified token, not the body.
type signupRequest struct {
	BusinessName string `json:"businessName"`
	Name         string `json:"name"`
}

// Signup provisions a new tenant and its owner atomically. Bearer-authed: uid +
// email are taken from the verified token (RequireAuth). Returns the created
// owner user.
func (h *SignupHandler) Signup(w http.ResponseWriter, r *http.Request) {
	uid, ok := reqctx.FirebaseUIDFrom(r.Context())
	if !ok || uid == "" {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	email, _ := reqctx.EmailFrom(r.Context())
	email = strings.ToLower(strings.TrimSpace(email))
	if !emailRe.MatchString(email) {
		httpx.WriteError(w, http.StatusBadRequest, "token missing a valid email")
		return
	}

	var req signupRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request")
		return
	}
	req.BusinessName = strings.TrimSpace(req.BusinessName)
	req.Name = strings.TrimSpace(req.Name)
	if req.BusinessName == "" {
		httpx.WriteError(w, http.StatusBadRequest, "business name required")
		return
	}

	owner, err := h.tenants.Signup(r.Context(), auth.SignupInput{
		BusinessName: req.BusinessName,
		Email:        email,
		FirebaseUID:  uid,
		OwnerName:    req.Name,
	}, h.provision)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}

	httpx.LoggerFrom(r.Context()).Info("tenant signup",
		slog.String("tenant_id", owner.TenantID), slog.String("user_id", owner.ID))
	httpx.WriteJSON(w, http.StatusCreated, owner)
}
