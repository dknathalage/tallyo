package httpapi

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/dknathalage/tallyo/internal/importer"
	"github.com/dknathalage/tallyo/internal/repository"
)

const maxImportUpload = 10 << 20 // 10 MiB

// ImportHandler serves the catalog import parse/preview/commit routes. It holds
// the catalog repo (diff/commit) and rate-tiers repo (tier create-if-missing).
// Routes are auth-gated by the server's RequireAuth group.
type ImportHandler struct {
	catalog *repository.CatalogRepo
	tiers   *repository.RateTiersRepo
}

// NewImportHandler constructs the handler. A nil dependency is a programmer error.
func NewImportHandler(catalog *repository.CatalogRepo, tiers *repository.RateTiersRepo) *ImportHandler {
	if catalog == nil || tiers == nil {
		panic("NewImportHandler: nil dependency")
	}
	return &ImportHandler{catalog: catalog, tiers: tiers}
}

// readUpload reads the multipart "file" field, bounded by maxImportUpload.
func (h *ImportHandler) readUpload(w http.ResponseWriter, r *http.Request) ([]byte, bool) {
	if err := r.ParseMultipartForm(maxImportUpload); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid multipart form")
		return nil, false
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		WriteError(w, http.StatusBadRequest, "file required")
		return nil, false
	}
	defer func() { _ = file.Close() }()
	data, err := io.ReadAll(io.LimitReader(file, maxImportUpload))
	if err != nil || len(data) == 0 {
		WriteError(w, http.StatusBadRequest, "file required")
		return nil, false
	}
	return data, true
}

// parseMapping reads the "mapping" multipart field (JSON) into a transient Mapping.
func parseMapping(r *http.Request) (importer.Mapping, error) {
	var m importer.Mapping
	raw := r.FormValue("mapping")
	if raw == "" {
		return m, fmt.Errorf("mapping required")
	}
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return m, fmt.Errorf("invalid mapping: %w", err)
	}
	if m.HeaderRow < 1 {
		m.HeaderRow = 1
	}
	return m, nil
}

// Parse parses an uploaded file, samples rows, runs detection, and returns
// headers + sample + a suggested mapping. Writes nothing.
func (h *ImportHandler) Parse(w http.ResponseWriter, r *http.Request) {
	data, ok := h.readUpload(w, r)
	if !ok {
		return
	}
	fileType := r.FormValue("fileType")
	sheet := r.FormValue("sheetName")
	headerRow := 1
	if v := r.FormValue("headerRow"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 1 {
			headerRow = n
		}
	}
	headers, rows, err := importer.ParseRows(data, fileType, sheet, headerRow)
	if err != nil {
		WriteError(w, http.StatusBadRequest, fmt.Sprintf("parse file: %v", err))
		return
	}
	sample := rows
	if len(sample) > 50 {
		sample = sample[:50]
	}
	WriteJSON(w, http.StatusOK, map[string]any{
		"headers":    headers,
		"sample":     sample,
		"suggestion": importer.DetectMapping(headers, sample),
	})
}

// diffFromRequest runs parse → map → diff using the inline mapping.
func (h *ImportHandler) diffFromRequest(w http.ResponseWriter, r *http.Request) (importer.DiffResult, bool) {
	data, ok := h.readUpload(w, r)
	if !ok {
		return importer.DiffResult{}, false
	}
	m, err := parseMapping(r)
	if err != nil {
		WriteError(w, http.StatusBadRequest, err.Error())
		return importer.DiffResult{}, false
	}
	_, rows, err := importer.ParseRows(data, m.FileType, m.SheetName, m.HeaderRow)
	if err != nil {
		WriteError(w, http.StatusBadRequest, fmt.Sprintf("parse file: %v", err))
		return importer.DiffResult{}, false
	}
	mapped, rowErrs, err := importer.ApplyMapping(rows, m)
	if err != nil {
		WriteError(w, http.StatusBadRequest, fmt.Sprintf("apply mapping: %v", err))
		return importer.DiffResult{}, false
	}
	diff, err := importer.Diff(r.Context(), h.catalog, mapped, len(rowErrs))
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal error")
		return importer.DiffResult{}, false
	}
	return diff, true
}

// Preview returns the diff without writing anything.
func (h *ImportHandler) Preview(w http.ResponseWriter, r *http.Request) {
	diff, ok := h.diffFromRequest(w, r)
	if !ok {
		return
	}
	WriteJSON(w, http.StatusOK, diff)
}

// Commit re-parses, diffs, and writes new items (+ updated when updateExisting),
// creating any referenced tiers.
func (h *ImportHandler) Commit(w http.ResponseWriter, r *http.Request) {
	diff, ok := h.diffFromRequest(w, r)
	if !ok {
		return
	}
	updateExisting := r.FormValue("updateExisting") == "true"
	res, err := importer.Commit(r.Context(), h.catalog, h.tiers, diff, updateExisting)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	WriteJSON(w, http.StatusOK, res)
}
