package payer

import (
	"net/http"

	"github.com/dknathalage/tallyo/internal/httpx"
	"github.com/dknathalage/tallyo/internal/listquery"
	"github.com/go-chi/chi/v5"
)

// Handler serves the payer CRUD plus bulk-delete routes.
type Handler struct {
	svc *Service
}

// NewHandler constructs the handler. A nil svc is a programmer error.
func NewHandler(svc *Service) *Handler {
	if svc == nil {
		panic("payer.NewHandler: nil svc")
	}
	return &Handler{svc: svc}
}

// Routes registers the payer routes on r. Mounted inside the
// authenticated /api group by the composition root.
func (h *Handler) Routes(r chi.Router) {
	r.Get("/payers", h.List)
	r.Post("/payers", h.Create)
	r.Post("/payers/bulk-delete", h.BulkDelete)
	r.Get("/payers/{uuid}", h.Get)
	r.Put("/payers/{uuid}", h.Update)
	r.Delete("/payers/{uuid}", h.Delete)
}

// List returns payers. With DataTable query params (sort/page/limit/f.*)
// it returns a paged {rows,total}; otherwise it keeps the legacy ?search= array
// for callers not yet migrated.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if listquery.IsListQuery(q) {
		c := listquery.Build(q, PayerCols)
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

// Get returns a single payer by uuid, or 404 when not found.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.ParseUUID(r, "uuid")
	if !ok {
		httpx.WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	p, err := h.svc.Get(r.Context(), id)
	if httpx.WriteServiceError(w, err) {
		return
	}
	httpx.WriteJSON(w, http.StatusOK, p)
}

// Create inserts a payer. An empty name is rejected with 422.
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var in PayerInput
	if err := httpx.DecodeJSON(r, &in); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request")
		return
	}
	p, err := h.svc.Create(r.Context(), in)
	if httpx.WriteServiceError(w, err) {
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, p)
}

// Update mutates a payer. Empty name → 422; unknown uuid → 404.
func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.ParseUUID(r, "uuid")
	if !ok {
		httpx.WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var in PayerInput
	if err := httpx.DecodeJSON(r, &in); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request")
		return
	}
	p, err := h.svc.Update(r.Context(), id, in)
	if httpx.WriteServiceError(w, err) {
		return
	}
	httpx.WriteJSON(w, http.StatusOK, p)
}

// Delete removes a payer by uuid. Unknown uuid → 404.
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

// BulkDelete removes every payer whose uuid is in the request body. The
// uuids are validated against the tenant first; an unknown uuid → 400.
func (h *Handler) BulkDelete(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Ids []string `json:"ids"`
	}
	if err := httpx.DecodeJSON(r, &body); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request")
		return
	}
	ids, err := h.svc.ResolvePayerIDs(r.Context(), body.Ids)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.svc.BulkDelete(r.Context(), ids); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
