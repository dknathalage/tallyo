package httpapi

import (
	"net/http"
	"strconv"

	"github.com/dknathalage/tallyo/internal/repository"
	"github.com/dknathalage/tallyo/internal/service"
	"github.com/go-chi/chi/v5"
)

// CatalogHandler serves catalog CRUD, bulk-delete, categories, and the
// per-item tier-rate sub-routes.
type CatalogHandler struct {
	svc *service.CatalogService
}

// NewCatalogHandler constructs the handler. A nil svc is a programmer error.
func NewCatalogHandler(svc *service.CatalogService) *CatalogHandler {
	if svc == nil {
		panic("NewCatalogHandler: nil svc")
	}
	return &CatalogHandler{svc: svc}
}

// List returns catalog items; ?search= switches to a name/sku search.
func (h *CatalogHandler) List(w http.ResponseWriter, r *http.Request) {
	search := r.URL.Query().Get("search")
	var (
		items []*repository.CatalogItem
		err   error
	)
	if search != "" {
		items, err = h.svc.Search(r.Context(), search)
	} else {
		items, err = h.svc.List(r.Context())
	}
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	WriteJSON(w, http.StatusOK, items)
}

// Get returns a single catalog item by id, or 404 when not found.
func (h *CatalogHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r)
	if !ok {
		WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	it, err := h.svc.Get(r.Context(), id)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if it == nil {
		WriteError(w, http.StatusNotFound, "not found")
		return
	}
	WriteJSON(w, http.StatusOK, it)
}

// Create inserts a catalog item. An empty name is rejected with 400.
func (h *CatalogHandler) Create(w http.ResponseWriter, r *http.Request) {
	var in repository.CatalogItemInput
	if err := DecodeJSON(r, &in); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid request")
		return
	}
	if in.Name == "" {
		WriteError(w, http.StatusBadRequest, "name required")
		return
	}
	it, err := h.svc.Create(r.Context(), in)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	WriteJSON(w, http.StatusCreated, it)
}

// Update mutates a catalog item. Empty name → 400; unknown id → 404.
func (h *CatalogHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r)
	if !ok {
		WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var in repository.CatalogItemInput
	if err := DecodeJSON(r, &in); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid request")
		return
	}
	if in.Name == "" {
		WriteError(w, http.StatusBadRequest, "name required")
		return
	}
	it, err := h.svc.Update(r.Context(), id, in)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if it == nil {
		WriteError(w, http.StatusNotFound, "not found")
		return
	}
	WriteJSON(w, http.StatusOK, it)
}

// Delete removes a catalog item by id.
func (h *CatalogHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r)
	if !ok {
		WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	if err := h.svc.Delete(r.Context(), id); err != nil {
		WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// BulkDelete removes every catalog item whose id is in the request body.
func (h *CatalogHandler) BulkDelete(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Ids []int64 `json:"ids"`
	}
	if err := DecodeJSON(r, &body); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid request")
		return
	}
	if err := h.svc.BulkDelete(r.Context(), body.Ids); err != nil {
		WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Categories returns the distinct catalog categories as a JSON array.
func (h *CatalogHandler) Categories(w http.ResponseWriter, r *http.Request) {
	cats, err := h.svc.Categories(r.Context())
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	WriteJSON(w, http.StatusOK, cats)
}

// GetRates returns the per-tier overrides for a catalog item.
func (h *CatalogHandler) GetRates(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r)
	if !ok {
		WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	rates, err := h.svc.GetRates(r.Context(), id)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	WriteJSON(w, http.StatusOK, rates)
}

// SetRate sets a per-tier rate override for a catalog item.
func (h *CatalogHandler) SetRate(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r)
	if !ok {
		WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	tierID, err := strconv.ParseInt(chi.URLParam(r, "tierId"), 10, 64)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "invalid tier id")
		return
	}
	var body struct {
		Rate float64 `json:"rate"`
	}
	if err := DecodeJSON(r, &body); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid request")
		return
	}
	if err := h.svc.SetRate(r.Context(), id, tierID, body.Rate); err != nil {
		WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"rateTierId": tierID, "rate": body.Rate})
}
