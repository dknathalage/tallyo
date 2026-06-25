package businessprofile

import (
	"net/http"

	"github.com/dknathalage/tallyo/internal/httpx"
	"github.com/go-chi/chi/v5"
)

// Handler serves the business-profile GET/PUT routes.
type Handler struct {
	svc *Service
}

// NewHandler constructs the handler. A nil svc is a programmer error.
func NewHandler(svc *Service) *Handler {
	if svc == nil {
		panic("businessprofile.NewHandler: nil svc")
	}
	return &Handler{svc: svc}
}

// Routes registers the business-profile routes on r. Mounted inside the
// authenticated /api group by the composition root. All roles may read;
// owner/admin may write (RequireRole gate applied here in the slice).
func (h *Handler) Routes(r chi.Router) {
	r.Get("/business-profile", h.Get)
	r.With(httpx.RequireRole("owner", "admin")).Put("/business-profile", h.Put)
}

// Get returns the current profile, or JSON null when none has been saved yet.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	p, err := h.svc.Get(r.Context())
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, p) // p may be nil → JSON null
}

// Put upserts the profile. An empty name is rejected with 422 (the input's
// Validate); any other save failure is an internal error (500).
func (h *Handler) Put(w http.ResponseWriter, r *http.Request) {
	var in BusinessProfileInput
	if err := httpx.DecodeJSON(r, &in); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request")
		return
	}
	if err := h.svc.Save(r.Context(), in); httpx.WriteServiceError(w, err) {
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
