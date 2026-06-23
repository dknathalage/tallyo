package client

import (
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/dknathalage/tallyo/internal/httpx"
	"github.com/dknathalage/tallyo/internal/listquery"
	"github.com/go-chi/chi/v5"
)

// Handler serves the client CRUD plus bulk-delete routes.
type Handler struct {
	svc *Service
}

// NewHandler constructs the handler. A nil svc is a programmer error.
func NewHandler(svc *Service) *Handler {
	if svc == nil {
		panic("client.NewHandler: nil svc")
	}
	return &Handler{svc: svc}
}

// Routes registers the client routes on r. Mounted inside the
// authenticated /api group by the composition root.
func (h *Handler) Routes(r chi.Router) {
	r.Get("/clients", h.List)
	r.Post("/clients", h.Create)
	r.Post("/clients/bulk-delete", h.BulkDelete)
	r.Get("/clients/{clientUUID}", h.Get)
	r.Put("/clients/{clientUUID}", h.Update)
	r.Delete("/clients/{clientUUID}", h.Delete)
}

// List returns clients. With DataTable query params (sort/page/limit/f.*)
// it returns a paged {rows,total}; otherwise it keeps the legacy ?search= array
// for callers not yet migrated.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if isListQuery(q) {
		c := listquery.Build(q, ClientCols)
		res, err := h.svc.Query(r.Context(), c)
		if err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "internal error")
			return
		}
		httpx.WriteJSON(w, http.StatusOK, res)
		return
	}
	clients, err := h.svc.List(r.Context(), q.Get("search"))
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, clients)
}

// isListQuery is true when the request carries DataTable query params.
func isListQuery(q url.Values) bool {
	if q.Has("sort") || q.Has("page") || q.Has("limit") {
		return true
	}
	for k := range q {
		if strings.HasPrefix(k, "f.") {
			return true
		}
	}
	return false
}

// Get returns a single client by uuid, or 404 when not found.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.ParseUUID(r, "clientUUID")
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

// Create inserts a client. An empty name is rejected with 400.
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var in ClientInput
	if err := httpx.DecodeJSON(r, &in); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request")
		return
	}
	if in.Name == "" {
		httpx.WriteError(w, http.StatusBadRequest, "name required")
		return
	}
	p, err := h.svc.Create(r.Context(), in)
	if writeClientInputError(w, err) {
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, p)
}

// Update mutates a client. Empty name → 400; unknown id → 404; unknown
// payer uuid → 400.
func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.ParseUUID(r, "clientUUID")
	if !ok {
		httpx.WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var in ClientInput
	if err := httpx.DecodeJSON(r, &in); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request")
		return
	}
	if in.Name == "" {
		httpx.WriteError(w, http.StatusBadRequest, "name required")
		return
	}
	p, err := h.svc.Update(r.Context(), id, in)
	if writeClientInputError(w, err) {
		return
	}
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

// writeClientInputError maps a client-input validation failure to a 400 and
// returns true; on any other error (or nil) it writes nothing and returns false
// so the caller falls through to its generic handling. It covers the payer and
// type sentinels plus the field-level *ValidationError (type-driven NDIS field
// gating), mirroring the invoice/estimate validation-error handling.
func writeClientInputError(w http.ResponseWriter, err error) bool {
	if errors.Is(err, errPayerNotFound) {
		httpx.WriteError(w, http.StatusBadRequest, "unknown payer")
		return true
	}
	if errors.Is(err, errInvalidType) {
		httpx.WriteError(w, http.StatusBadRequest, "invalid client type")
		return true
	}
	var ve *ValidationError
	if errors.As(err, &ve) && ve != nil {
		httpx.WriteJSON(w, http.StatusBadRequest, map[string]any{
			"error":   "validation failed",
			"details": ve.Errors,
		})
		return true
	}
	return false
}

// Delete removes a client by uuid.
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.ParseUUID(r, "clientUUID")
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

// BulkDelete removes every client whose uuid is in the request body. The
// uuids are resolved to int PKs first; an unknown uuid → 400.
func (h *Handler) BulkDelete(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Ids []string `json:"ids"`
	}
	if err := httpx.DecodeJSON(r, &body); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request")
		return
	}
	ids, err := h.svc.ResolveClientIDs(r.Context(), body.Ids)
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
