package httpapi

import (
	"io"
	"net/http"
	"strconv"

	"github.com/dknathalage/tallyo/internal/repository"
	"github.com/dknathalage/tallyo/internal/service"
	"github.com/go-chi/chi/v5"
)

// CustomItemHandler serves the per-tenant custom-item CRUD plus bulk-delete
// routes (the tenant-scoped successor to the old catalog items).
type CustomItemHandler struct {
	svc *service.CustomItemService
}

// NewCustomItemHandler constructs the handler. A nil svc is a programmer error.
func NewCustomItemHandler(svc *service.CustomItemService) *CustomItemHandler {
	if svc == nil {
		panic("NewCustomItemHandler: nil svc")
	}
	return &CustomItemHandler{svc: svc}
}

// List returns custom items; ?search= switches to a name search.
func (h *CustomItemHandler) List(w http.ResponseWriter, r *http.Request) {
	search := r.URL.Query().Get("search")
	var (
		items []*repository.CustomItem
		err   error
	)
	if search != "" {
		items, err = h.svc.Search(r.Context(), search)
	} else {
		items, err = h.svc.List(r.Context())
	}
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	WriteJSON(w, http.StatusOK, items)
}

// Get returns a single custom item by id, or 404 when not found.
func (h *CustomItemHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r)
	if !ok {
		WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	it, err := h.svc.Get(r.Context(), id)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if it == nil {
		WriteError(w, http.StatusNotFound, "not found")
		return
	}
	WriteJSON(w, http.StatusOK, it)
}

// Create inserts a custom item. An empty name is rejected with 400.
func (h *CustomItemHandler) Create(w http.ResponseWriter, r *http.Request) {
	var in repository.CustomItemInput
	if err := DecodeJSON(r, &in); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid request")
		return
	}
	if in.Name == "" {
		WriteError(w, http.StatusBadRequest, "name required")
		return
	}
	it, err := h.svc.Create(r.Context(), in)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	WriteJSON(w, http.StatusCreated, it)
}

// Update mutates a custom item. Empty name → 400; unknown id → 404.
func (h *CustomItemHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r)
	if !ok {
		WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var in repository.CustomItemInput
	if err := DecodeJSON(r, &in); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid request")
		return
	}
	if in.Name == "" {
		WriteError(w, http.StatusBadRequest, "name required")
		return
	}
	it, err := h.svc.Update(r.Context(), id, in)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if it == nil {
		WriteError(w, http.StatusNotFound, "not found")
		return
	}
	WriteJSON(w, http.StatusOK, it)
}

// Delete removes a custom item by id.
func (h *CustomItemHandler) Delete(w http.ResponseWriter, r *http.Request) {
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

// BulkDelete removes every custom item whose id is in the request body.
func (h *CustomItemHandler) BulkDelete(w http.ResponseWriter, r *http.Request) {
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

// SupportCatalogHandler serves read access to the GLOBAL NDIS Support Catalogue
// (versions, support items, zone prices). It is NOT tenant-scoped. The
// platform-admin XLSX ingest is served via Ingest (gated by RequirePlatformAdmin).
type SupportCatalogHandler struct {
	svc    *service.SupportCatalogService
	ingest *service.CatalogIngestService
}

// NewSupportCatalogHandler constructs the handler. A nil svc is a programmer
// error. ingest may be nil when the platform-admin upload route is not mounted.
func NewSupportCatalogHandler(svc *service.SupportCatalogService, ingest *service.CatalogIngestService) *SupportCatalogHandler {
	if svc == nil {
		panic("NewSupportCatalogHandler: nil svc")
	}
	return &SupportCatalogHandler{svc: svc, ingest: ingest}
}

// maxCatalogUpload caps the multipart catalogue upload (the official NDIS
// Support Catalogue XLSX is a few MB; 32 MiB is a generous safety ceiling).
const maxCatalogUpload = 32 << 20

// Ingest accepts a multipart XLSX upload (field "file") plus "label" and
// "effectiveFrom" form fields and loads a new catalogue version. Platform-admin
// only (gated by RequirePlatformAdmin at the route). Returns the created version
// summary as JSON. The whole upload is rejected (400) when parsing fails or a
// required column is missing — no partial version is created (the tx rolls back).
func (h *SupportCatalogHandler) Ingest(w http.ResponseWriter, r *http.Request) {
	if h.ingest == nil {
		WriteError(w, http.StatusNotFound, "ingest not available")
		return
	}
	// Cap the request body before parsing to bound memory.
	r.Body = http.MaxBytesReader(w, r.Body, maxCatalogUpload)
	if err := r.ParseMultipartForm(maxCatalogUpload); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid or oversized upload")
		return
	}

	label := r.FormValue("label")
	effectiveFrom := r.FormValue("effectiveFrom")
	if label == "" || effectiveFrom == "" {
		WriteError(w, http.StatusBadRequest, "label and effectiveFrom are required")
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		WriteError(w, http.StatusBadRequest, "file is required")
		return
	}
	defer func() { _ = file.Close() }()

	data, err := io.ReadAll(file)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "could not read upload")
		return
	}
	if len(data) == 0 {
		WriteError(w, http.StatusBadRequest, "uploaded file is empty")
		return
	}

	filename := ""
	if header != nil {
		filename = header.Filename
	}
	summary, err := h.ingest.IngestXLSX(r.Context(), data, label, effectiveFrom, filename)
	if err != nil {
		// Parse/validation failures are the client's problem (bad file shape).
		WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	WriteJSON(w, http.StatusCreated, summary)
}

// ListVersions returns all catalogue versions.
func (h *SupportCatalogHandler) ListVersions(w http.ResponseWriter, r *http.Request) {
	versions, err := h.svc.ListVersions(r.Context())
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	WriteJSON(w, http.StatusOK, versions)
}

// ListItems returns the support items in a catalogue version. The {id} path
// param is the version id.
func (h *SupportCatalogHandler) ListItems(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r)
	if !ok {
		WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	items, err := h.svc.ListSupportItems(r.Context(), id)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	WriteJSON(w, http.StatusOK, items)
}

// ListPrices returns the zone prices for a support item. The {itemId} path param
// is the support item id.
func (h *SupportCatalogHandler) ListPrices(w http.ResponseWriter, r *http.Request) {
	itemID, err := strconv.ParseInt(chi.URLParam(r, "itemId"), 10, 64)
	if err != nil || itemID <= 0 {
		WriteError(w, http.StatusBadRequest, "invalid item id")
		return
	}
	prices, perr := h.svc.ListPrices(r.Context(), itemID)
	if perr != nil {
		WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	WriteJSON(w, http.StatusOK, prices)
}
