package customitem

import (
	"net/http"

	"github.com/dknathalage/tallyo/internal/httpx"
	"github.com/go-chi/chi/v5"
)

// Handler serves the per-tenant custom-item CRUD plus bulk-delete routes.
type Handler struct {
	svc *Service
}

// NewHandler constructs the handler. A nil svc is a programmer error.
func NewHandler(svc *Service) *Handler {
	if svc == nil {
		panic("customitem.NewHandler: nil svc")
	}
	return &Handler{svc: svc}
}

// Routes registers the custom-item routes on r. Mounted inside the
// authenticated /api group by the composition root.
func (h *Handler) Routes(r chi.Router) {
	r.Get("/custom-items", h.List)
	r.Post("/custom-items", h.Create)
	r.Post("/custom-items/bulk-delete", h.BulkDelete)
	r.Get("/custom-items/{id}", h.Get)
	r.Put("/custom-items/{id}", h.Update)
	r.Delete("/custom-items/{id}", h.Delete)
}

// List returns custom items; ?search= switches to a name search.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	search := r.URL.Query().Get("search")
	var (
		items []*CustomItem
		err   error
	)
	if search != "" {
		items, err = h.svc.Search(r.Context(), search)
	} else {
		items, err = h.svc.List(r.Context())
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, items)
}

// Get returns a single custom item by id, or 404 when not found.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.ParseID(r)
	if !ok {
		httpx.WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	it, err := h.svc.Get(r.Context(), id)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if it == nil {
		httpx.WriteError(w, http.StatusNotFound, "not found")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, it)
}

// Create inserts a custom item. An empty name is rejected with 400.
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var in CustomItemInput
	if err := httpx.DecodeJSON(r, &in); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request")
		return
	}
	if in.Name == "" {
		httpx.WriteError(w, http.StatusBadRequest, "name required")
		return
	}
	it, err := h.svc.Create(r.Context(), in)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, it)
}

// Update mutates a custom item. Empty name → 400; unknown id → 404.
func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.ParseID(r)
	if !ok {
		httpx.WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var in CustomItemInput
	if err := httpx.DecodeJSON(r, &in); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request")
		return
	}
	if in.Name == "" {
		httpx.WriteError(w, http.StatusBadRequest, "name required")
		return
	}
	it, err := h.svc.Update(r.Context(), id, in)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if it == nil {
		httpx.WriteError(w, http.StatusNotFound, "not found")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, it)
}

// Delete removes a custom item by id.
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.ParseID(r)
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

// BulkDelete removes every custom item whose id is in the request body.
func (h *Handler) BulkDelete(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Ids []int64 `json:"ids"`
	}
	if err := httpx.DecodeJSON(r, &body); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request")
		return
	}
	if err := h.svc.BulkDelete(r.Context(), body.Ids); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
