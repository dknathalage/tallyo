package invoice

import (
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
	r.Post("/invoices/draft-from-shifts", h.DraftFromShifts)
	r.Post("/invoices/bulk-delete", h.BulkDelete)
	r.Post("/invoices/bulk-status", h.BulkStatus)
	r.Get("/invoices/{uuid}", h.Get)
	r.Put("/invoices/{uuid}", h.Update)
	r.Delete("/invoices/{uuid}", h.Delete)
	r.Post("/invoices/{uuid}/status", h.Status)
	r.Get("/invoices/{uuid}/pdf", h.Pdf)
	r.Get("/participants/{participantUUID}/stats", h.ParticipantStats)
}

// invoiceRequest is the flat write payload. ParticipantUUID/PlanManagerUUID
// arrive as uuids under the public field names (participantId/planManagerId) and
// are resolved to int FKs before the service is called; the remaining fields
// mirror InvoiceInput. LineItems carry the priced lines (catalogue refs already
// uuid TEXT — passed through unchanged).
type invoiceRequest struct {
	ParticipantUUID  string                  `json:"participantId"`
	PlanManagerUUID  *string                 `json:"planManagerId"`
	Status           string                  `json:"status"`
	IssueDate        string                  `json:"issueDate"`
	DueDate          string                  `json:"dueDate"`
	Tax              float64                 `json:"tax"`
	Notes            string                  `json:"notes"`
	BusinessSnapshot string                  `json:"businessSnapshot"`
	ClientSnapshot   string                  `json:"participantSnapshot"`
	PayerSnapshot    string                  `json:"planManagerSnapshot"`
	LineItems        []billing.LineItemInput `json:"lineItems"`
}

// resolveInput translates an invoiceRequest's participant/plan-manager uuids into
// int FKs and returns the int-keyed InvoiceInput. Writes a 400 and returns
// ok=false when the participant uuid is missing/unknown for the tenant. A
// missing/empty plan-manager uuid stays NULL; a present-but-unknown one 400s.
func (h *Handler) resolveInput(w http.ResponseWriter, r *http.Request, req invoiceRequest) (InvoiceInput, bool) {
	if req.ParticipantUUID == "" || len(req.LineItems) == 0 {
		httpx.WriteError(w, http.StatusBadRequest, "participant and at least one line item are required")
		return InvoiceInput{}, false
	}
	pid, err := h.svc.ResolveParticipant(r.Context(), req.ParticipantUUID)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
		return InvoiceInput{}, false
	}
	if pid == 0 {
		httpx.WriteError(w, http.StatusBadRequest, "unknown participant")
		return InvoiceInput{}, false
	}
	var pmID *int64
	if req.PlanManagerUUID != nil && *req.PlanManagerUUID != "" {
		id, err := h.svc.ResolvePlanManager(r.Context(), *req.PlanManagerUUID)
		if err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "internal error")
			return InvoiceInput{}, false
		}
		if id == 0 {
			httpx.WriteError(w, http.StatusBadRequest, "unknown plan manager")
			return InvoiceInput{}, false
		}
		pmID = &id
	}
	return InvoiceInput{
		ParticipantID:    pid,
		PlanManagerID:    pmID,
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

// writeValidationError, when err is (or wraps) a *billing.ValidationError,
// writes a 422 envelope and returns true. Otherwise it writes nothing and
// returns false so callers can fall through to generic error handling.
func writeValidationError(w http.ResponseWriter, err error) bool {
	ve, ok := billing.AsValidationError(err)
	if !ok || ve == nil {
		return false
	}
	httpx.WriteJSON(w, http.StatusUnprocessableEntity, map[string]any{
		"error":   "validation failed",
		"details": ve.Errors,
	})
	return true
}

// List performs a read-time overdue sweep, then returns invoices filtered by the
// optional ?participantId= or ?status= query params.
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
	if pUUID := participantFilter(q); pUUID != "" {
		participantID, err := h.svc.ResolveParticipant(r.Context(), pUUID)
		if err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "internal error")
			return
		}
		if participantID == 0 {
			httpx.WriteJSON(w, http.StatusOK, []*Invoice{})
			return
		}
		invs, err := h.svc.ListParticipantInvoices(r.Context(), participantID)
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

// participantFilter returns the participant uuid filter from the query, accepting
// the canonical ?participant= and the legacy ?participantId= key.
func participantFilter(q url.Values) string {
	if v := q.Get("participant"); v != "" {
		return v
	}
	return q.Get("participantId")
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

// Create inserts an invoice. A missing/unknown participant or empty line items → 400.
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
	if err != nil {
		if writeValidationError(w, err) {
			return
		}
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, inv)
}

// DraftFromShifts drafts one invoice from the posted recorded shift ids. An
// empty list or a validation failure (mixed participants, an empty shift, a
// non-recorded shift) → 400.
func (h *Handler) DraftFromShifts(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ShiftIds []string `json:"shiftIds"`
	}
	if err := httpx.DecodeJSON(r, &body); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request")
		return
	}
	if len(body.ShiftIds) == 0 {
		httpx.WriteError(w, http.StatusBadRequest, "at least one shift is required")
		return
	}
	shiftIDs, err := h.svc.ResolveShiftIDs(r.Context(), body.ShiftIds)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	inv, err := h.svc.DraftFromShifts(r.Context(), shiftIDs)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, inv)
}

// Update rewrites an invoice. Missing participant/items → 400; unknown uuid → 404.
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
	if err != nil {
		if writeValidationError(w, err) {
			return
		}
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
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
// are resolved to int PKs first; an unknown uuid → 400.
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
// The uuids are resolved to int PKs first; an unknown uuid → 400.
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

// ParticipantStats returns the count and summed total of one participant's
// invoices. The {participantUUID} path param is the participant uuid.
func (h *Handler) ParticipantStats(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.ParseUUID(r, "participantUUID")
	if !ok {
		httpx.WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	stats, err := h.svc.ParticipantStats(r.Context(), id)
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
