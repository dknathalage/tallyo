package httpapi

import (
	"net/http"

	"github.com/dknathalage/tallyo/internal/repository"
	"github.com/dknathalage/tallyo/internal/service"
)

// BusinessProfileHandler serves the singleton business-profile GET/PUT routes.
type BusinessProfileHandler struct {
	svc *service.BusinessProfileService
}

// NewBusinessProfileHandler constructs the handler. A nil svc is a programmer error.
func NewBusinessProfileHandler(svc *service.BusinessProfileService) *BusinessProfileHandler {
	if svc == nil {
		panic("NewBusinessProfileHandler: nil svc")
	}
	return &BusinessProfileHandler{svc: svc}
}

// Get returns the current profile, or JSON null when none has been saved yet.
func (h *BusinessProfileHandler) Get(w http.ResponseWriter, r *http.Request) {
	p, err := h.svc.Get(r.Context())
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	WriteJSON(w, http.StatusOK, p) // p may be nil → JSON null
}

// Put upserts the profile. An empty name is rejected with 400 before the save;
// any other save failure is an internal error (500).
func (h *BusinessProfileHandler) Put(w http.ResponseWriter, r *http.Request) {
	var in repository.BusinessProfileInput
	if err := DecodeJSON(r, &in); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid request")
		return
	}
	if in.Name == "" {
		WriteError(w, http.StatusBadRequest, "name required")
		return
	}
	if err := h.svc.Save(r.Context(), in); err != nil {
		WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
