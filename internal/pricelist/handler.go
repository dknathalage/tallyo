package pricelist

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/dknathalage/tallyo/internal/httpx"
	"github.com/go-chi/chi/v5"
)

// Handler serves access to the tenant-owned price list (versions, items, zone
// prices). It is tenant-scoped: each tenant owns and populates its own price
// list. Reads are open to any authenticated tenant user; the upload-and-map
// import (write) is gated to owner/admin.
type Handler struct {
	svc     *Service
	import_ *ImportService
}

// NewHandler constructs the handler. A nil svc is a programmer error. imp may be
// nil when the import routes are not mounted.
func NewHandler(svc *Service, imp *ImportService) *Handler {
	if svc == nil {
		panic("pricelist.NewHandler: nil svc")
	}
	return &Handler{svc: svc, import_: imp}
}

// Routes registers the price-list routes on r. Mounted inside the authenticated
// /api group by the composition root.
func (h *Handler) Routes(r chi.Router) {
	r.Get("/price-list/versions", h.ListVersions)
	r.Get("/price-list/versions/{versionUUID}/items", h.ListItems)
	r.Get("/price-list/items/{itemUUID}/prices", h.ListPrices)
	// Generic two-step upload-and-map import, owner/admin only.
	r.With(httpx.RequireRole("owner", "admin")).Post("/price-list/import/inspect", h.Inspect)
	r.With(httpx.RequireRole("owner", "admin")).Post("/price-list/import/commit", h.Commit)
}

// maxPriceListUpload caps the multipart price-list upload (a few MB; 32 MiB is a
// generous safety ceiling).
const maxPriceListUpload = 32 << 20

// readUpload parses the multipart form and returns the uploaded file bytes plus
// the file type ("csv"/"xlsx" inferred from the filename), header row, and sheet
// name. It writes the error response itself and returns ok=false on failure.
func readUpload(w http.ResponseWriter, r *http.Request) (data []byte, fileType, sheetName string, headerRow int, ok bool) {
	r.Body = http.MaxBytesReader(w, r.Body, maxPriceListUpload)
	if err := r.ParseMultipartForm(maxPriceListUpload); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid or oversized upload")
		return nil, "", "", 0, false
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "file is required")
		return nil, "", "", 0, false
	}
	defer func() { _ = file.Close() }()
	data, err = io.ReadAll(file)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "could not read upload")
		return nil, "", "", 0, false
	}
	if len(data) == 0 {
		httpx.WriteError(w, http.StatusBadRequest, "uploaded file is empty")
		return nil, "", "", 0, false
	}
	fileType = "csv"
	if header != nil && len(header.Filename) >= 5 && header.Filename[len(header.Filename)-5:] == ".xlsx" {
		fileType = "xlsx"
	}
	sheetName = r.FormValue("sheetName")
	headerRow = 1
	if hr := r.FormValue("headerRow"); hr != "" {
		if n, err := strconv.Atoi(hr); err == nil && n >= 1 {
			headerRow = n
		}
	}
	return data, fileType, sheetName, headerRow, true
}

// Inspect accepts a multipart upload (field "file", optional "headerRow"/
// "sheetName") and returns the parsed headers + a capped sample. Persists
// nothing. Owner/admin only (gated at the route).
func (h *Handler) Inspect(w http.ResponseWriter, r *http.Request) {
	if h.import_ == nil {
		httpx.WriteError(w, http.StatusNotFound, "import not available")
		return
	}
	data, fileType, sheetName, headerRow, ok := readUpload(w, r)
	if !ok {
		return
	}
	res, err := h.import_.Inspect(data, fileType, sheetName, headerRow)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, res)
}

// Commit accepts a multipart upload (field "file") plus a "mapping" JSON object
// (sourceHeader→targetField) and a "label", and loads a new price-list version.
// The whole upload is rejected (400) when parsing/mapping fails — no partial
// version is created (the tx rolls back). Owner/admin only (gated at the route).
func (h *Handler) Commit(w http.ResponseWriter, r *http.Request) {
	if h.import_ == nil {
		httpx.WriteError(w, http.StatusNotFound, "import not available")
		return
	}
	data, fileType, sheetName, headerRow, ok := readUpload(w, r)
	if !ok {
		return
	}
	label := r.FormValue("label")
	if label == "" {
		httpx.WriteError(w, http.StatusBadRequest, "label is required")
		return
	}
	mappingRaw := r.FormValue("mapping")
	if mappingRaw == "" {
		httpx.WriteError(w, http.StatusBadRequest, "mapping is required")
		return
	}
	var mapping map[string]string
	if err := json.Unmarshal([]byte(mappingRaw), &mapping); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "mapping must be a JSON object")
		return
	}
	summary, err := h.import_.ImportMapped(r.Context(), data, fileType, sheetName, headerRow, mapping, label)
	if err != nil {
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
