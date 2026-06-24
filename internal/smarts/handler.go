package smarts

import (
	"errors"
	"net/http"

	"github.com/dknathalage/tallyo/internal/httpx"
	"github.com/go-chi/chi/v5"
)

// Handler exposes the Smarts as one POST per Smart. When disabled (no API key)
// every route returns 503 so the SPA's gated buttons never hit a 200 SPA
// fallback.
type Handler struct {
	svc     *Service
	enabled bool
}

// NewHandler builds the handler. svc may be nil when enabled is false (the
// guard-only handler used when AI is disabled).
func NewHandler(svc *Service, enabled bool) *Handler {
	if enabled && svc == nil {
		panic("smarts.NewHandler: enabled handler needs a service")
	}
	return &Handler{svc: svc, enabled: enabled}
}

// Routes registers the Smart routes. Mounted inside the tenant group, so every
// handler has a tenant in context. map-import follows the price-list import's
// owner/admin gate; the editable-draft Smarts are available to any tenant user.
func (h *Handler) Routes(r chi.Router) {
	r.Post("/smarts/draft-invoice", h.DraftInvoice)
	r.Post("/smarts/suggest-lines", h.SuggestLines)
	r.Post("/smarts/follow-up", h.FollowUp)
	r.With(httpx.RequireRole("owner", "admin")).Post("/smarts/map-import", h.MapImport)
}

// DraftInvoice: POST /smarts/draft-invoice {clientId} → {id} (new draft uuid).
func (h *Handler) DraftInvoice(w http.ResponseWriter, r *http.Request) {
	if h.guard(w) {
		return
	}
	var body struct {
		ClientID string `json:"clientId"`
	}
	if err := httpx.DecodeJSON(r, &body); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request")
		return
	}
	uuid, err := h.svc.DraftInvoiceFromSessions(r.Context(), body.ClientID)
	if err != nil {
		writeSmartError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"id": uuid})
}

// SuggestLines: POST /smarts/suggest-lines {note, serviceDate} → []LineItemInput.
func (h *Handler) SuggestLines(w http.ResponseWriter, r *http.Request) {
	if h.guard(w) {
		return
	}
	var in SuggestInput
	if err := httpx.DecodeJSON(r, &in); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request")
		return
	}
	lines, err := h.svc.SuggestLines(r.Context(), in)
	if err != nil {
		writeSmartError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, lines)
}

// FollowUp: POST /smarts/follow-up {invoiceId} → {subject, body}.
func (h *Handler) FollowUp(w http.ResponseWriter, r *http.Request) {
	if h.guard(w) {
		return
	}
	var body struct {
		InvoiceID string `json:"invoiceId"`
	}
	if err := httpx.DecodeJSON(r, &body); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request")
		return
	}
	fu, err := h.svc.DraftFollowUp(r.Context(), body.InvoiceID)
	if err != nil {
		writeSmartError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, fu)
}

// MapImport: POST /smarts/map-import {headers, rows} → {mappings}.
func (h *Handler) MapImport(w http.ResponseWriter, r *http.Request) {
	if h.guard(w) {
		return
	}
	var in MapInput
	if err := httpx.DecodeJSON(r, &in); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request")
		return
	}
	res, err := h.svc.MapImport(r.Context(), in)
	if err != nil {
		writeSmartError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, res)
}

// guard writes a 503 and returns true when AI is disabled.
func (h *Handler) guard(w http.ResponseWriter) bool {
	if !h.enabled {
		httpx.WriteError(w, http.StatusServiceUnavailable, "AI features are not configured")
		return true
	}
	return false
}

// writeSmartError maps a Smart error to an HTTP status. Data/precondition errors
// are 422 or 404; everything else (model failures) is 502 — never leak raw
// model/tool error strings.
func writeSmartError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		httpx.WriteError(w, http.StatusNotFound, "not found")
	case errors.Is(err, ErrNoData):
		httpx.WriteError(w, http.StatusUnprocessableEntity, err.Error())
	case errors.Is(err, ErrNoPriceList):
		httpx.WriteError(w, http.StatusUnprocessableEntity, "no price list is in effect for that date")
	default:
		httpx.WriteError(w, http.StatusBadGateway, "the AI couldn't complete this — try again")
	}
}
