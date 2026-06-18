package httpapi

import (
	"net/http"

	"github.com/dknathalage/tallyo/internal/customitem"
	"github.com/dknathalage/tallyo/internal/export"
	"github.com/dknathalage/tallyo/internal/service"
)

// ExportHandler serves CSV and Excel exports of the tenant's custom items,
// invoices, and estimates. All routes are auth-gated by the server's RequireAuth
// group.
type ExportHandler struct {
	customItems *customitem.Service
	invoices    *service.InvoiceService
	estimates   *service.EstimateService
}

// NewExportHandler constructs the handler. A nil service is a programmer error.
func NewExportHandler(customItems *customitem.Service, invoices *service.InvoiceService, estimates *service.EstimateService) *ExportHandler {
	if customItems == nil || invoices == nil || estimates == nil {
		panic("NewExportHandler: nil service")
	}
	return &ExportHandler{customItems: customItems, invoices: invoices, estimates: estimates}
}

// writeDownload sets download headers and writes the body. A write error after
// headers are committed cannot be reported to the client, so it is ignored.
func writeDownload(w http.ResponseWriter, contentType, filename string, body []byte) {
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

// Catalog exports the tenant's custom items as CSV (default) or XLSX when
// ?format=xlsx.
func (h *ExportHandler) Catalog(w http.ResponseWriter, r *http.Request) {
	items, err := h.customItems.List(r.Context())
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if r.URL.Query().Get("format") == "xlsx" {
		b, xerr := export.CatalogXLSX(items)
		if xerr != nil {
			WriteError(w, http.StatusInternalServerError, "internal error")
			return
		}
		writeDownload(w, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", "catalog.xlsx", b)
		return
	}
	b, cerr := export.CatalogCSV(items)
	if cerr != nil {
		WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeDownload(w, "text/csv", "catalog.csv", b)
}

// Invoices exports invoices as CSV.
func (h *ExportHandler) Invoices(w http.ResponseWriter, r *http.Request) {
	invoices, err := h.invoices.List(r.Context())
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	b, cerr := export.InvoicesCSV(invoices)
	if cerr != nil {
		WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeDownload(w, "text/csv", "invoices.csv", b)
}

// Estimates exports estimates as CSV.
func (h *ExportHandler) Estimates(w http.ResponseWriter, r *http.Request) {
	estimates, err := h.estimates.List(r.Context())
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	b, cerr := export.EstimatesCSV(estimates)
	if cerr != nil {
		WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeDownload(w, "text/csv", "estimates.csv", b)
}
