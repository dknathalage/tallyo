package shift

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/dknathalage/tallyo/internal/billing"
	"github.com/dknathalage/tallyo/internal/httpx"
	"github.com/dknathalage/tallyo/internal/reqctx"
	"github.com/go-chi/chi/v5"
)

// ShiftDivider is the narrow interface the shift handler needs to divide ONE
// shift's note into priced line items. It is declared here (not imported from the
// agent slice) and satisfied by *agent.Smarts, wired in internal/app — the same
// consumer-declared pattern as InvoiceChecker. A nil divider (AI disabled) makes
// the /divide route return 503.
type ShiftDivider interface {
	DivideShift(ctx context.Context, shiftID int64) error
}

// Handler serves the shift lifecycle routes: per-client listing,
// tenant-wide listing, the billing-suggestion and to-record prompts, plus shift
// CRUD, the status-transition endpoint, and the AI divide route.
type Handler struct {
	svc     *Service
	divider ShiftDivider // nil when AI is disabled → /divide returns 503
}

// NewHandler constructs the handler. A nil svc is a programmer error. divider may
// be nil (AI disabled), in which case the /divide route returns 503.
func NewHandler(svc *Service, divider ShiftDivider) *Handler {
	if svc == nil {
		panic("shift.NewHandler: nil svc")
	}
	return &Handler{svc: svc, divider: divider}
}

// Routes registers all shift routes on r. Mounted inside the authenticated
// /api group by the composition root (server.go).
func (h *Handler) Routes(r chi.Router) {
	r.Get("/shifts", h.List)
	r.Get("/shifts/suggestions", h.Suggestions)
	r.Get("/shifts/to-record", h.ToRecord)
	r.Post("/shifts", h.Create)
	r.Get("/shifts/{shiftUUID}", h.Get)
	r.Put("/shifts/{shiftUUID}", h.Update)
	r.Delete("/shifts/{shiftUUID}", h.Delete)
	r.Post("/shifts/{shiftUUID}/status", h.UpdateStatus)
	r.Get("/shifts/{shiftUUID}/items", h.ListItems)
	r.Post("/shifts/{shiftUUID}/items", h.AddItem)
	r.Patch("/shifts/{shiftUUID}/items/{itemUUID}", h.UpdateItem)
	r.Delete("/shifts/{shiftUUID}/items/{itemUUID}", h.DeleteItem)
	r.Post("/shifts/{shiftUUID}/divide", h.Divide)
}

// List returns the tenant's shifts. With ?client={clientUUID} it
// returns only that client's shifts (resolving the client uuid to its
// int FK — this replaces the old nested client→shifts read). An optional
// ?status= filter restricts the lifecycle status in either mode.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	if clientUUID := r.URL.Query().Get("client"); clientUUID != "" {
		shifts, err := h.svc.ListByClientUUID(r.Context(), clientUUID, status)
		if err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "internal error")
			return
		}
		httpx.WriteJSON(w, http.StatusOK, shifts)
		return
	}
	shifts, err := h.svc.List(r.Context(), status)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, shifts)
}

// Suggestions returns each client's recorded-but-unbilled billing prompt.
func (h *Handler) Suggestions(w http.ResponseWriter, r *http.Request) {
	out, err := h.svc.Suggestions(r.Context())
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

// ToRecord returns the tenant's scheduled shifts still awaiting a record.
func (h *Handler) ToRecord(w http.ResponseWriter, r *http.Request) {
	out, err := h.svc.ToRecord(r.Context())
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

// shiftBody is the HTTP write shape of a shift. ClientUUID arrives as the
// client's uuid (resolved to the int FK before insert/update); the other
// fields mirror ShiftInput. It is the inbound DTO — ShiftInput stays int-keyed
// for the cross-slice ShiftCreator contract (agent import).
type shiftBody struct {
	ClientUUID  string   `json:"clientId"`
	ServiceDate string   `json:"serviceDate"`
	Note        string   `json:"note"`
	Tags        []string `json:"tags"`
	Status      string   `json:"status"`
}

// Get returns a single shift by uuid, or 404 when not found.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	shiftUUID, ok := httpx.ParseUUID(r, "shiftUUID")
	if !ok {
		httpx.WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	sh, err := h.svc.GetByUUID(r.Context(), shiftUUID)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if sh == nil {
		httpx.WriteError(w, http.StatusNotFound, "not found")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, sh)
}

// Create inserts a shift. A missing/unknown client uuid or service date → 400.
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var body shiftBody
	if err := httpx.DecodeJSON(r, &body); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request")
		return
	}
	if body.ClientUUID == "" {
		httpx.WriteError(w, http.StatusBadRequest, "client required")
		return
	}
	if body.ServiceDate == "" {
		httpx.WriteError(w, http.StatusBadRequest, "service date required")
		return
	}
	in, ok := h.resolveBody(w, r, body)
	if !ok {
		return
	}
	sh, err := h.svc.Create(r.Context(), in)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, sh)
}

// resolveBody translates a shiftBody's client uuid into the int FK and
// returns the int-keyed ShiftInput. Writes a 400 and returns ok=false when the
// client uuid is unknown for the tenant.
func (h *Handler) resolveBody(w http.ResponseWriter, r *http.Request, body shiftBody) (ShiftInput, bool) {
	pid, err := h.svc.ResolveClient(r.Context(), body.ClientUUID)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
		return ShiftInput{}, false
	}
	if pid == 0 {
		httpx.WriteError(w, http.StatusBadRequest, "unknown client")
		return ShiftInput{}, false
	}
	return ShiftInput{
		ClientID:    pid,
		ServiceDate: body.ServiceDate,
		Note:        body.Note,
		Tags:        body.Tags,
		Status:      body.Status,
	}, true
}

// Update mutates a shift. Empty service date → 400; unknown shift uuid → 404;
// unknown client uuid → 400.
func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	shiftUUID, ok := httpx.ParseUUID(r, "shiftUUID")
	if !ok {
		httpx.WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var body shiftBody
	if err := httpx.DecodeJSON(r, &body); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request")
		return
	}
	if body.ServiceDate == "" {
		httpx.WriteError(w, http.StatusBadRequest, "service date required")
		return
	}
	in, ok := h.resolveBody(w, r, body)
	if !ok {
		return
	}
	sh, err := h.svc.Update(r.Context(), shiftUUID, in)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if sh == nil {
		httpx.WriteError(w, http.StatusNotFound, "not found")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, sh)
}

// Delete removes a shift by uuid.
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	shiftUUID, ok := httpx.ParseUUID(r, "shiftUUID")
	if !ok {
		httpx.WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	if err := h.svc.Delete(r.Context(), shiftUUID); err != nil {
		if errors.Is(err, ErrShiftBilled) {
			httpx.WriteError(w, http.StatusConflict, "cannot delete a billed shift")
			return
		}
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ListItems returns a shift's line items (billed + unbilled), [] when none.
// Unknown shift uuid → 404.
func (h *Handler) ListItems(w http.ResponseWriter, r *http.Request) {
	shiftUUID, ok := httpx.ParseUUID(r, "shiftUUID")
	if !ok {
		httpx.WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	items, err := h.svc.ListItemsByShiftUUID(r.Context(), shiftUUID)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if items == nil {
		httpx.WriteError(w, http.StatusNotFound, "not found")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, items)
}

// AddItem adds one line item to a shift. Unknown shift → 404; invalid line → 400.
func (h *Handler) AddItem(w http.ResponseWriter, r *http.Request) {
	shiftUUID, ok := httpx.ParseUUID(r, "shiftUUID")
	if !ok {
		httpx.WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	in, ok := decodeItem(w, r)
	if !ok {
		return
	}
	item, err := h.svc.AddItemByShiftUUID(r.Context(), shiftUUID, in)
	if err != nil {
		if errors.Is(err, billing.ErrUnknownCustomItem) {
			httpx.WriteError(w, http.StatusBadRequest, "unknown custom item")
			return
		}
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if item == nil {
		httpx.WriteError(w, http.StatusNotFound, "not found")
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, item)
}

// UpdateItem rewrites one unbilled item addressed by uuid under its shift uuid.
// Unknown/billed item → 404.
func (h *Handler) UpdateItem(w http.ResponseWriter, r *http.Request) {
	shiftUUID, ok := httpx.ParseUUID(r, "shiftUUID")
	if !ok {
		httpx.WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	itemUUID, ok := httpx.ParseUUID(r, "itemUUID")
	if !ok {
		httpx.WriteError(w, http.StatusBadRequest, "invalid item id")
		return
	}
	in, ok := decodeItem(w, r)
	if !ok {
		return
	}
	item, err := h.svc.UpdateItemByShiftUUID(r.Context(), shiftUUID, itemUUID, in)
	if err != nil {
		if errors.Is(err, billing.ErrUnknownCustomItem) {
			httpx.WriteError(w, http.StatusBadRequest, "unknown custom item")
			return
		}
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if item == nil {
		httpx.WriteError(w, http.StatusNotFound, "not found or already billed")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, item)
}

// DeleteItem removes one unbilled item by uuid under its shift uuid (no-op when
// absent/billed).
func (h *Handler) DeleteItem(w http.ResponseWriter, r *http.Request) {
	shiftUUID, ok := httpx.ParseUUID(r, "shiftUUID")
	if !ok {
		httpx.WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	itemUUID, ok := httpx.ParseUUID(r, "itemUUID")
	if !ok {
		httpx.WriteError(w, http.StatusBadRequest, "invalid item id")
		return
	}
	if err := h.svc.DeleteItemByShiftUUID(r.Context(), shiftUUID, itemUUID); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Divide runs the AI divide Smart over ONE shift — turning its note into priced
// catalogue line items (idempotent: a re-divide replaces the shift's unbilled
// items) — then returns the shift's items. Returns 503 when AI is disabled.
// Synchronous: it blocks for the Smart run on a detached, bounded context so a
// client disconnect does not abort the model call.
func (h *Handler) Divide(w http.ResponseWriter, r *http.Request) {
	if h.divider == nil {
		httpx.WriteError(w, http.StatusServiceUnavailable, "AI not configured")
		return
	}
	shiftUUID, ok := httpx.ParseUUID(r, "shiftUUID")
	if !ok {
		httpx.WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	tenantID, tok := reqctx.TenantFrom(r.Context())
	if !tok || tenantID <= 0 {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	uid, _ := reqctx.UserFrom(r.Context())

	ctx, cancel := context.WithTimeout(reqctx.WithUser(reqctx.WithTenant(context.Background(), tenantID), uid), 5*time.Minute)
	defer cancel()

	id, err := h.svc.ResolveShiftID(ctx, shiftUUID)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if id == 0 {
		httpx.WriteError(w, http.StatusNotFound, "not found")
		return
	}
	if err := h.divider.DivideShift(ctx, id); err != nil {
		httpx.WriteError(w, http.StatusBadGateway, "couldn't divide this shift into line items")
		return
	}
	items, err := h.svc.ListItems(ctx, id)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if items == nil {
		items = []*billing.LineItem{}
	}
	httpx.WriteJSON(w, http.StatusOK, items)
}

// decodeItem decodes + validates a line-item body: quantity ≥ 0 and a line is
// either catalogue-coded or custom, not both. Writes a 400 and returns ok=false
// on failure.
func decodeItem(w http.ResponseWriter, r *http.Request) (billing.LineItemInput, bool) {
	var in billing.LineItemInput
	if err := httpx.DecodeJSON(r, &in); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request")
		return in, false
	}
	if in.Quantity < 0 {
		httpx.WriteError(w, http.StatusBadRequest, "quantity must not be negative")
		return in, false
	}
	if in.Code != "" && in.CustomItemID != nil {
		httpx.WriteError(w, http.StatusBadRequest, "a line is either catalogue-coded or custom, not both")
		return in, false
	}
	return in, true
}

// statusRequest is the body of UpdateStatus: the target lifecycle status.
type statusRequest struct {
	Status string `json:"status"`
}

// UpdateStatus advances a shift's lifecycle status by uuid. An empty status → 400.
func (h *Handler) UpdateStatus(w http.ResponseWriter, r *http.Request) {
	shiftUUID, ok := httpx.ParseUUID(r, "shiftUUID")
	if !ok {
		httpx.WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var req statusRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request")
		return
	}
	if req.Status == "" {
		httpx.WriteError(w, http.StatusBadRequest, "status required")
		return
	}
	if err := h.svc.UpdateStatus(r.Context(), shiftUUID, req.Status); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
