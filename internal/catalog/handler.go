package catalog

import (
	"errors"
	"io"
	"net/http"

	"github.com/dknathalage/tallyo/internal/httpx"
	"github.com/go-chi/chi/v5"
)

// Handler serves access to the per-tenant NDIS Support Catalogue
// (versions, support items, zone prices). It is tenant-scoped: each tenant owns
// and populates its own catalogue. Reads are open to any authenticated tenant
// user; the XLSX ingest (write) is gated to owner/admin via Ingest.
type Handler struct {
	svc    *Service
	ingest *IngestService
}

// NewHandler constructs the handler. A nil svc is a programmer error. ingest may
// be nil when the upload route is not mounted.
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
	r.Get("/support-catalog/versions/{versionUUID}/items", h.ListItems)
	r.Get("/support-catalog/items/{itemUUID}/prices", h.ListPrices)
	// DEFERRED: catalogue XLSX ingest. Route reserved + wired (ParseXLSX is
	// retained), now gated to owner/admin like other tenant resources rather
	// than platform-admin. No new upload UI this pass.
	r.With(httpx.RequireRole("owner", "admin")).Post("/support-catalog/versions", h.Ingest)
}

// maxCatalogUpload caps the multipart catalogue upload (the official NDIS
// Support Catalogue XLSX is a few MB; 32 MiB is a generous safety ceiling).
const maxCatalogUpload = 32 << 20

// Ingest accepts a multipart XLSX upload (field "file") plus "label" and
// "effectiveFrom" form fields and loads a new catalogue version. Owner/admin
// only (gated by RequireRole at the route). Returns the created version
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

// ListItems returns the support items in a catalogue version. The
// {versionUUID} path param is the version uuid. A non-uuid path 400s; an unknown
// uuid 404s.
func (h *Handler) ListItems(w http.ResponseWriter, r *http.Request) {
	versionUUID, ok := httpx.ParseUUID(r, "versionUUID")
	if !ok {
		httpx.WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	items, err := h.svc.ListSupportItemsByVersionUUID(r.Context(), versionUUID)
	if errors.Is(err, ErrNotFound) {
		httpx.WriteError(w, http.StatusNotFound, "catalogue version not found")
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, items)
}

// ListPrices returns the zone prices for a support item. The {itemUUID} path
// param is the support item uuid. A non-uuid path 400s; an unknown uuid 404s.
func (h *Handler) ListPrices(w http.ResponseWriter, r *http.Request) {
	itemUUID, ok := httpx.ParseUUID(r, "itemUUID")
	if !ok {
		httpx.WriteError(w, http.StatusBadRequest, "invalid item id")
		return
	}
	prices, err := h.svc.ListPricesByItemUUID(r.Context(), itemUUID)
	if errors.Is(err, ErrNotFound) {
		httpx.WriteError(w, http.StatusNotFound, "support item not found")
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, prices)
}
