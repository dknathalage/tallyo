package httpapi

import (
	"context"
	"errors"
	"net/http"
	"sync/atomic"

	"github.com/dknathalage/tallyo/internal/auth"
)

// minPasswordLen is the minimum acceptable owner password length.
const minPasswordLen = 8

// errNilUsers is returned when a SetupHandler is constructed without a repo.
var errNilUsers = errors.New("httpapi.NewSetupHandler: users and tenants repos are required")

// SetupHandler serves the first-run setup flow: owner creation while there are
// zero tenants, then a permanent 409 once an owner exists. It caches the
// "owner exists" state in an atomic flag to avoid a COUNT per request, with a
// re-guard against the create race.
//
// TODO(J5): full signup/onboarding (multi-tenant provisioning, roles, suspended
// guard) is owned by J5. This handler implements the minimal single-owner
// first-run path: create one tenant, then the platform-admin owner inside it.
type SetupHandler struct {
	users       *auth.UsersRepo
	tenants     *auth.TenantsRepo
	ownerExists atomic.Bool
}

// NewSetupHandler constructs the handler and initializes the cached flag from
// the current tenant count. Nil repos are programmer errors.
func NewSetupHandler(users *auth.UsersRepo, tenants *auth.TenantsRepo) (*SetupHandler, error) {
	if users == nil || tenants == nil {
		return nil, errNilUsers
	}
	n, err := tenants.Count(context.Background())
	if err != nil {
		return nil, err
	}
	h := &SetupHandler{users: users, tenants: tenants}
	h.ownerExists.Store(n > 0)
	return h, nil
}

// OwnerExists reports whether an owner account exists. Exported for use by
// later middleware that gates routes during first-run.
func (h *SetupHandler) OwnerExists() bool {
	return h.ownerExists.Load()
}

// Status reports whether an owner account exists.
func (h *SetupHandler) Status(w http.ResponseWriter, _ *http.Request) {
	WriteJSON(w, http.StatusOK, map[string]bool{"ownerExists": h.ownerExists.Load()})
}

// createOwnerRequest is the decoded body for CreateOwner.
type createOwnerRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// CreateOwner creates the first user as the owner. It is blocked with 409 once
// any user exists.
func (h *SetupHandler) CreateOwner(w http.ResponseWriter, r *http.Request) {
	if h.ownerExists.Load() {
		WriteError(w, http.StatusConflict, "owner already exists")
		return
	}

	var req createOwnerRequest
	if err := DecodeJSON(r, &req); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid request")
		return
	}
	if status, msg := validateOwner(req); status != 0 {
		WriteError(w, status, msg)
		return
	}

	ctx := r.Context()
	n, err := h.tenants.Count(ctx)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if n > 0 {
		h.ownerExists.Store(true)
		WriteError(w, http.StatusConflict, "owner already exists")
		return
	}

	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	// First-run: provision one tenant, then the platform-admin owner inside it.
	// TODO(J5): tenant name should come from the signup form; default for now.
	tenant, err := h.tenants.Create(ctx, req.Email)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	u, err := h.users.Create(ctx, tenant.ID, req.Email, hash, "", "owner", true)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	h.ownerExists.Store(true)
	WriteJSON(w, http.StatusCreated, u)
}

// validateOwner checks the request fields. It returns a zero status when valid,
// otherwise the HTTP status and message to write.
func validateOwner(req createOwnerRequest) (int, string) {
	if req.Email == "" {
		return http.StatusBadRequest, "email required"
	}
	if len(req.Password) < minPasswordLen {
		return http.StatusBadRequest, "password too short"
	}
	return 0, ""
}
