package recurring

import (
	"errors"
	"net/http"

	"github.com/dknathalage/tallyo/internal/httpx"
	"github.com/dknathalage/tallyo/internal/listquery"
	"github.com/go-chi/chi/v5"
)

// Handler serves the recurring-template CRUD plus the generate route.
type Handler struct {
	svc *Service
}

// NewHandler constructs the handler. A nil svc is a programmer error.
func NewHandler(svc *Service) *Handler {
	if svc == nil {
		panic("recurring.NewHandler: nil svc")
	}
	return &Handler{svc: svc}
}

// Routes registers the recurring routes on r. Mounted inside the authenticated
// /api group by the composition root.
func (h *Handler) Routes(r chi.Router) {
	r.Get("/recurring", h.List)
	r.Post("/recurring", h.Create)
	r.Get("/recurring/{recurringUUID}", h.Get)
	r.Put("/recurring/{recurringUUID}", h.Update)
	r.Delete("/recurring/{recurringUUID}", h.Delete)
	r.Post("/recurring/{recurringUUID}/generate", h.Generate)
}

// List returns templates. With DataTable query params (sort/page/limit/f.*) it
// returns a paged {rows,total}; otherwise it keeps the legacy array, where
// ?active=true (or 1) returns only active templates.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if listquery.IsListQuery(q) {
		c := listquery.Build(q, RecurringCols)
		res, err := h.svc.Query(r.Context(), c)
		if err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "internal error")
			return
		}
		httpx.WriteJSON(w, http.StatusOK, res)
		return
	}
	active := q.Get("active")
	activeOnly := active == "true" || active == "1"
	tpls, err := h.svc.List(r.Context(), activeOnly)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, tpls)
}

// Get returns a single template by uuid, or 404 when not found.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.ParseUUID(r, "recurringUUID")
	if !ok {
		httpx.WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	tpl, err := h.svc.Get(r.Context(), id)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if tpl == nil {
		httpx.WriteError(w, http.StatusNotFound, "not found")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, tpl)
}

// validateRecurring checks the writable fields at the request boundary.
func validateRecurring(in RecurringInput) (string, bool) {
	if in.Name == "" {
		return "name required", false
	}
	if in.ClientUUID == nil || *in.ClientUUID == "" {
		return "client required", false
	}
	if in.Frequency == "" {
		return "frequency required", false
	}
	return "", true
}

// Create inserts a template after validating name, client and frequency.
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var in RecurringInput
	if err := httpx.DecodeJSON(r, &in); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request")
		return
	}
	if msg, ok := validateRecurring(in); !ok {
		httpx.WriteError(w, http.StatusBadRequest, msg)
		return
	}
	tpl, err := h.svc.Create(r.Context(), in)
	if errors.Is(err, errClientNotFound) {
		httpx.WriteError(w, http.StatusBadRequest, "unknown client")
		return
	}
	if errors.Is(err, errPlanManagerNotFound) {
		httpx.WriteError(w, http.StatusBadRequest, "unknown plan manager")
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, tpl)
}

// Update rewrites a template by uuid. Validation fails → 400; unknown id → 404;
// unknown client/plan-manager uuid → 400.
func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.ParseUUID(r, "recurringUUID")
	if !ok {
		httpx.WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var in RecurringInput
	if err := httpx.DecodeJSON(r, &in); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request")
		return
	}
	if msg, ok := validateRecurring(in); !ok {
		httpx.WriteError(w, http.StatusBadRequest, msg)
		return
	}
	tpl, err := h.svc.Update(r.Context(), id, in)
	if errors.Is(err, errClientNotFound) {
		httpx.WriteError(w, http.StatusBadRequest, "unknown client")
		return
	}
	if errors.Is(err, errPlanManagerNotFound) {
		httpx.WriteError(w, http.StatusBadRequest, "unknown plan manager")
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if tpl == nil {
		httpx.WriteError(w, http.StatusNotFound, "not found")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, tpl)
}

// Delete removes a template by uuid.
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.ParseUUID(r, "recurringUUID")
	if !ok {
		httpx.WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	if err := h.svc.Delete(r.Context(), id); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Generate produces a draft invoice from the template and advances its next_due.
// Unknown id → 404; otherwise the generated invoice is returned with 200.
func (h *Handler) Generate(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.ParseUUID(r, "recurringUUID")
	if !ok {
		httpx.WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	inv, err := h.svc.GenerateOne(r.Context(), id)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if inv == nil {
		httpx.WriteError(w, http.StatusNotFound, "not found")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, inv)
}
