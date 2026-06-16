package httpapi

import (
	"net/http"

	"github.com/dknathalage/tallyo/internal/repository"
	"github.com/dknathalage/tallyo/internal/service"
)

// PlanManagerHandler serves the plan-manager CRUD plus bulk-delete routes.
type PlanManagerHandler struct {
	svc *service.PlanManagerService
}

// NewPlanManagerHandler constructs the handler. A nil svc is a programmer error.
func NewPlanManagerHandler(svc *service.PlanManagerService) *PlanManagerHandler {
	if svc == nil {
		panic("NewPlanManagerHandler: nil svc")
	}
	return &PlanManagerHandler{svc: svc}
}

// List returns plan managers, optionally filtered by the ?search= query param.
func (h *PlanManagerHandler) List(w http.ResponseWriter, r *http.Request) {
	search := r.URL.Query().Get("search")
	managers, err := h.svc.List(r.Context(), search)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	WriteJSON(w, http.StatusOK, managers)
}

// Get returns a single plan manager by id, or 404 when not found.
func (h *PlanManagerHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r)
	if !ok {
		WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	p, err := h.svc.Get(r.Context(), id)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if p == nil {
		WriteError(w, http.StatusNotFound, "not found")
		return
	}
	WriteJSON(w, http.StatusOK, p)
}

// Create inserts a plan manager. An empty name is rejected with 400.
func (h *PlanManagerHandler) Create(w http.ResponseWriter, r *http.Request) {
	var in repository.PlanManagerInput
	if err := DecodeJSON(r, &in); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid request")
		return
	}
	if in.Name == "" {
		WriteError(w, http.StatusBadRequest, "name required")
		return
	}
	p, err := h.svc.Create(r.Context(), in)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	WriteJSON(w, http.StatusCreated, p)
}

// Update mutates a plan manager. Empty name → 400; unknown id → 404.
func (h *PlanManagerHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r)
	if !ok {
		WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var in repository.PlanManagerInput
	if err := DecodeJSON(r, &in); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid request")
		return
	}
	if in.Name == "" {
		WriteError(w, http.StatusBadRequest, "name required")
		return
	}
	p, err := h.svc.Update(r.Context(), id, in)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if p == nil {
		WriteError(w, http.StatusNotFound, "not found")
		return
	}
	WriteJSON(w, http.StatusOK, p)
}

// Delete removes a plan manager by id.
func (h *PlanManagerHandler) Delete(w http.ResponseWriter, r *http.Request) {
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

// BulkDelete removes every plan manager whose id is in the request body.
func (h *PlanManagerHandler) BulkDelete(w http.ResponseWriter, r *http.Request) {
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
