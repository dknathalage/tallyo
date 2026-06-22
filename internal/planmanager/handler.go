package planmanager

import (
	"net/http"

	"github.com/dknathalage/tallyo/internal/httpx"
	"github.com/dknathalage/tallyo/internal/listquery"
	"github.com/go-chi/chi/v5"
)

// Handler serves the plan-manager CRUD plus bulk-delete routes.
type Handler struct {
	svc *Service
}

// NewHandler constructs the handler. A nil svc is a programmer error.
func NewHandler(svc *Service) *Handler {
	if svc == nil {
		panic("planmanager.NewHandler: nil svc")
	}
	return &Handler{svc: svc}
}

// Routes registers the plan-manager routes on r. Mounted inside the
// authenticated /api group by the composition root.
func (h *Handler) Routes(r chi.Router) {
	r.Get("/plan-managers", h.List)
	r.Post("/plan-managers", h.Create)
	r.Post("/plan-managers/bulk-delete", h.BulkDelete)
	r.Get("/plan-managers/{uuid}", h.Get)
	r.Put("/plan-managers/{uuid}", h.Update)
	r.Delete("/plan-managers/{uuid}", h.Delete)
}

// List returns plan managers. With DataTable query params (sort/page/limit/f.*)
// it returns a paged {rows,total}; otherwise it keeps the legacy ?search= array
// for callers not yet migrated.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if listquery.IsListQuery(q) {
		c := listquery.Build(q, PlanManagerCols)
		res, err := h.svc.Query(r.Context(), c)
		if err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "internal error")
			return
		}
		httpx.WriteJSON(w, http.StatusOK, res)
		return
	}
	managers, err := h.svc.List(r.Context(), q.Get("search"))
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, managers)
}

// Get returns a single plan manager by uuid, or 404 when not found.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.ParseUUID(r, "uuid")
	if !ok {
		httpx.WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	p, err := h.svc.Get(r.Context(), id)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if p == nil {
		httpx.WriteError(w, http.StatusNotFound, "not found")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, p)
}

// Create inserts a plan manager. An empty name is rejected with 400.
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var in PlanManagerInput
	if err := httpx.DecodeJSON(r, &in); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request")
		return
	}
	if in.Name == "" {
		httpx.WriteError(w, http.StatusBadRequest, "name required")
		return
	}
	p, err := h.svc.Create(r.Context(), in)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, p)
}

// Update mutates a plan manager. Empty name → 400; unknown uuid → 404.
func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.ParseUUID(r, "uuid")
	if !ok {
		httpx.WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var in PlanManagerInput
	if err := httpx.DecodeJSON(r, &in); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request")
		return
	}
	if in.Name == "" {
		httpx.WriteError(w, http.StatusBadRequest, "name required")
		return
	}
	p, err := h.svc.Update(r.Context(), id, in)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if p == nil {
		httpx.WriteError(w, http.StatusNotFound, "not found")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, p)
}

// Delete removes a plan manager by uuid.
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.ParseUUID(r, "uuid")
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

// BulkDelete removes every plan manager whose id is in the request body.
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
