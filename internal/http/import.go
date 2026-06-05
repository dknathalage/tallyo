package httpapi

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/dknathalage/tallyo/internal/importer"
	"github.com/dknathalage/tallyo/internal/repository"
)

// maxImportUpload caps the multipart upload size for catalog imports.
const maxImportUpload = 10 << 20 // 10 MiB

// ImportHandler serves the catalog import preview and commit routes. It holds
// the catalog repo (for diff/commit), the column-mappings repo (to resolve a
// mapping id), and the rate-tiers repo (to resolve tier names for commit).
// All routes are auth-gated by the server's RequireAuth group.
type ImportHandler struct {
	catalog  *repository.CatalogRepo
	mappings *repository.ColumnMappingsRepo
	tiers    *repository.RateTiersRepo
}

// NewImportHandler constructs the handler. A nil dependency is a programmer
// error.
func NewImportHandler(catalog *repository.CatalogRepo, mappings *repository.ColumnMappingsRepo, tiers *repository.RateTiersRepo) *ImportHandler {
	if catalog == nil || mappings == nil || tiers == nil {
		panic("NewImportHandler: nil dependency")
	}
	return &ImportHandler{catalog: catalog, mappings: mappings, tiers: tiers}
}

// parsedImport bundles the diff produced from a multipart request body so the
// preview and commit handlers can share the parse → map → diff pipeline.
type parsedImport struct {
	diff importer.DiffResult
}

// buildMapping converts a persisted ColumnMapping to a transient importer.Mapping.
// TierMapping stores header→tierID strings; we load existing tiers once and
// resolve each ID to a name, falling back to the raw ID string if lookup fails.
func (h *ImportHandler) buildMapping(r *http.Request, m *repository.ColumnMapping) (importer.Mapping, error) {
	var fields map[string]string
	if s := strings.TrimSpace(m.Mapping); s != "" && s != "{}" {
		if err := json.Unmarshal([]byte(s), &fields); err != nil {
			return importer.Mapping{}, fmt.Errorf("parse mapping: %w", err)
		}
	}
	if fields == nil {
		fields = map[string]string{}
	}

	tierCols := map[string]string{}
	var rawTierMap map[string]string
	if s := strings.TrimSpace(m.TierMapping); s != "" && s != "{}" {
		if err := json.Unmarshal([]byte(s), &rawTierMap); err != nil {
			return importer.Mapping{}, fmt.Errorf("parse tier mapping: %w", err)
		}
	}
	if len(rawTierMap) > 0 {
		existingTiers, err := h.tiers.List(r.Context())
		if err != nil {
			return importer.Mapping{}, fmt.Errorf("list tiers: %w", err)
		}
		byID := make(map[int64]string, len(existingTiers))
		for _, t := range existingTiers { // bounded by len(existingTiers)
			byID[t.ID] = t.Name
		}
		for header, idStr := range rawTierMap { // bounded by len(rawTierMap)
			id, err := strconv.ParseInt(strings.TrimSpace(idStr), 10, 64)
			if err != nil {
				tierCols[header] = idStr // fall back to raw string
				continue
			}
			if name, ok := byID[id]; ok {
				tierCols[header] = name
			} else {
				tierCols[header] = idStr // fall back to raw string
			}
		}
	}

	return importer.Mapping{
		Fields:    fields,
		TierCols:  tierCols,
		FileType:  m.FileType,
		SheetName: m.SheetName,
		HeaderRow: int(m.HeaderRow),
	}, nil
}

// parseImportRequest reads the multipart file + mappingId, resolves the column
// mapping, parses and maps the rows, and diffs against the catalog. On failure
// it writes the appropriate error response and returns ok=false.
func (h *ImportHandler) parseImportRequest(w http.ResponseWriter, r *http.Request) (parsedImport, bool) {
	if err := r.ParseMultipartForm(maxImportUpload); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid multipart form")
		return parsedImport{}, false
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		WriteError(w, http.StatusBadRequest, "file required")
		return parsedImport{}, false
	}
	defer func() { _ = file.Close() }()

	data, err := io.ReadAll(io.LimitReader(file, maxImportUpload))
	if err != nil {
		WriteError(w, http.StatusBadRequest, "could not read file")
		return parsedImport{}, false
	}
	if len(data) == 0 {
		WriteError(w, http.StatusBadRequest, "file required")
		return parsedImport{}, false
	}

	mappingID, err := strconv.ParseInt(r.FormValue("mappingId"), 10, 64)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "invalid mappingId")
		return parsedImport{}, false
	}
	m, err := h.mappings.Get(r.Context(), mappingID)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal error")
		return parsedImport{}, false
	}
	if m == nil {
		WriteError(w, http.StatusBadRequest, "mapping not found")
		return parsedImport{}, false
	}

	fileType := m.FileType
	if override := r.FormValue("fileType"); override != "" {
		fileType = override
	}

	mapping, err := h.buildMapping(r, m)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal error")
		return parsedImport{}, false
	}
	mapping.FileType = fileType

	_, rows, err := importer.ParseRows(data, fileType, m.SheetName, int(m.HeaderRow))
	if err != nil {
		WriteError(w, http.StatusBadRequest, fmt.Sprintf("parse file: %v", err))
		return parsedImport{}, false
	}
	mapped, rowErrs, err := importer.ApplyMapping(rows, mapping)
	if err != nil {
		WriteError(w, http.StatusBadRequest, fmt.Sprintf("apply mapping: %v", err))
		return parsedImport{}, false
	}
	diff, err := importer.Diff(r.Context(), h.catalog, mapped, len(rowErrs))
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal error")
		return parsedImport{}, false
	}
	return parsedImport{diff: diff}, true
}

// Preview parses an uploaded catalog file, applies the named column mapping, and
// returns the diff against the existing catalog without writing anything.
func (h *ImportHandler) Preview(w http.ResponseWriter, r *http.Request) {
	parsed, ok := h.parseImportRequest(w, r)
	if !ok {
		return
	}
	WriteJSON(w, http.StatusOK, parsed.diff)
}

// Commit re-parses the uploaded file (stateless), diffs it, and writes the new
// items plus, when updateExisting=true, the changed items.
func (h *ImportHandler) Commit(w http.ResponseWriter, r *http.Request) {
	parsed, ok := h.parseImportRequest(w, r)
	if !ok {
		return
	}
	updateExisting := r.FormValue("updateExisting") == "true"
	res, err := importer.Commit(r.Context(), h.catalog, h.tiers, parsed.diff, updateExisting)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	WriteJSON(w, http.StatusOK, res)
}
