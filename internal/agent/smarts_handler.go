package agent

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/dknathalage/tallyo/internal/httpx"
	"github.com/dknathalage/tallyo/internal/reqctx"
)

// SmartsHandler serves the one-shot AI "Smarts" routes: import-shifts (the
// per-session divide route is served by the session handler via a SessionDivider
// interface). Every handler 503s when the feature is disabled.
type SmartsHandler struct {
	smarts  *Smarts
	enabled bool
}

// NewSmartsHandler constructs the handler. When enabled is true a non-nil Smarts
// is required (programmer error otherwise); when disabled the handler is still
// registered but every route returns 503, keeping wiring uniform.
func NewSmartsHandler(s *Smarts, enabled bool) *SmartsHandler {
	if enabled && s == nil {
		panic("NewSmartsHandler: enabled handler requires a non-nil Smarts")
	}
	return &SmartsHandler{smarts: s, enabled: enabled}
}

// importShiftsRequest is the body of ImportShifts: the client the shifts
// are for, and the free-text timesheet to extract per-day rows from.
type importShiftsRequest struct {
	ClientID int64  `json:"clientId"`
	Text     string `json:"text"`
}

// guard enforces the enabled flag and pulls the authenticated tenant+user from
// the request context (attached upstream by RequireAuth). It returns ok=false
// after writing the appropriate error response.
func (h *SmartsHandler) guard(w http.ResponseWriter, r *http.Request) (tenantID, userID int64, ok bool) {
	if !h.enabled {
		httpx.WriteError(w, http.StatusServiceUnavailable, "AI not configured")
		return 0, 0, false
	}
	tid, tok := reqctx.TenantFrom(r.Context())
	if !tok || tid <= 0 {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return 0, 0, false
	}
	uid, uok := reqctx.UserFrom(r.Context())
	if !uok || uid <= 0 {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return 0, 0, false
	}
	return tid, uid, true
}

// ImportShifts turns a free-text timesheet into recorded shifts for one
// client via the import-shifts Smart. Returns the created shifts.
func (h *SmartsHandler) ImportShifts(w http.ResponseWriter, r *http.Request) {
	tenantID, userID, ok := h.guard(w, r)
	if !ok {
		return
	}
	var req importShiftsRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.ClientID <= 0 {
		httpx.WriteError(w, http.StatusBadRequest, "clientId is required")
		return
	}
	if strings.TrimSpace(req.Text) == "" {
		httpx.WriteError(w, http.StatusBadRequest, "text is required")
		return
	}

	ctx, cancel := context.WithTimeout(detach(tenantID, userID), 2*time.Minute)
	defer cancel()

	created, err := h.smarts.ImportShifts(ctx, req.ClientID, req.Text)
	if err != nil {
		slog.Error("import shifts", slog.Any("error", err))
		httpx.WriteError(w, http.StatusBadGateway, "could not extract shifts from the timesheet")
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, created)
}

// detach derives a fresh background context carrying the request's tenant+user.
// The request context is canceled when the handler returns, so a goroutine or
// blocking model call that outlives the request must NOT inherit it.
func detach(tenantID, userID int64) context.Context {
	return reqctx.WithUser(reqctx.WithTenant(context.Background(), tenantID), userID)
}
