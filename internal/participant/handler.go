package participant

import (
	"net/http"

	"github.com/dknathalage/tallyo/internal/httpx"
	"github.com/go-chi/chi/v5"
)

// Handler serves the participant CRUD plus bulk-delete routes.
type Handler struct {
	svc *Service
}

// NewHandler constructs the handler. A nil svc is a programmer error.
func NewHandler(svc *Service) *Handler {
	if svc == nil {
		panic("participant.NewHandler: nil svc")
	}
	return &Handler{svc: svc}
}

// Routes registers the participant routes on r. Mounted inside the
// authenticated /api group by the composition root.
func (h *Handler) Routes(r chi.Router) {
	r.Get("/participants", h.List)
	r.Post("/participants", h.Create)
	r.Post("/participants/bulk-delete", h.BulkDelete)
	r.Get("/participants/{id}", h.Get)
	r.Put("/participants/{id}", h.Update)
	r.Delete("/participants/{id}", h.Delete)
}

// List returns participants, optionally filtered by the ?search= query param.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	search := r.URL.Query().Get("search")
	participants, err := h.svc.List(r.Context(), search)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, participants)
}

// Get returns a single participant by id, or 404 when not found.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.ParseID(r)
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

// Create inserts a participant. An empty name is rejected with 400.
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var in ParticipantInput
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

// Update mutates a participant. Empty name → 400; unknown id → 404.
func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.ParseID(r)
	if !ok {
		httpx.WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var in ParticipantInput
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

// Delete removes a participant by id.
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

// BulkDelete removes every participant whose id is in the request body.
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
