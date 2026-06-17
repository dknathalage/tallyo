package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/dknathalage/tallyo/internal/agent"
	"github.com/dknathalage/tallyo/internal/reqctx"
	"github.com/go-chi/chi/v5"
)

// AgentHandler serves the AI agent routes: conversation create/list, message
// history, message send (async), permission decisions, checkpoint revert, and a
// per-conversation SSE stream. Every handler 503s when the agent is disabled.
type AgentHandler struct {
	agent   *agent.Agent
	store   *agent.Store
	events  *agent.Events
	budget  *agent.Budget
	enabled bool
}

// NewAgentHandler constructs the handler. When ag is nil OR enabled is false the
// handler is registered but every route returns 503; this keeps wiring uniform
// (the route group is present) while the AI feature is off. A non-nil ag is
// required when enabled is true (programmer error otherwise).
func NewAgentHandler(ag *agent.Agent, budget *agent.Budget, enabled bool) *AgentHandler {
	if enabled && ag == nil {
		panic("NewAgentHandler: enabled handler requires a non-nil agent")
	}
	h := &AgentHandler{agent: ag, budget: budget, enabled: enabled}
	if ag != nil {
		h.store = ag.Store()
		h.events = ag.Events()
	}
	return h
}

// draftInvoiceRequest is the body of DraftInvoiceFromNotes: the inclusive note
// service-date range to bill.
type draftInvoiceRequest struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// DraftInvoiceFromNotes runs the AI agent over a participant's notes for a date
// range and creates the invoice WITHOUT human approval — the server
// auto-approves the agent's create_invoice. It reuses the full harness
// (catalogue-authoritative pricing, completeness verification, note billing) and
// returns the created invoice. Synchronous: it blocks for the agent run, on a
// detached context (bounded) so a client disconnect does not abort a model call.
func (h *AgentHandler) DraftInvoiceFromNotes(w http.ResponseWriter, r *http.Request) {
	tenantID, userID, ok := h.guard(w, r)
	if !ok {
		return
	}
	pid, ok := parseID(r)
	if !ok {
		WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var req draftInvoiceRequest
	if err := DecodeJSON(r, &req); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.From == "" || req.To == "" {
		WriteError(w, http.StatusBadRequest, "from and to are required")
		return
	}

	ctx, cancel := context.WithTimeout(detach(tenantID, userID), 5*time.Minute)
	defer cancel()

	conv, err := h.store.CreateConversation(ctx, "Invoice from notes")
	if err != nil {
		slog.Error("draft invoice: create conversation", slog.Any("error", err))
		WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}

	prompt := fmt.Sprintf("Draft an NDIS invoice for participant id %d for all supports "+
		"recorded in their notes between %s and %s. Read the notes, map each activity to the "+
		"correct NDIS support item code (prefer each note's candidates), and create the invoice "+
		"with notesFrom %s and notesTo %s.", pid, req.From, req.To, req.From, req.To)

	if err := h.agent.Start(ctx, conv.ID, prompt); err != nil {
		slog.Error("draft invoice: agent start", slog.Int64("conversationId", conv.ID), slog.Any("error", err))
		WriteError(w, http.StatusBadGateway, "the assistant could not draft the invoice")
		return
	}

	inv, err := h.autoApproveInvoice(ctx, conv.ID)
	if err != nil {
		slog.Error("draft invoice: auto-approve", slog.Int64("conversationId", conv.ID), slog.Any("error", err))
		WriteError(w, http.StatusBadGateway, "the assistant did not produce an invoice from these notes")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_, _ = w.Write(inv)
}

// autoApproveInvoice approves the agent's pending create_invoice for the
// conversation (no human gate) and returns the created invoice JSON (the
// approved step stores its result as the step summary). Bounded so a misbehaving
// model cannot loop. Returns an error when no invoice was produced.
func (h *AgentHandler) autoApproveInvoice(ctx context.Context, convID int64) (json.RawMessage, error) {
	const farFuture = "2999-01-01T00:00:00Z"
	for iter := 0; iter < 6; iter++ { // bounded
		steps, err := h.store.ListExpiredAwaitingSteps(ctx, farFuture)
		if err != nil {
			return nil, fmt.Errorf("list awaiting steps: %w", err)
		}
		var stepID int64
		for i := range steps { // bounded by len(steps)
			c, e := h.store.GetConversationByMessage(ctx, steps[i].MessageID)
			if e != nil {
				continue
			}
			if c.ID == convID {
				stepID = steps[i].ID
				break
			}
		}
		if stepID == 0 {
			break // no pending write for this conversation
		}
		if err := h.agent.Decide(ctx, stepID, true); err != nil {
			return nil, fmt.Errorf("approve step %d: %w", stepID, err)
		}
		done, err := h.store.GetStep(ctx, stepID)
		if err == nil && done.ToolName == "create_invoice" && done.Summary != "" && done.Summary != "null" {
			return json.RawMessage(done.Summary), nil
		}
	}
	return nil, fmt.Errorf("no invoice was produced")
}

// guard enforces the enabled flag and pulls the authenticated tenant+user from
// the request context (attached upstream by RequireAuth). It returns ok=false
// after writing the appropriate error response.
func (h *AgentHandler) guard(w http.ResponseWriter, r *http.Request) (tenantID, userID int64, ok bool) {
	if !h.enabled {
		WriteError(w, http.StatusServiceUnavailable, "AI not configured")
		return 0, 0, false
	}
	tid, tok := reqctx.TenantFrom(r.Context())
	if !tok || tid <= 0 {
		WriteError(w, http.StatusUnauthorized, "unauthorized")
		return 0, 0, false
	}
	uid, uok := reqctx.UserFrom(r.Context())
	if !uok || uid <= 0 {
		WriteError(w, http.StatusUnauthorized, "unauthorized")
		return 0, 0, false
	}
	return tid, uid, true
}

// detach derives a fresh background context carrying the request's tenant+user.
// The request context is canceled when the handler returns, so a goroutine that
// outlives the request (the plan/execute loop) must NOT inherit it.
func detach(tenantID, userID int64) context.Context {
	return reqctx.WithUser(reqctx.WithTenant(context.Background(), tenantID), userID)
}

// createConversationRequest is the create body.
type createConversationRequest struct {
	Title string `json:"title"`
}

// CreateConversation creates a conversation owned by the acting tenant+user.
func (h *AgentHandler) CreateConversation(w http.ResponseWriter, r *http.Request) {
	if _, _, ok := h.guard(w, r); !ok {
		return
	}
	var req createConversationRequest
	if err := DecodeJSON(r, &req); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	title := req.Title
	if title == "" {
		title = "New conversation"
	}
	conv, err := h.store.CreateConversation(r.Context(), title)
	if err != nil {
		LoggerFrom(r.Context()).Error("create conversation failed", slog.Any("error", err))
		WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	WriteJSON(w, http.StatusCreated, conv)
}

// ListConversations returns the acting tenant's conversations.
func (h *AgentHandler) ListConversations(w http.ResponseWriter, r *http.Request) {
	if _, _, ok := h.guard(w, r); !ok {
		return
	}
	convs, err := h.store.ListConversations(r.Context())
	if err != nil {
		LoggerFrom(r.Context()).Error("list conversations failed", slog.Any("error", err))
		WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	WriteJSON(w, http.StatusOK, convs)
}

// ListMessages returns a conversation's message history, 404 when the
// conversation is not owned by the acting tenant.
func (h *AgentHandler) ListMessages(w http.ResponseWriter, r *http.Request) {
	if _, _, ok := h.guard(w, r); !ok {
		return
	}
	id, ok := parseID(r)
	if !ok {
		WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	if _, err := h.store.GetConversation(r.Context(), id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			WriteError(w, http.StatusNotFound, "not found")
			return
		}
		LoggerFrom(r.Context()).Error("get conversation failed", slog.Any("error", err))
		WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	msgs, err := h.store.ListMessages(r.Context(), id)
	if err != nil {
		LoggerFrom(r.Context()).Error("list messages failed", slog.Any("error", err))
		WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	WriteJSON(w, http.StatusOK, msgs)
}

// sendMessageRequest is the send body.
type sendMessageRequest struct {
	Text string `json:"text"`
}

// SendMessage rate-limit-checks the user, verifies the conversation belongs to
// the tenant, then runs the plan/execute loop in a detached goroutine and
// returns 202. The client watches the SSE stream for results.
func (h *AgentHandler) SendMessage(w http.ResponseWriter, r *http.Request) {
	tenantID, userID, ok := h.guard(w, r)
	if !ok {
		return
	}
	id, ok := parseID(r)
	if !ok {
		WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var req sendMessageRequest
	if err := DecodeJSON(r, &req); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Text == "" {
		WriteError(w, http.StatusBadRequest, "text is required")
		return
	}
	// Rate-limit pre-check (per-user sliding window).
	if h.budget != nil && !h.budget.AllowMessage(r.Context(), userID) {
		WriteError(w, http.StatusTooManyRequests, "rate limit exceeded")
		return
	}
	// Ownership: only stream/run against the tenant's own conversation.
	if _, err := h.store.GetConversation(r.Context(), id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			WriteError(w, http.StatusNotFound, "not found")
			return
		}
		LoggerFrom(r.Context()).Error("get conversation failed", slog.Any("error", err))
		WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Run the loop detached: the request context is canceled when this handler
	// returns, which would kill blocking model calls mid-turn. Carry tenant+user
	// on a background-derived context so audited writes stay scoped.
	text := req.Text
	go func() {
		ctx := detach(tenantID, userID)
		if err := h.agent.Start(ctx, id, text); err != nil {
			slog.Error("agent start failed", slog.Int64("conversationId", id), slog.Any("error", err))
		}
	}()

	WriteJSON(w, http.StatusAccepted, map[string]any{"status": "accepted", "conversationId": id})
}

// decisionRequest is the permission-decision body.
type decisionRequest struct {
	Decision string `json:"decision"`
}

// Decide resolves an awaiting risky-tool step and resumes the loop. Like
// SendMessage it runs detached and returns 202.
func (h *AgentHandler) Decide(w http.ResponseWriter, r *http.Request) {
	tenantID, userID, ok := h.guard(w, r)
	if !ok {
		return
	}
	id, ok := parseID(r)
	if !ok {
		WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var req decisionRequest
	if err := DecodeJSON(r, &req); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	var allow bool
	switch req.Decision {
	case "allow":
		allow = true
	case "deny":
		allow = false
	default:
		WriteError(w, http.StatusBadRequest, "decision must be \"allow\" or \"deny\"")
		return
	}
	// Verify the step belongs to the acting tenant before resuming.
	if _, err := h.store.GetStep(r.Context(), id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			WriteError(w, http.StatusNotFound, "not found")
			return
		}
		LoggerFrom(r.Context()).Error("get step failed", slog.Any("error", err))
		WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}

	go func() {
		ctx := detach(tenantID, userID)
		if err := h.agent.Decide(ctx, id, allow); err != nil {
			slog.Error("agent decide failed", slog.Int64("stepId", id), slog.Any("error", err))
		}
	}()

	WriteJSON(w, http.StatusAccepted, map[string]any{"status": "accepted", "stepId": id})
}

// conflictJSON is the JSON shape of a skipped revert conflict.
type conflictJSON struct {
	Table string `json:"table"`
	PK    int64  `json:"pk"`
}

// Revert undoes a checkpoint's recorded changes and returns the conflicts it
// skipped. It runs synchronously: revert is bounded (one pass over the recorded
// changes) and the client needs the conflict list in the response.
func (h *AgentHandler) Revert(w http.ResponseWriter, r *http.Request) {
	if _, _, ok := h.guard(w, r); !ok {
		return
	}
	id, ok := parseID(r)
	if !ok {
		WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	// Ownership: scope the checkpoint to the acting tenant.
	if _, err := h.store.GetCheckpoint(r.Context(), id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			WriteError(w, http.StatusNotFound, "not found")
			return
		}
		LoggerFrom(r.Context()).Error("get checkpoint failed", slog.Any("error", err))
		WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	conflicts, err := h.agent.Revert(r.Context(), id)
	if err != nil {
		LoggerFrom(r.Context()).Error("revert failed", slog.Any("error", err))
		WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	out := make([]conflictJSON, 0, len(conflicts))
	for i := range conflicts { // bounded by len(conflicts)
		out = append(out, conflictJSON{Table: conflicts[i].Table, PK: conflicts[i].PK})
	}
	WriteJSON(w, http.StatusOK, map[string]any{"conflicts": out})
}

// Stream is the per-conversation SSE endpoint. It mirrors EventsHandler.Stream:
// text/event-stream headers, flush per event via http.ResponseController, and
// ends on client disconnect. It subscribes to the agent's per-conversation hub.
func (h *AgentHandler) Stream(w http.ResponseWriter, r *http.Request) {
	if _, _, ok := h.guard(w, r); !ok {
		return
	}
	convID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || convID <= 0 {
		WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	// Ownership: only stream a conversation the acting tenant owns.
	if _, err := h.store.GetConversation(r.Context(), convID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			WriteError(w, http.StatusNotFound, "not found")
			return
		}
		LoggerFrom(r.Context()).Error("get conversation failed", slog.Any("error", err))
		WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}

	rc := http.NewResponseController(w)
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	if err := rc.Flush(); err != nil {
		return
	}

	ch, unsub := h.events.Subscribe(convID)
	defer unsub()

	heartbeat := time.NewTicker(25 * time.Second)
	defer heartbeat.Stop()

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case <-heartbeat.C:
			if _, err := w.Write([]byte(": heartbeat\n\n")); err != nil {
				return
			}
			if err := rc.Flush(); err != nil {
				return
			}
		case e, ok := <-ch:
			if !ok {
				return // hub dropped us (overflow) → client reconnects
			}
			if !writeAgentFrame(w, e) {
				return
			}
			if err := rc.Flush(); err != nil {
				return
			}
		}
	}
}

// writeAgentFrame marshals an agent.Event and writes one SSE data frame. It
// returns false only when a write fails (client gone); a marshal error skips the
// event but keeps the stream alive (returns true).
func writeAgentFrame(w http.ResponseWriter, e agent.Event) bool {
	data, err := json.Marshal(e)
	if err != nil {
		return true // skip a bad event rather than kill the stream
	}
	if _, err := w.Write([]byte("data: ")); err != nil {
		return false
	}
	if _, err := w.Write(data); err != nil {
		return false
	}
	if _, err := w.Write([]byte("\n\n")); err != nil {
		return false
	}
	return true
}
