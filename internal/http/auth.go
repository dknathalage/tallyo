package httpapi

import (
	"log"
	"net/http"

	"github.com/alexedwards/scs/v2"
	"github.com/dknathalage/tallyo/internal/auth"
)

// AuthHandler implements session login/logout and the authenticated "me" route.
type AuthHandler struct {
	sm    *scs.SessionManager
	users *auth.UsersRepo
}

// NewAuthHandler constructs the handler. Nil dependencies are programmer errors.
func NewAuthHandler(sm *scs.SessionManager, users *auth.UsersRepo) *AuthHandler {
	if sm == nil || users == nil {
		panic("NewAuthHandler: nil dep")
	}
	return &AuthHandler{sm: sm, users: users}
}

// Login verifies credentials and establishes a session. It uses a single error
// message for both unknown email and bad password to avoid user enumeration,
// and renews the session token to prevent session fixation.
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := DecodeJSON(r, &in); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid request")
		return
	}
	if in.Email == "" || in.Password == "" {
		WriteError(w, http.StatusBadRequest, "email and password required")
		return
	}
	id, hash, found, err := h.users.GetCredentials(r.Context(), in.Email)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if !found || !auth.VerifyPassword(hash, in.Password) {
		// Same message for unknown email + bad password (no user enumeration).
		WriteError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	if err := h.sm.RenewToken(r.Context()); err != nil {
		WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	h.sm.Put(r.Context(), "userID", int(id))
	// Last-login is best-effort: login must not fail if recording it errors.
	if err := h.users.TouchLastLogin(r.Context(), id); err != nil {
		log.Printf("Login: touch last login: %v", err)
	}
	u, err := h.users.GetByID(r.Context(), id)
	if err != nil || u == nil {
		WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	WriteJSON(w, http.StatusOK, u)
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
