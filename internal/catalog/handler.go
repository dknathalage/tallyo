package catalog

import (
	"io"
	"net/http"
	"strconv"

	"github.com/dknathalage/tallyo/internal/httpx"
	"github.com/go-chi/chi/v5"
)

// Handler serves read access to the GLOBAL NDIS Support Catalogue
// (versions, support items, zone prices). It is NOT tenant-scoped. The
// platform-admin XLSX ingest is served via Ingest (gated by RequirePlatformAdmin).
type Handler struct {
	svc    *Service
	ingest *IngestService
}

// NewHandler constructs the handler. A nil svc is a programmer error. ingest may
// be nil when the platform-admin upload route is not mounted.
func NewHandler(svc *Service, ingest *IngestService) *Handler {
	if svc == nil {
		panic("catalog.NewHandler: nil svc")
	}
	return &Handler{svc: svc, ingest: ingest}
}

// Routes registers the support-catalogue routes on r. Mounted inside the
// authenticated /api group by the composition root.
func (h *Handler) Routes(r chi.Router) {
	r.Get("/support-catalog/versions", h.ListVersions)
	r.Get("/support-catalog/versions/{id}/items", h.ListItems)
	r.Get("/support-catalog/items/{itemId}/prices", h.ListPrices)
	r.With(httpx.RequirePlatformAdmin).Post("/support-catalog/versions", h.Ingest)
}

// maxCatalogUpload caps the multipart catalogue upload (the official NDIS
// Support Catalogue XLSX is a few MB; 32 MiB is a generous safety ceiling).
const maxCatalogUpload = 32 << 20

// Ingest accepts a multipart XLSX upload (field "file") plus "label" and
// "effectiveFrom" form fields and loads a new catalogue version. Platform-admin
// only (gated by RequirePlatformAdmin at the route). Returns the created version
// summary as JSON. The whole upload is rejected (400) when parsing fails or a
// required column is missing — no partial version is created (the tx rolls back).
func (h *Handler) Ingest(w http.ResponseWriter, r *http.Request) {
	if h.ingest == nil {
		httpx.WriteError(w, http.StatusNotFound, "ingest not available")
		return
	}
	// Cap the request body before parsing to bound memory.
	r.Body = http.MaxBytesReader(w, r.Body, maxCatalogUpload)
	if err := r.ParseMultipartForm(maxCatalogUpload); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid or oversized upload")
		return
	}

	label := r.FormValue("label")
	effectiveFrom := r.FormValue("effectiveFrom")
	if label == "" || effectiveFrom == "" {
		httpx.WriteError(w, http.StatusBadRequest, "label and effectiveFrom are required")
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "file is required")
		return
	}
	defer func() { _ = file.Close() }()

	data, err := io.ReadAll(file)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "could not read upload")
		return
	}
	if len(data) == 0 {
		httpx.WriteError(w, http.StatusBadRequest, "uploaded file is empty")
		return
	}

	filename := ""
	if header != nil {
		filename = header.Filename
	}
	summary, err := h.ingest.IngestXLSX(r.Context(), data, label, effectiveFrom, filename)
	if err != nil {
		// Parse/validation failures are the client's problem (bad file shape).
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, summary)
}

// ListVersions returns all catalogue versions.
func (h *Handler) ListVersions(w http.ResponseWriter, r *http.Request) {
	versions, err := h.svc.ListVersions(r.Context())
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, versions)
}

// ListItems returns the support items in a catalogue version. The {id} path
// param is the version id.
func (h *Handler) ListItems(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.ParseID(r)
	if !ok {
		httpx.WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	items, err := h.svc.ListSupportItems(r.Context(), id)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, items)
}

// ListPrices returns the zone prices for a support item. The {itemId} path param
// is the support item id.
func (h *Handler) ListPrices(w http.ResponseWriter, r *http.Request) {
	itemID, err := strconv.ParseInt(chi.URLParam(r, "itemId"), 10, 64)
	if err != nil || itemID <= 0 {
		httpx.WriteError(w, http.StatusBadRequest, "invalid item id")
		return
	}
	prices, perr := h.svc.ListPrices(r.Context(), itemID)
	if perr != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, prices)
}
