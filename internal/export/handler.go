package export

import (
	"net/http"

	"github.com/dknathalage/tallyo/internal/customitem"
	"github.com/dknathalage/tallyo/internal/estimate"
	"github.com/dknathalage/tallyo/internal/httpx"
	"github.com/dknathalage/tallyo/internal/invoice"
)

// Handler serves CSV and Excel exports of the tenant's custom items,
// invoices, and estimates. All routes are auth-gated by the server's RequireAuth
// group.
type Handler struct {
	customItems *customitem.Service
	invoices    *invoice.Service
	estimates   *estimate.Service
}

// NewHandler constructs the handler. A nil service is a programmer error.
func NewHandler(customItems *customitem.Service, invoices *invoice.Service, estimates *estimate.Service) *Handler {
	if customItems == nil || invoices == nil || estimates == nil {
		panic("export.NewHandler: nil service")
	}
	return &Handler{customItems: customItems, invoices: invoices, estimates: estimates}
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
func (h *Handler) Catalog(w http.ResponseWriter, r *http.Request) {
	items, err := h.customItems.List(r.Context())
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if r.URL.Query().Get("format") == "xlsx" {
		b, xerr := CatalogXLSX(items)
		if xerr != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "internal error")
			return
		}
		writeDownload(w, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", "catalog.xlsx", b)
		return
	}
	b, cerr := CatalogCSV(items)
	if cerr != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeDownload(w, "text/csv", "catalog.csv", b)
}

// Invoices exports invoices as CSV.
func (h *Handler) Invoices(w http.ResponseWriter, r *http.Request) {
	invoices, err := h.invoices.List(r.Context())
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	b, cerr := InvoicesCSV(invoices)
	if cerr != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeDownload(w, "text/csv", "invoices.csv", b)
}

// Estimates exports estimates as CSV.
func (h *Handler) Estimates(w http.ResponseWriter, r *http.Request) {
	estimates, err := h.estimates.List(r.Context())
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	b, cerr := EstimatesCSV(estimates)
	if cerr != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeDownload(w, "text/csv", "estimates.csv", b)
}
