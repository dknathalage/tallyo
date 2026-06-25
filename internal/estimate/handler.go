package estimate

import (
	"errors"
	"net/http"
	"net/url"

	"github.com/dknathalage/tallyo/internal/billing"
	"github.com/dknathalage/tallyo/internal/httpx"
	"github.com/dknathalage/tallyo/internal/listquery"
	"github.com/dknathalage/tallyo/internal/pdf"
	"github.com/go-chi/chi/v5"
)

// Handler serves the estimate CRUD, status, duplicate, bulk and
// convert-to-invoice routes.
type Handler struct {
	svc *Service
}

// NewHandler constructs the handler. A nil svc is a programmer error.
func NewHandler(svc *Service) *Handler {
	if svc == nil {
		panic("estimate.NewHandler: nil svc")
	}
	return &Handler{svc: svc}
}

// Routes registers the estimate routes on r. Mounted inside the authenticated
// /api group by the composition root.
func (h *Handler) Routes(r chi.Router) {
	r.Get("/estimates", h.List)
	r.Post("/estimates", h.Create)
	r.Post("/estimates/bulk-delete", h.BulkDelete)
	r.Post("/estimates/bulk-status", h.BulkStatus)
	r.Get("/estimates/{uuid}", h.Get)
	r.Put("/estimates/{uuid}", h.Update)
	r.Delete("/estimates/{uuid}", h.Delete)
	r.Post("/estimates/{uuid}/status", h.Status)
	r.Post("/estimates/{uuid}/duplicate", h.Duplicate)
	r.Get("/estimates/{uuid}/pdf", h.Pdf)
	r.Post("/estimates/{uuid}/convert", h.Convert)
}

// estimateRequest is the flat write payload. ClientUUID/PayerUUID
// arrive as uuids under the public field names (clientId/payerId) and
// are validated against the tenant before the service is called; the remaining fields
// mirror EstimateInput. LineItems carry the priced lines (catalogue refs already
// uuid TEXT — passed through unchanged).
type estimateRequest struct {
	ClientUUID       string                  `json:"clientId"`
	PayerUUID        *string                 `json:"payerId"`
	Status           string                  `json:"status"`
	IssueDate        string                  `json:"issueDate"`
	ValidUntil       string                  `json:"validUntil"`
	Tax              float64                 `json:"tax"`
	Notes            string                  `json:"notes"`
	BusinessSnapshot string                  `json:"businessSnapshot"`
	ClientSnapshot   string                  `json:"clientSnapshot"`
	PayerSnapshot    string                  `json:"payerSnapshot"`
	LineItems        []billing.LineItemInput `json:"lineItems"`
}

// resolveInput resolves an estimateRequest's client/payer uuids
// to row ids (uuid) and returns the EstimateInput. Writes a 400 and returns
// ok=false when the client uuid is missing/unknown for the tenant. A
// missing/empty payer uuid stays NULL; a present-but-unknown one 400s.
func (h *Handler) resolveInput(w http.ResponseWriter, r *http.Request, req estimateRequest) (EstimateInput, bool) {
	if req.ClientUUID == "" || len(req.LineItems) == 0 {
		httpx.WriteError(w, http.StatusBadRequest, "client and at least one line item are required")
		return EstimateInput{}, false
	}
	pid, err := h.svc.ResolveClient(r.Context(), req.ClientUUID)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
		return EstimateInput{}, false
	}
	if pid == "" {
		httpx.WriteError(w, http.StatusBadRequest, "unknown client")
		return EstimateInput{}, false
	}
	var pmID *string
	if req.PayerUUID != nil && *req.PayerUUID != "" {
		id, err := h.svc.ResolvePayer(r.Context(), *req.PayerUUID)
		if err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "internal error")
			return EstimateInput{}, false
		}
		if id == "" {
			httpx.WriteError(w, http.StatusBadRequest, "unknown payer")
			return EstimateInput{}, false
		}
		pmID = &id
	}
	return EstimateInput{
		ClientID:         pid,
		PayerID:          pmID,
		Status:           req.Status,
		IssueDate:        req.IssueDate,
		ValidUntil:       req.ValidUntil,
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

// clientFilter returns the client uuid filter from the query, accepting
// the canonical ?client= and the legacy ?clientId= key.
func clientFilter(q url.Values) string {
	if v := q.Get("client"); v != "" {
		return v
	}
	return q.Get("clientId")
}

// List returns estimates filtered by the optional ?client= (client
// uuid) or ?status= query params. Unlike invoices there is no read-time overdue
// sweep.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if listquery.IsListQuery(q) {
		c := listquery.Build(q, EstimateCols)
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
			httpx.WriteJSON(w, http.StatusOK, []*Estimate{})
			return
		}
		ests, err := h.svc.ListClientEstimates(r.Context(), clientID)
		if err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "internal error")
			return
		}
		httpx.WriteJSON(w, http.StatusOK, ests)
		return
	}
	if status := q.Get("status"); status != "" {
		ests, err := h.svc.ListByStatus(r.Context(), status)
		if err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "internal error")
			return
		}
		httpx.WriteJSON(w, http.StatusOK, ests)
		return
	}
	ests, err := h.svc.List(r.Context())
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, ests)
}

// Get returns a single estimate (with line items), or 404 when not found.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	uid, ok := httpx.ParseUUID(r, "uuid")
	if !ok {
		httpx.WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	est, err := h.svc.GetByUUID(r.Context(), uid)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if est == nil {
		httpx.WriteError(w, http.StatusNotFound, "not found")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, est)
}

// Create inserts an estimate. A missing/unknown client or empty line items → 400.
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var req estimateRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request")
		return
	}
	in, ok := h.resolveInput(w, r, req)
	if !ok {
		return
	}
	est, err := h.svc.Create(r.Context(), in, req.LineItems)
	if writeUnknownCustomItem(w, err) {
		return
	}
	if httpx.WriteServiceError(w, err) {
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, est)
}

// Update rewrites an estimate. Missing client/items → 400; unknown uuid → 404.
func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	uid, ok := httpx.ParseUUID(r, "uuid")
	if !ok {
		httpx.WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var req estimateRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request")
		return
	}
	in, ok := h.resolveInput(w, r, req)
	if !ok {
		return
	}
	est, err := h.svc.UpdateByUUID(r.Context(), uid, in, req.LineItems)
	if writeUnknownCustomItem(w, err) {
		return
	}
	if httpx.WriteServiceError(w, err) {
		return
	}
	if est == nil {
		httpx.WriteError(w, http.StatusNotFound, "not found")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, est)
}

// Delete removes an estimate by uuid.
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

// Status flips just the estimate status. An empty status → 400.
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

// Duplicate copies an estimate into a new draft and returns it. An unknown uuid → 404.
func (h *Handler) Duplicate(w http.ResponseWriter, r *http.Request) {
	uid, ok := httpx.ParseUUID(r, "uuid")
	if !ok {
		httpx.WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	est, err := h.svc.DuplicateByUUID(r.Context(), uid)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if est == nil {
		httpx.WriteError(w, http.StatusNotFound, "not found")
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, est)
}

// BulkDelete removes every estimate whose uuid is in the request body. The uuids
// are validated against the tenant first; an unknown uuid → 400.
func (h *Handler) BulkDelete(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Ids []string `json:"ids"`
	}
	if err := httpx.DecodeJSON(r, &body); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request")
		return
	}
	ids, err := h.svc.ResolveEstimateIDs(r.Context(), body.Ids)
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

// BulkStatus sets the status of every estimate whose uuid is in the request
// body. The uuids are validated against the tenant first; an unknown uuid → 400.
func (h *Handler) BulkStatus(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Ids    []string `json:"ids"`
		Status string   `json:"status"`
	}
	if err := httpx.DecodeJSON(r, &body); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request")
		return
	}
	ids, err := h.svc.ResolveEstimateIDs(r.Context(), body.Ids)
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

// Convert turns an accepted estimate into an invoice. A non-accepted or
// already-converted estimate → 409; an unknown id → 404.
func (h *Handler) Convert(w http.ResponseWriter, r *http.Request) {
	uid, ok := httpx.ParseUUID(r, "uuid")
	if !ok {
		httpx.WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	res, err := h.svc.ConvertByUUID(r.Context(), uid)
	if err != nil {
		if errors.Is(err, ErrNotAccepted) {
			httpx.WriteError(w, http.StatusConflict, "only accepted estimates can be converted")
			return
		}
		if errors.Is(err, ErrAlreadyConverted) {
			httpx.WriteError(w, http.StatusConflict, "estimate already converted")
			return
		}
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if res == nil {
		httpx.WriteError(w, http.StatusNotFound, "not found")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, res)
}

// Pdf renders an estimate to PDF and returns it as a file download.
func (h *Handler) Pdf(w http.ResponseWriter, r *http.Request) {
	uid, ok := httpx.ParseUUID(r, "uuid")
	if !ok {
		httpx.WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	est, err := h.svc.GetByUUID(r.Context(), uid)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if est == nil {
		httpx.WriteError(w, http.StatusNotFound, "not found")
		return
	}
	estDoc := &pdf.EstimateDoc{
		Number:           est.Number,
		IssueDate:        est.IssueDate,
		ValidUntil:       est.ValidUntil,
		Status:           est.Status,
		BusinessSnapshot: est.BusinessSnapshot,
		ClientSnapshot:   est.ClientSnapshot,
		LineItems:        est.LineItems,
		Subtotal:         est.Subtotal,
		Tax:              est.Tax,
		Total:            est.Total,
		Notes:            est.Notes,
	}
	b, err := pdf.RenderEstimate(estDoc)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "pdf render failed")
		return
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", `attachment; filename="`+est.Number+`.pdf"`)
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(b); err != nil {
		// client gone; nothing to do
	}
}
