package httpapi

import (
	"errors"
	"net/http"
	"time"

	"github.com/dknathalage/tallyo/internal/auth"
	"github.com/go-chi/chi/v5"
)

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
	u := UserFrom(r.Context())
	if u == nil {
		WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if u.Role != "owner" {
		WriteError(w, http.StatusForbidden, "owner only")
		return
	}
	var in struct {
		Email string `json:"email"`
		Role  string `json:"role"`
	}
	if err := DecodeJSON(r, &in); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid request")
		return
	}
	if in.Email == "" {
		WriteError(w, http.StatusBadRequest, "email is required")
		return
	}
	role := in.Role
	if role == "" {
		role = "member"
	}
	inv, err := h.invites.Create(r.Context(), u.TenantID, in.Email, role, u.ID, inviteTTL)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	WriteJSON(w, http.StatusCreated, map[string]string{
		"token":     inv.Token,
		"acceptUrl": "/accept-invite?token=" + inv.Token,
	})
}

// Validate reports whether a token is usable. It never echoes the token back so
// it cannot be confirmed/leaked via the response body.
func (h *InviteHandler) Validate(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	inv, err := h.invites.Validate(r.Context(), token)
	if errors.Is(err, auth.ErrInviteInvalid) {
		WriteError(w, http.StatusNotFound, "invite not found or expired")
		return
	}
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	WriteJSON(w, http.StatusOK, map[string]string{"email": inv.Email, "role": inv.Role})
}

// Accept consumes an invite atomically: it re-validates the invite, creates the
// member user, and marks the invite used in a single transaction. An already-used
// or invalid token is rejected with 409, as is an email already registered.
func (h *InviteHandler) Accept(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	var in struct {
		Password string `json:"password"`
	}
	if err := DecodeJSON(r, &in); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid request")
		return
	}
	if len(in.Password) < minPasswordLen {
		WriteError(w, http.StatusBadRequest, "password too short")
		return
	}
	hash, err := auth.HashPassword(in.Password)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	// TODO(J5): the invitee's display name should come from the accept form;
	// passed empty for now.
	if _, err := h.invites.Accept(r.Context(), token, "", hash); err != nil {
		if errors.Is(err, auth.ErrInviteInvalid) {
			WriteError(w, http.StatusConflict, "invite invalid or already used")
			return
		}
		if errors.Is(err, auth.ErrEmailTaken) {
			WriteError(w, http.StatusConflict, "email already registered")
			return
		}
		WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	WriteJSON(w, http.StatusCreated, map[string]string{"status": "ok"})
}
