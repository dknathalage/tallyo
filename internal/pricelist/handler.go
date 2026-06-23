package pricelist

import (
	"errors"
	"io"
	"net/http"

	"github.com/dknathalage/tallyo/internal/httpx"
	"github.com/go-chi/chi/v5"
)

// Handler serves access to the tenant-owned price list (versions, items, zone
// prices). It is tenant-scoped: each tenant owns and populates its own price
// list. Reads are open to any authenticated tenant user; the XLSX ingest (write)
// is gated to owner/admin via Ingest.
type Handler struct {
	svc    *Service
	ingest *IngestService
}

// NewHandler constructs the handler. A nil svc is a programmer error. ingest may
// be nil when the upload route is not mounted.
func NewHandler(svc *Service, ingest *IngestService) *Handler {
	if svc == nil {
		panic("pricelist.NewHandler: nil svc")
	}
	return &Handler{svc: svc, ingest: ingest}
}

// Routes registers the price-list routes on r. Mounted inside the authenticated
// /api group by the composition root.
func (h *Handler) Routes(r chi.Router) {
	r.Get("/price-list/versions", h.ListVersions)
	r.Get("/price-list/versions/{versionUUID}/items", h.ListItems)
	r.Get("/price-list/items/{itemUUID}/prices", h.ListPrices)
	// DEFERRED: price-list XLSX ingest. Route reserved + wired (ParseXLSX is
	// retained), gated to owner/admin like other tenant resources. No new upload
	// UI this pass.
	r.With(httpx.RequireRole("owner", "admin")).Post("/price-list/versions", h.Ingest)
}

// maxPriceListUpload caps the multipart price-list upload (a few MB; 32 MiB is a
// generous safety ceiling).
const maxPriceListUpload = 32 << 20

// Ingest accepts a multipart XLSX upload (field "file") plus "label" and
// "effectiveFrom" form fields and loads a new price-list version. Owner/admin
// only (gated by RequireRole at the route). Returns the created version
// summary as JSON. The whole upload is rejected (400) when parsing fails or a
// required column is missing — no partial version is created (the tx rolls back).
func (h *Handler) Ingest(w http.ResponseWriter, r *http.Request) {
	if h.ingest == nil {
		httpx.WriteError(w, http.StatusNotFound, "ingest not available")
		return
	}
	// Cap the request body before parsing to bound memory.
	r.Body = http.MaxBytesReader(w, r.Body, maxPriceListUpload)
	if err := r.ParseMultipartForm(maxPriceListUpload); err != nil {
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

// ListVersions returns all price-list versions.
func (h *Handler) ListVersions(w http.ResponseWriter, r *http.Request) {
	versions, err := h.svc.ListVersions(r.Context())
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, versions)
}

// ListItems returns the items in a price-list version. The {versionUUID} path
// param is the version uuid. A non-uuid path 400s; an unknown uuid 404s.
func (h *Handler) ListItems(w http.ResponseWriter, r *http.Request) {
	versionUUID, ok := httpx.ParseUUID(r, "versionUUID")
	if !ok {
		httpx.WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	items, err := h.svc.ListItemsByVersionUUID(r.Context(), versionUUID)
	if errors.Is(err, ErrNotFound) {
		httpx.WriteError(w, http.StatusNotFound, "price-list version not found")
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, items)
}

// ListPrices returns the zone prices for an item. The {itemUUID} path param is
// the item uuid. A non-uuid path 400s; an unknown uuid 404s.
func (h *Handler) ListPrices(w http.ResponseWriter, r *http.Request) {
	itemUUID, ok := httpx.ParseUUID(r, "itemUUID")
	if !ok {
		httpx.WriteError(w, http.StatusBadRequest, "invalid item id")
		return
	}
	prices, err := h.svc.ListPricesByItemUUID(r.Context(), itemUUID)
	if errors.Is(err, ErrNotFound) {
		httpx.WriteError(w, http.StatusNotFound, "item not found")
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, prices)
}
