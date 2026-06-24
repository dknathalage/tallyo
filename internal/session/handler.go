package session

import (
	"errors"
	"net/http"

	"github.com/dknathalage/tallyo/internal/billing"
	"github.com/dknathalage/tallyo/internal/httpx"
	"github.com/go-chi/chi/v5"
)

// Handler serves the session lifecycle routes: per-client listing,
// tenant-wide listing, the billing-suggestion and to-record prompts, plus session
// CRUD and the status-transition endpoint.
type Handler struct {
	svc *Service
}

// NewHandler constructs the handler. A nil svc is a programmer error.
func NewHandler(svc *Service) *Handler {
	if svc == nil {
		panic("session.NewHandler: nil svc")
	}
	return &Handler{svc: svc}
}

// Routes registers all session routes on r. Mounted inside the authenticated
// /api group by the composition root (server.go).
func (h *Handler) Routes(r chi.Router) {
	r.Get("/sessions", h.List)
	r.Get("/sessions/suggestions", h.Suggestions)
	r.Get("/sessions/to-record", h.ToRecord)
	r.Post("/sessions", h.Create)
	r.Get("/sessions/{sessionUUID}", h.Get)
	r.Put("/sessions/{sessionUUID}", h.Update)
	r.Delete("/sessions/{sessionUUID}", h.Delete)
	r.Post("/sessions/{sessionUUID}/status", h.UpdateStatus)
	r.Get("/sessions/{sessionUUID}/items", h.ListItems)
	r.Post("/sessions/{sessionUUID}/items", h.AddItem)
	r.Patch("/sessions/{sessionUUID}/items/{itemUUID}", h.UpdateItem)
	r.Delete("/sessions/{sessionUUID}/items/{itemUUID}", h.DeleteItem)
}

// List returns the tenant's sessions. With ?client={clientUUID} it
// returns only that client's sessions (resolving the client uuid to its
// int FK — this replaces the old nested client→sessions read). An optional
// ?status= filter restricts the lifecycle status in either mode.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	if clientUUID := r.URL.Query().Get("client"); clientUUID != "" {
		sessions, err := h.svc.ListByClientUUID(r.Context(), clientUUID, status)
		if err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "internal error")
			return
		}
		httpx.WriteJSON(w, http.StatusOK, sessions)
		return
	}
	sessions, err := h.svc.List(r.Context(), status)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, sessions)
}

// Suggestions returns each client's recorded-but-unbilled billing prompt.
func (h *Handler) Suggestions(w http.ResponseWriter, r *http.Request) {
	out, err := h.svc.Suggestions(r.Context())
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

// ToRecord returns the tenant's scheduled sessions still awaiting a record.
func (h *Handler) ToRecord(w http.ResponseWriter, r *http.Request) {
	out, err := h.svc.ToRecord(r.Context())
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

// sessionBody is the HTTP write shape of a session. ClientUUID arrives as the
// client's uuid (resolved to the int FK before insert/update); the other
// fields mirror SessionInput. It is the inbound DTO — SessionInput stays int-keyed
// for the cross-slice SessionCreator contract (agent import).
type sessionBody struct {
	ClientUUID  string   `json:"clientId"`
	ServiceDate string   `json:"serviceDate"`
	Note        string   `json:"note"`
	Tags        []string `json:"tags"`
	Status      string   `json:"status"`
}

// Get returns a single session by uuid, or 404 when not found.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	sessionUUID, ok := httpx.ParseUUID(r, "sessionUUID")
	if !ok {
		httpx.WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	sh, err := h.svc.GetByUUID(r.Context(), sessionUUID)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if sh == nil {
		httpx.WriteError(w, http.StatusNotFound, "not found")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, sh)
}

// Create inserts a session. A missing/unknown client uuid or service date → 400.
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var body sessionBody
	if err := httpx.DecodeJSON(r, &body); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request")
		return
	}
	if body.ClientUUID == "" {
		httpx.WriteError(w, http.StatusBadRequest, "client required")
		return
	}
	if body.ServiceDate == "" {
		httpx.WriteError(w, http.StatusBadRequest, "service date required")
		return
	}
	in, ok := h.resolveBody(w, r, body)
	if !ok {
		return
	}
	sh, err := h.svc.Create(r.Context(), in)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, sh)
}

// resolveBody translates a sessionBody's client uuid into the int FK and
// returns the int-keyed SessionInput. Writes a 400 and returns ok=false when the
// client uuid is unknown for the tenant.
func (h *Handler) resolveBody(w http.ResponseWriter, r *http.Request, body sessionBody) (SessionInput, bool) {
	pid, err := h.svc.ResolveClient(r.Context(), body.ClientUUID)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
		return SessionInput{}, false
	}
	if pid == "" {
		httpx.WriteError(w, http.StatusBadRequest, "unknown client")
		return SessionInput{}, false
	}
	return SessionInput{
		ClientID:    pid,
		ServiceDate: body.ServiceDate,
		Note:        body.Note,
		Tags:        body.Tags,
		Status:      body.Status,
	}, true
}

// Update mutates a session. Empty service date → 400; unknown session uuid → 404;
// unknown client uuid → 400.
func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	sessionUUID, ok := httpx.ParseUUID(r, "sessionUUID")
	if !ok {
		httpx.WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var body sessionBody
	if err := httpx.DecodeJSON(r, &body); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request")
		return
	}
	if body.ServiceDate == "" {
		httpx.WriteError(w, http.StatusBadRequest, "service date required")
		return
	}
	in, ok := h.resolveBody(w, r, body)
	if !ok {
		return
	}
	sh, err := h.svc.Update(r.Context(), sessionUUID, in)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if sh == nil {
		httpx.WriteError(w, http.StatusNotFound, "not found")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, sh)
}

// Delete removes a session by uuid.
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	sessionUUID, ok := httpx.ParseUUID(r, "sessionUUID")
	if !ok {
		httpx.WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	if err := h.svc.Delete(r.Context(), sessionUUID); err != nil {
		if errors.Is(err, ErrSessionBilled) {
			httpx.WriteError(w, http.StatusConflict, "cannot delete a billed session")
			return
		}
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ListItems returns a session's line items (billed + unbilled), [] when none.
// Unknown session uuid → 404.
func (h *Handler) ListItems(w http.ResponseWriter, r *http.Request) {
	sessionUUID, ok := httpx.ParseUUID(r, "sessionUUID")
	if !ok {
		httpx.WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	items, err := h.svc.ListItemsBySessionUUID(r.Context(), sessionUUID)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if items == nil {
		httpx.WriteError(w, http.StatusNotFound, "not found")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, items)
}

// AddItem adds one line item to a session. Unknown session → 404; invalid line → 400.
func (h *Handler) AddItem(w http.ResponseWriter, r *http.Request) {
	sessionUUID, ok := httpx.ParseUUID(r, "sessionUUID")
	if !ok {
		httpx.WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	in, ok := decodeItem(w, r)
	if !ok {
		return
	}
	item, err := h.svc.AddItemBySessionUUID(r.Context(), sessionUUID, in)
	if err != nil {
		if errors.Is(err, billing.ErrUnknownCustomItem) {
			httpx.WriteError(w, http.StatusBadRequest, "unknown custom item")
			return
		}
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if item == nil {
		httpx.WriteError(w, http.StatusNotFound, "not found")
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, item)
}

// UpdateItem rewrites one unbilled item addressed by uuid under its session uuid.
// Unknown/billed item → 404.
func (h *Handler) UpdateItem(w http.ResponseWriter, r *http.Request) {
	sessionUUID, ok := httpx.ParseUUID(r, "sessionUUID")
	if !ok {
		httpx.WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	itemUUID, ok := httpx.ParseUUID(r, "itemUUID")
	if !ok {
		httpx.WriteError(w, http.StatusBadRequest, "invalid item id")
		return
	}
	in, ok := decodeItem(w, r)
	if !ok {
		return
	}
	item, err := h.svc.UpdateItemBySessionUUID(r.Context(), sessionUUID, itemUUID, in)
	if err != nil {
		if errors.Is(err, billing.ErrUnknownCustomItem) {
			httpx.WriteError(w, http.StatusBadRequest, "unknown custom item")
			return
		}
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if item == nil {
		httpx.WriteError(w, http.StatusNotFound, "not found or already billed")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, item)
}

// DeleteItem removes one unbilled item by uuid under its session uuid (no-op when
// absent/billed).
func (h *Handler) DeleteItem(w http.ResponseWriter, r *http.Request) {
	sessionUUID, ok := httpx.ParseUUID(r, "sessionUUID")
	if !ok {
		httpx.WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	itemUUID, ok := httpx.ParseUUID(r, "itemUUID")
	if !ok {
		httpx.WriteError(w, http.StatusBadRequest, "invalid item id")
		return
	}
	if err := h.svc.DeleteItemBySessionUUID(r.Context(), sessionUUID, itemUUID); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// decodeItem decodes + validates a line-item body: quantity ≥ 0 and a line is
// either catalogue-coded or custom, not both. Writes a 400 and returns ok=false
// on failure.
func decodeItem(w http.ResponseWriter, r *http.Request) (billing.LineItemInput, bool) {
	var in billing.LineItemInput
	if err := httpx.DecodeJSON(r, &in); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request")
		return in, false
	}
	if in.Quantity < 0 {
		httpx.WriteError(w, http.StatusBadRequest, "quantity must not be negative")
		return in, false
	}
	if in.Code != "" && in.CustomItemID != nil {
		httpx.WriteError(w, http.StatusBadRequest, "a line is either catalogue-coded or custom, not both")
		return in, false
	}
	return in, true
}

// statusRequest is the body of UpdateStatus: the target lifecycle status.
type statusRequest struct {
	Status string `json:"status"`
}

// UpdateStatus advances a session's lifecycle status by uuid. An empty status → 400.
func (h *Handler) UpdateStatus(w http.ResponseWriter, r *http.Request) {
	sessionUUID, ok := httpx.ParseUUID(r, "sessionUUID")
	if !ok {
		httpx.WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var req statusRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request")
		return
	}
	if req.Status == "" {
		httpx.WriteError(w, http.StatusBadRequest, "status required")
		return
	}
	if err := h.svc.UpdateStatus(r.Context(), sessionUUID, req.Status); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
