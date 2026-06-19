package shift

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/dknathalage/tallyo/internal/billing"
	"github.com/dknathalage/tallyo/internal/httpx"
	"github.com/go-chi/chi/v5"
)

// Handler serves the shift lifecycle routes: per-participant listing,
// tenant-wide listing, the billing-suggestion and to-record prompts, plus shift
// CRUD and the status-transition endpoint.
type Handler struct {
	svc *Service
}

// NewHandler constructs the handler. A nil svc is a programmer error.
func NewHandler(svc *Service) *Handler {
	if svc == nil {
		panic("shift.NewHandler: nil svc")
	}
	return &Handler{svc: svc}
}

// Routes registers all shift routes on r. Mounted inside the authenticated
// /api group by the composition root (server.go).
func (h *Handler) Routes(r chi.Router) {
	r.Get("/participants/{id}/shifts", h.ListForParticipant)
	r.Get("/shifts", h.List)
	r.Get("/shifts/suggestions", h.Suggestions)
	r.Get("/shifts/to-record", h.ToRecord)
	r.Post("/shifts", h.Create)
	r.Get("/shifts/{id}", h.Get)
	r.Put("/shifts/{id}", h.Update)
	r.Delete("/shifts/{id}", h.Delete)
	r.Post("/shifts/{id}/status", h.UpdateStatus)
	r.Get("/shifts/{id}/items", h.ListItems)
	r.Post("/shifts/{id}/items", h.AddItem)
	r.Patch("/shifts/{id}/items/{itemId}", h.UpdateItem)
	r.Delete("/shifts/{id}/items/{itemId}", h.DeleteItem)
}

// ListForParticipant returns a participant's shifts, optionally restricted to the
// ?from=&to= service-date window and a ?status= filter. The chi {id} path param
// is the participant id.
func (h *Handler) ListForParticipant(w http.ResponseWriter, r *http.Request) {
	participantID, ok := httpx.ParseID(r)
	if !ok {
		httpx.WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	from := r.URL.Query().Get("from")
	to := r.URL.Query().Get("to")
	status := r.URL.Query().Get("status")
	shifts, err := h.svc.ListParticipant(r.Context(), participantID, from, to)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if status != "" {
		filtered := make([]*Shift, 0, len(shifts))
		for i := range shifts { // bounded by len(shifts)
			if shifts[i].Status == status {
				filtered = append(filtered, shifts[i])
			}
		}
		shifts = filtered
	}
	httpx.WriteJSON(w, http.StatusOK, shifts)
}

// List returns every shift for the acting tenant, optionally restricted to a
// ?status= filter (used to populate the shifts table).
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	shifts, err := h.svc.List(r.Context(), status)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, shifts)
}

// Suggestions returns each participant's recorded-but-unbilled billing prompt.
func (h *Handler) Suggestions(w http.ResponseWriter, r *http.Request) {
	out, err := h.svc.Suggestions(r.Context())
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

// ToRecord returns the tenant's scheduled shifts still awaiting a record.
func (h *Handler) ToRecord(w http.ResponseWriter, r *http.Request) {
	out, err := h.svc.ToRecord(r.Context())
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

// Get returns a single shift by id, or 404 when not found.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.ParseID(r)
	if !ok {
		httpx.WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	sh, err := h.svc.Get(r.Context(), id)
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

// Create inserts a shift. A missing participant or service date → 400.
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var in ShiftInput
	if err := httpx.DecodeJSON(r, &in); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request")
		return
	}
	if in.ParticipantID == 0 {
		httpx.WriteError(w, http.StatusBadRequest, "participant required")
		return
	}
	if in.ServiceDate == "" {
		httpx.WriteError(w, http.StatusBadRequest, "service date required")
		return
	}
	sh, err := h.svc.Create(r.Context(), in)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, sh)
}

// Update mutates a shift. Empty service date → 400; unknown id → 404.
func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.ParseID(r)
	if !ok {
		httpx.WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var in ShiftInput
	if err := httpx.DecodeJSON(r, &in); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request")
		return
	}
	if in.ServiceDate == "" {
		httpx.WriteError(w, http.StatusBadRequest, "service date required")
		return
	}
	sh, err := h.svc.Update(r.Context(), id, in)
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

// Delete removes a shift by id.
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.ParseID(r)
	if !ok {
		httpx.WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	if err := h.svc.Delete(r.Context(), id); err != nil {
		if errors.Is(err, ErrShiftBilled) {
			httpx.WriteError(w, http.StatusConflict, "cannot delete a billed shift")
			return
		}
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ListItems returns a shift's line items (billed + unbilled), [] when none.
func (h *Handler) ListItems(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.ParseID(r)
	if !ok {
		httpx.WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	items, err := h.svc.ListItems(r.Context(), id)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if items == nil {
		items = []*billing.LineItem{}
	}
	httpx.WriteJSON(w, http.StatusOK, items)
}

// AddItem adds one line item to a shift. Unknown shift → 404; invalid line → 400.
func (h *Handler) AddItem(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.ParseID(r)
	if !ok {
		httpx.WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	in, ok := decodeItem(w, r)
	if !ok {
		return
	}
	item, err := h.svc.AddItem(r.Context(), id, in)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if item == nil {
		httpx.WriteError(w, http.StatusNotFound, "not found")
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, item)
}

// UpdateItem rewrites one unbilled item. Unknown/billed item → 404.
func (h *Handler) UpdateItem(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.ParseID(r)
	if !ok {
		httpx.WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	itemID, ok := parseItemID(r)
	if !ok {
		httpx.WriteError(w, http.StatusBadRequest, "invalid item id")
		return
	}
	in, ok := decodeItem(w, r)
	if !ok {
		return
	}
	item, err := h.svc.UpdateItem(r.Context(), id, itemID, in)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if item == nil {
		httpx.WriteError(w, http.StatusNotFound, "not found or already billed")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, item)
}

// DeleteItem removes one unbilled item (no-op when absent/billed).
func (h *Handler) DeleteItem(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.ParseID(r)
	if !ok {
		httpx.WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	itemID, ok := parseItemID(r)
	if !ok {
		httpx.WriteError(w, http.StatusBadRequest, "invalid item id")
		return
	}
	if err := h.svc.DeleteItem(r.Context(), id, itemID); err != nil {
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

// parseItemID reads the {itemId} path param.
func parseItemID(r *http.Request) (int64, bool) {
	v, err := strconv.ParseInt(chi.URLParam(r, "itemId"), 10, 64)
	if err != nil || v <= 0 {
		return 0, false
	}
	return v, true
}

// statusRequest is the body of UpdateStatus: the target lifecycle status.
type statusRequest struct {
	Status string `json:"status"`
}

// UpdateStatus advances a shift's lifecycle status. An empty status → 400.
func (h *Handler) UpdateStatus(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.ParseID(r)
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
	if err := h.svc.UpdateStatus(r.Context(), id, req.Status); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
