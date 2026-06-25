package taxrate

import (
	"net/http"

	"github.com/dknathalage/tallyo/internal/httpx"
	"github.com/dknathalage/tallyo/internal/listquery"
	"github.com/go-chi/chi/v5"
)

// Handler serves the tax-rate CRUD routes.
type Handler struct {
	svc *Service
}

// NewHandler constructs the handler. A nil svc is a programmer error.
func NewHandler(svc *Service) *Handler {
	if svc == nil {
		panic("taxrate.NewHandler: nil svc")
	}
	return &Handler{svc: svc}
}

// Routes registers the tax-rate routes on r. Mounted inside the authenticated
// /api group by the composition root.
func (h *Handler) Routes(r chi.Router) {
	r.Get("/tax-rates", h.List)
	r.Post("/tax-rates", h.Create)
	r.Get("/tax-rates/{uuid}", h.Get)
	r.Put("/tax-rates/{uuid}", h.Update)
	r.Delete("/tax-rates/{uuid}", h.Delete)
}

// List returns tax rates. With DataTable query params (sort/page/limit/f.*) it
// returns a paged {rows,total}; otherwise it keeps the legacy full JSON array.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if listquery.IsListQuery(q) {
		c := listquery.Build(q, TaxRateCols)
		res, err := h.svc.Query(r.Context(), c)
		if err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "internal error")
			return
		}
		httpx.WriteJSON(w, http.StatusOK, res)
		return
	}
	rates, err := h.svc.List(r.Context())
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, rates)
}

// Get returns a single tax rate by uuid, or 404 when not found.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.ParseUUID(r, "uuid")
	if !ok {
		httpx.WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	t, err := h.svc.Get(r.Context(), id)
	if httpx.WriteServiceError(w, err) {
		return
	}
	httpx.WriteJSON(w, http.StatusOK, t)
}

// Create inserts a tax rate. An empty name is rejected with 422.
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var in TaxRateInput
	if err := httpx.DecodeJSON(r, &in); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request")
		return
	}
	t, err := h.svc.Create(r.Context(), in)
	if httpx.WriteServiceError(w, err) {
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, t)
}

// Update mutates a tax rate. Empty name → 422; unknown uuid → 404.
func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.ParseUUID(r, "uuid")
	if !ok {
		httpx.WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var in TaxRateInput
	if err := httpx.DecodeJSON(r, &in); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request")
		return
	}
	t, err := h.svc.Update(r.Context(), id, in)
	if httpx.WriteServiceError(w, err) {
		return
	}
	httpx.WriteJSON(w, http.StatusOK, t)
}

// Delete removes a tax rate by uuid. Unknown uuid → 404.
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.ParseUUID(r, "uuid")
	if !ok {
		httpx.WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	if err := h.svc.Delete(r.Context(), id); httpx.WriteServiceError(w, err) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
