package invoice

import (
	"errors"
	"log/slog"
	"net/http"
	"net/url"

	"github.com/dknathalage/tallyo/internal/billing"
	"github.com/dknathalage/tallyo/internal/httpx"
	"github.com/dknathalage/tallyo/internal/listquery"
	"github.com/dknathalage/tallyo/internal/pdf"
	"github.com/dknathalage/tallyo/internal/reqctx"
	"github.com/go-chi/chi/v5"
)

// Handler serves the invoice CRUD, status, bulk and per-client stats routes.
type Handler struct {
	svc *Service
}

// NewHandler constructs the handler. A nil svc is a programmer error.
func NewHandler(svc *Service) *Handler {
	if svc == nil {
		panic("invoice.NewHandler: nil svc")
	}
	return &Handler{svc: svc}
}

// Routes registers the invoice routes on r. Mounted inside the authenticated
// /api group by the composition root.
func (h *Handler) Routes(r chi.Router) {
	r.Get("/invoices", h.List)
	r.Post("/invoices", h.Create)
	r.Post("/invoices/draft-from-sessions", h.DraftFromSessions)
	r.Post("/invoices/bulk-delete", h.BulkDelete)
	r.Post("/invoices/bulk-status", h.BulkStatus)
	r.Get("/invoices/{uuid}", h.Get)
	r.Put("/invoices/{uuid}", h.Update)
	r.Delete("/invoices/{uuid}", h.Delete)
	r.Post("/invoices/{uuid}/status", h.Status)
	r.Get("/invoices/{uuid}/pdf", h.Pdf)
	r.Get("/clients/{clientUUID}/stats", h.ClientStats)
}

// invoiceRequest is the flat write payload. ClientUUID/PayerUUID
// arrive as uuids under the public field names (clientId/payerId) and
// are validated against the tenant before the service is called; the remaining fields
// mirror InvoiceInput. LineItems carry the priced lines (catalogue refs already
// uuid TEXT — passed through unchanged).
type invoiceRequest struct {
	ClientUUID       string                  `json:"clientId"`
	PayerUUID        *string                 `json:"payerId"`
	Status           string                  `json:"status"`
	IssueDate        string                  `json:"issueDate"`
	DueDate          string                  `json:"dueDate"`
	Tax              float64                 `json:"tax"`
	Notes            string                  `json:"notes"`
	BusinessSnapshot string                  `json:"businessSnapshot"`
	ClientSnapshot   string                  `json:"clientSnapshot"`
	PayerSnapshot    string                  `json:"payerSnapshot"`
	LineItems        []billing.LineItemInput `json:"lineItems"`
}

// resolveInput resolves an invoiceRequest's client/payer uuids to row ids
// and returns the resolved InvoiceInput. Writes a 400 and returns
// ok=false when the client uuid is missing/unknown for the tenant. A
// missing/empty payer uuid stays NULL; a present-but-unknown one 400s.
func (h *Handler) resolveInput(w http.ResponseWriter, r *http.Request, req invoiceRequest) (InvoiceInput, bool) {
	if req.ClientUUID == "" || len(req.LineItems) == 0 {
		httpx.WriteError(w, http.StatusBadRequest, "client and at least one line item are required")
		return InvoiceInput{}, false
	}
	pid, err := h.svc.ResolveClient(r.Context(), req.ClientUUID)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
		return InvoiceInput{}, false
	}
	if pid == "" {
		httpx.WriteError(w, http.StatusBadRequest, "unknown client")
		return InvoiceInput{}, false
	}
	var pmID *string
	if req.PayerUUID != nil && *req.PayerUUID != "" {
		id, err := h.svc.ResolvePayer(r.Context(), *req.PayerUUID)
		if err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "internal error")
			return InvoiceInput{}, false
		}
		if id == "" {
			httpx.WriteError(w, http.StatusBadRequest, "unknown payer")
			return InvoiceInput{}, false
		}
		pmID = &id
	}
	return InvoiceInput{
		ClientID:         pid,
		PayerID:          pmID,
		Status:           req.Status,
		IssueDate:        req.IssueDate,
		DueDate:          req.DueDate,
		Tax:              req.Tax,
		Notes:            req.Notes,
		BusinessSnapshot: req.BusinessSnapshot,
		ClientSnapshot:   req.ClientSnapshot,
		PayerSnapshot:    req.PayerSnapshot,
	}, true
}

// writeUnknownCustomItem maps the unknown-custom-item billing sentinel to a 400
// and returns true; otherwise it writes nothing and returns false so the caller
// falls through to httpx.WriteServiceError. httpx cannot import billing, so this
// one domain sentinel is mapped here at the slice (mirroring client's
// errPayerNotFound→400 pre-check); the rest (*billing.ValidationError→422,
// ErrNotFound→404, else 500) is handled uniformly by WriteServiceError.
func writeUnknownCustomItem(w http.ResponseWriter, err error) bool {
	if errors.Is(err, billing.ErrUnknownCustomItem) {
		httpx.WriteError(w, http.StatusBadRequest, "unknown custom item")
		return true
	}
	return false
}

// List performs a read-time overdue sweep, then returns invoices filtered by the
// optional ?clientId= or ?status= query params.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	if _, err := h.svc.MarkOverdueForTenant(r.Context(), reqctx.MustTenant(r.Context())); err != nil {
		httpx.LoggerFrom(r.Context()).Error("overdue sweep on list failed", slog.Any("error", err))
	}
	q := r.URL.Query()
	if listquery.IsListQuery(q) {
		c := listquery.Build(q, InvoiceCols)
		res, err := h.svc.Query(r.Context(), c)
		if err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "internal error")
			return
		}
		httpx.WriteJSON(w, http.StatusOK, res)
		return
	}
	if pUUID := clientFilter(q); pUUID != "" {
		clientID, err := h.svc.ResolveClient(r.Context(), pUUID)
		if err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "internal error")
			return
		}
		if clientID == "" {
			httpx.WriteJSON(w, http.StatusOK, []*Invoice{})
			return
		}
		invs, err := h.svc.ListClientInvoices(r.Context(), clientID)
		if err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "internal error")
			return
		}
		httpx.WriteJSON(w, http.StatusOK, invs)
		return
	}
	if status := q.Get("status"); status != "" {
		invs, err := h.svc.ListByStatus(r.Context(), status)
		if err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "internal error")
			return
		}
		httpx.WriteJSON(w, http.StatusOK, invs)
		return
	}
	invs, err := h.svc.List(r.Context())
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, invs)
}

// clientFilter returns the client uuid filter from the query, accepting
// the canonical ?client= and the legacy ?clientId= key.
func clientFilter(q url.Values) string {
	if v := q.Get("client"); v != "" {
		return v
	}
	return q.Get("clientId")
}

// Get returns a single invoice (with line items), or 404 when not found.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	uid, ok := httpx.ParseUUID(r, "uuid")
	if !ok {
		httpx.WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	inv, err := h.svc.GetByUUID(r.Context(), uid)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if inv == nil {
		httpx.WriteError(w, http.StatusNotFound, "not found")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, inv)
}

// Create inserts an invoice. A missing/unknown client or empty line items → 400.
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var req invoiceRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request")
		return
	}
	in, ok := h.resolveInput(w, r, req)
	if !ok {
		return
	}
	inv, err := h.svc.Create(r.Context(), in, req.LineItems)
	if writeUnknownCustomItem(w, err) {
		return
	}
	if httpx.WriteServiceError(w, err) {
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, inv)
}

// DraftFromSessions drafts one invoice from the posted recorded session ids. An
// empty list or a validation failure (mixed clients, an empty session, a
// non-recorded session) → 400.
func (h *Handler) DraftFromSessions(w http.ResponseWriter, r *http.Request) {
	var body struct {
		SessionIds []string `json:"sessionIds"`
	}
	if err := httpx.DecodeJSON(r, &body); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request")
		return
	}
	if len(body.SessionIds) == 0 {
		httpx.WriteError(w, http.StatusBadRequest, "at least one session is required")
		return
	}
	sessionIDs, err := h.svc.ResolveSessionIDs(r.Context(), body.SessionIds)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	inv, err := h.svc.DraftFromSessions(r.Context(), sessionIDs)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, inv)
}

// Update rewrites an invoice. Missing client/items → 400; unknown uuid → 404.
func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	uid, ok := httpx.ParseUUID(r, "uuid")
	if !ok {
		httpx.WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var req invoiceRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request")
		return
	}
	in, ok := h.resolveInput(w, r, req)
	if !ok {
		return
	}
	inv, err := h.svc.UpdateByUUID(r.Context(), uid, in, req.LineItems)
	if writeUnknownCustomItem(w, err) {
		return
	}
	if httpx.WriteServiceError(w, err) {
		return
	}
	if inv == nil {
		httpx.WriteError(w, http.StatusNotFound, "not found")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, inv)
}

// Delete removes an invoice by uuid.
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	uid, ok := httpx.ParseUUID(r, "uuid")
	if !ok {
		httpx.WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	if err := h.svc.DeleteByUUID(r.Context(), uid); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Status flips just the invoice status. An empty status → 400.
func (h *Handler) Status(w http.ResponseWriter, r *http.Request) {
	uid, ok := httpx.ParseUUID(r, "uuid")
	if !ok {
		httpx.WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var body struct {
		Status string `json:"status"`
	}
	if err := httpx.DecodeJSON(r, &body); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request")
		return
	}
	if body.Status == "" {
		httpx.WriteError(w, http.StatusBadRequest, "status required")
		return
	}
	if err := h.svc.UpdateStatusByUUID(r.Context(), uid, body.Status); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": body.Status})
}

// BulkDelete removes every invoice whose uuid is in the request body. The uuids
// are validated against the tenant first; an unknown uuid → 400.
func (h *Handler) BulkDelete(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Ids []string `json:"ids"`
	}
	if err := httpx.DecodeJSON(r, &body); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request")
		return
	}
	ids, err := h.svc.ResolveInvoiceIDs(r.Context(), body.Ids)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.svc.BulkDelete(r.Context(), ids); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// BulkStatus sets the status of every invoice whose uuid is in the request body.
// The uuids are validated against the tenant first; an unknown uuid → 400.
func (h *Handler) BulkStatus(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Ids    []string `json:"ids"`
		Status string   `json:"status"`
	}
	if err := httpx.DecodeJSON(r, &body); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request")
		return
	}
	ids, err := h.svc.ResolveInvoiceIDs(r.Context(), body.Ids)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.svc.BulkUpdateStatus(r.Context(), ids, body.Status); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ClientStats returns the count and summed total of one client's
// invoices. The {clientUUID} path param is the client uuid.
func (h *Handler) ClientStats(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.ParseUUID(r, "clientUUID")
	if !ok {
		httpx.WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	stats, err := h.svc.ClientStats(r.Context(), id)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if stats == nil {
		httpx.WriteError(w, http.StatusNotFound, "not found")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, stats)
}

// Pdf renders an invoice to PDF and returns it as a file download.
func (h *Handler) Pdf(w http.ResponseWriter, r *http.Request) {
	uid, ok := httpx.ParseUUID(r, "uuid")
	if !ok {
		httpx.WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	inv, err := h.svc.GetByUUID(r.Context(), uid)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if inv == nil {
		httpx.WriteError(w, http.StatusNotFound, "not found")
		return
	}
	doc := &pdf.InvoiceDoc{
		Number:           inv.Number,
		IssueDate:        inv.IssueDate,
		DueDate:          inv.DueDate,
		Status:           inv.Status,
		BusinessSnapshot: inv.BusinessSnapshot,
		ClientSnapshot:   inv.ClientSnapshot,
		LineItems:        inv.LineItems,
		Subtotal:         inv.Subtotal,
		Tax:              inv.Tax,
		Total:            inv.Total,
		Notes:            inv.Notes,
	}
	b, err := pdf.RenderInvoice(doc)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "pdf render failed")
		return
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", `attachment; filename="`+inv.Number+`.pdf"`)
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(b); err != nil {
		// client gone; nothing to do
	}
}
