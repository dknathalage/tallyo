package catalogue

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/dknathalage/tallyo/internal/httpx"
	"github.com/dknathalage/tallyo/internal/listquery"
	"github.com/go-chi/chi/v5"
)

// Handler serves the per-tenant catalogue CRUD, bulk-delete, and the owner/admin
// upload-and-map import routes.
type Handler struct {
	svc *Service
}

// NewHandler constructs the handler. A nil svc is a programmer error.
func NewHandler(svc *Service) *Handler {
	if svc == nil {
		panic("catalogue.NewHandler: nil svc")
	}
	return &Handler{svc: svc}
}

// Routes registers the catalogue routes on r. Reads + CRUD are open to any
// authenticated tenant user; the import (write) is gated to owner/admin.
func (h *Handler) Routes(r chi.Router) {
	r.Get("/catalogue", h.List)
	r.Post("/catalogue", h.Create)
	r.Post("/catalogue/bulk-delete", h.BulkDelete)
	r.With(httpx.RequireRole("owner", "admin")).Post("/catalogue/import/inspect", h.Inspect)
	r.With(httpx.RequireRole("owner", "admin")).Post("/catalogue/import/commit", h.Commit)
	r.Get("/catalogue/{uuid}", h.Get)
	r.Put("/catalogue/{uuid}", h.Update)
	r.Delete("/catalogue/{uuid}", h.Delete)
}

// List returns catalogue items. DataTable query params switch to a paged
// {rows,total}; otherwise ?search= switches to an all-fields search.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if listquery.IsListQuery(q) {
		c := listquery.Build(q, CatalogueCols)
		res, err := h.svc.Query(r.Context(), c)
		if err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "internal error")
			return
		}
		httpx.WriteJSON(w, http.StatusOK, res)
		return
	}
	search := q.Get("search")
	var (
		items []*CatalogueItem
		err   error
	)
	if search != "" {
		items, err = h.svc.Search(r.Context(), search)
	} else {
		items, err = h.svc.List(r.Context())
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, items)
}

// Get returns a single catalogue item by uuid, or 404 when not found.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.ParseUUID(r, "uuid")
	if !ok {
		httpx.WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	it, err := h.svc.Get(r.Context(), id)
	if httpx.WriteServiceError(w, err) {
		return
	}
	httpx.WriteJSON(w, http.StatusOK, it)
}

// Create inserts a catalogue item. An empty name is rejected with 422.
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var in CatalogueItemInput
	if err := httpx.DecodeJSON(r, &in); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request")
		return
	}
	it, err := h.svc.Create(r.Context(), in)
	if httpx.WriteServiceError(w, err) {
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, it)
}

// Update mutates a catalogue item. Empty name -> 422; unknown uuid -> 404.
func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.ParseUUID(r, "uuid")
	if !ok {
		httpx.WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var in CatalogueItemInput
	if err := httpx.DecodeJSON(r, &in); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request")
		return
	}
	it, err := h.svc.Update(r.Context(), id, in)
	if httpx.WriteServiceError(w, err) {
		return
	}
	httpx.WriteJSON(w, http.StatusOK, it)
}

// Delete tombstones a catalogue item by uuid. Unknown uuid -> 404.
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.ParseUUID(r, "uuid")
	if !ok {
		httpx.WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	if err := h.svc.Delete(r.Context(), id); httpx.WriteServiceError(w, err) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// BulkDelete tombstones every catalogue item whose uuid is in the request body.
// The uuids are resolved to logical_ids first; an unknown uuid -> 400.
func (h *Handler) BulkDelete(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Ids []string `json:"ids"`
	}
	if err := httpx.DecodeJSON(r, &body); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request")
		return
	}
	logicalIDs, err := h.svc.ResolveLogicalIDs(r.Context(), body.Ids)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.svc.BulkDelete(r.Context(), logicalIDs); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// maxCatalogueUpload caps the multipart import upload (32 MiB safety ceiling).
const maxCatalogueUpload = 32 << 20

// readUpload parses the multipart form and returns the uploaded file bytes plus
// the file type ("csv"/"xlsx" from the filename), header row, and sheet name.
func readUpload(w http.ResponseWriter, r *http.Request) (data []byte, fileType, sheetName string, headerRow int, ok bool) {
	r.Body = http.MaxBytesReader(w, r.Body, maxCatalogueUpload)
	if err := r.ParseMultipartForm(maxCatalogueUpload); err != nil {
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
	if header != nil && strings.HasSuffix(strings.ToLower(header.Filename), ".xlsx") {
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

// Inspect previews an uploaded file (headers + sample). Owner/admin only.
func (h *Handler) Inspect(w http.ResponseWriter, r *http.Request) {
	data, fileType, sheetName, headerRow, ok := readUpload(w, r)
	if !ok {
		return
	}
	res, err := h.svc.Inspect(data, fileType, sheetName, headerRow)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, res)
}

// Commit applies a column mapping and upserts the catalogue by code. Owner/admin
// only. The whole upload is rejected (400) when parsing/mapping fails.
func (h *Handler) Commit(w http.ResponseWriter, r *http.Request) {
	data, fileType, sheetName, headerRow, ok := readUpload(w, r)
	if !ok {
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
	summary, err := h.svc.ImportMapped(r.Context(), data, fileType, sheetName, headerRow, mapping)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, summary)
}
