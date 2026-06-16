package httpapi

import (
	"log/slog"
	"net/http"
	"regexp"
	"strings"

	"github.com/alexedwards/scs/v2"
	"github.com/dknathalage/tallyo/internal/auth"
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

// SignupHandler serves the public self-serve tenant signup flow: one request
// provisions a tenant + owner + business profile and logs the new owner in.
// It supersedes the old single-org first-run setup flow.
//
// Platform admins are NOT created here: is_platform_admin is orthogonal to the
// tenant role and is provisioned out-of-band (e.g. a future admin CLI or a
// direct DB update), never via public signup.
type SignupHandler struct {
	sm      *scs.SessionManager
	tenants *auth.TenantsRepo
	users   *auth.UsersRepo
}

// NewSignupHandler constructs the handler. Nil dependencies are programmer errors.
func NewSignupHandler(sm *scs.SessionManager, tenants *auth.TenantsRepo, users *auth.UsersRepo) *SignupHandler {
	if sm == nil || tenants == nil || users == nil {
		panic("NewSignupHandler: nil dep")
	}
	return &SignupHandler{sm: sm, tenants: tenants, users: users}
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
	if err := DecodeJSON(r, &req); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid request")
		return
	}
	if status, msg := validateSignup(&req); status != 0 {
		WriteError(w, status, msg)
		return
	}

	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	owner, err := h.tenants.Signup(r.Context(), auth.SignupInput{
		BusinessName: req.BusinessName,
		Email:        req.Email,
		PasswordHash: hash,
		OwnerName:    req.Name,
		Zone:         req.Zone,
	})
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Establish the session: renew the token (fixation defence) then store the
	// new owner's identity + tenant so the response is an authenticated session.
	if err := h.sm.RenewToken(r.Context()); err != nil {
		WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	h.sm.Put(r.Context(), "userID", int(owner.ID))
	h.sm.Put(r.Context(), "tenantID", int(owner.TenantID))
	LoggerFrom(r.Context()).Info("tenant signup",
		slog.Int64("tenant_id", owner.TenantID), slog.Int64("user_id", owner.ID))
	WriteJSON(w, http.StatusCreated, owner)
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
