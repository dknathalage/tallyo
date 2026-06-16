package agent

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/dknathalage/tallyo/internal/agent/llm"
	"github.com/dknathalage/tallyo/internal/db/gen"
)

// sqlInt64 wraps a non-zero id as a valid nullable column (NULL when zero).
func sqlInt64(v int64) sql.NullInt64 {
	return sql.NullInt64{Int64: v, Valid: v != 0}
}

// sqlString wraps a non-empty string as a valid nullable column (NULL empty).
func sqlString(v string) sql.NullString {
	return sql.NullString{String: v, Valid: v != ""}
}

// --- tool_result persistence convention -------------------------------------
//
// A user turn answering a tool_use is persisted as an agent_message with
// role="user" whose content is exactly ONE llm.Block encoding the tool result:
//
//	Block{
//	  Type:      llm.BlockToolUse,   // reuse the existing block type
//	  ToolUseID: <original tool_use id>,
//	  ToolName:  toolResultMarker | toolResultErrorMarker,  // sentinel marker
//	  Text:      <JSON-encoded tool output / error message>,
//	}
//
// loadHistory recognises these sentinel-marked blocks and rebuilds them as an
// llm.Message{Role: RoleUser, ToolResults: [...]} so the provider adapter emits
// a real tool_result block keyed by the original tool_use id, with IsError
// preserved. Task 9's resume reconstructs the model window from exactly this.
const (
	toolResultMarker      = "__tool_result__"
	toolResultErrorMarker = "__tool_result_error__"
)

// clock abstracts time for deterministic await-expiry stamping in tests.
type clock interface{ Now() time.Time }

type wallClock struct{}

func (wallClock) Now() time.Time { return time.Now() }

// Agent orchestrates a conversation turn: a forced propose_plan phase followed
// by a bounded execute loop where read/meta tools auto-run and risky tools
// suspend for user approval.
type Agent struct {
	cfg    Config
	llm    llm.Client
	store  *Store
	reg    *Registry
	cp     *Checkpoint
	events *Events
	clock  clock
}

// NewAgent constructs an Agent and ensures the propose_plan meta tool is
// registered. A nil dependency is a programmer error.
func NewAgent(cfg Config, client llm.Client, store *Store, reg *Registry, cp *Checkpoint, events *Events) *Agent {
	if client == nil || store == nil || reg == nil || cp == nil || events == nil {
		panic("agent: NewAgent requires non-nil client, store, reg, cp, events")
	}
	RegisterMetaTools(reg)
	return &Agent{
		cfg:    cfg.WithDefaults(),
		llm:    client,
		store:  store,
		reg:    reg,
		cp:     cp,
		events: events,
		clock:  wallClock{},
	}
}

// Start runs one user turn end-to-end: persist the user message, run the plan
// phase, open a checkpoint for the plan message, then run the execute loop.
func (a *Agent) Start(ctx context.Context, convID int64, userText string) error {
	if convID <= 0 {
		return fmt.Errorf("start: invalid convID %d", convID)
	}
	if userText == "" {
		return fmt.Errorf("start: empty user text")
	}

	userMsg, err := a.store.CreateMessage(ctx, convID, "user",
		[]llm.Block{{Type: llm.BlockText, Text: userText}}, "{}")
	if err != nil {
		return fmt.Errorf("start: persist user message: %w", err)
	}

	_, planMsgID, err := a.plan(ctx, convID, userMsg.ID)
	if err != nil {
		a.events.Publish(convID, Event{Type: "error", Data: "planning failed"})
		return fmt.Errorf("start: plan: %w", err)
	}

	checkpointID, err := a.cp.Open(ctx, planMsgID)
	if err != nil {
		return fmt.Errorf("start: open checkpoint: %w", err)
	}

	if err := a.Execute(ctx, convID, checkpointID, planMsgID); err != nil {
		return fmt.Errorf("start: execute: %w", err)
	}
	return nil
}

// Execute runs the bounded model/tool loop. Read and meta tools run inline and
// feed their results back; the first risky tool suspends the turn for approval
// (resume is Task 9). The loop is bounded by cfg.MaxIterations (rule 2).
func (a *Agent) Execute(ctx context.Context, convID, checkpointID, messageID int64) error {
	if convID <= 0 || checkpointID <= 0 || messageID <= 0 {
		return fmt.Errorf("execute: invalid convID=%d checkpointID=%d messageID=%d", convID, checkpointID, messageID)
	}

	for i := 0; i < a.cfg.MaxIterations; i++ { // bounded by MaxIterations
		// TODO(task10): budget check (skip the turn / suspend when over budget).

		req := buildRequest(a.cfg, a.reg, defaultSystemPrompt, a.loadHistory(ctx, convID), "")
		resp, err := a.llm.CreateMessage(ctx, req)
		if err != nil {
			return fmt.Errorf("execute: model call: %w", err)
		}
		if resp == nil {
			return fmt.Errorf("execute: nil model response")
		}
		if _, err := a.persistAssistant(ctx, convID, resp); err != nil {
			return fmt.Errorf("execute: persist assistant: %w", err)
		}

		switch resp.StopReason {
		case llm.StopRefusal:
			a.events.Publish(convID, Event{Type: "error", Data: "model declined"})
			return a.commitCheckpoint(ctx, checkpointID)
		case llm.StopToolUse:
			// fall through to tool handling below
		default:
			a.events.Publish(convID, Event{Type: "message_final", Data: finalText(resp)})
			return a.commitCheckpoint(ctx, checkpointID)
		}

		suspended, err := a.runTools(ctx, convID, messageID, checkpointID, resp)
		if err != nil {
			return fmt.Errorf("execute: run tools: %w", err)
		}
		if suspended {
			return nil // awaiting approval; resume happens via Task 9's Decide
		}
		// All reads handled; continue to the next model turn.
	}

	a.events.Publish(convID, Event{Type: "error", Data: "max iterations reached"})
	return a.commitCheckpoint(ctx, checkpointID)
}

// runTools handles every tool_use block in resp. It returns suspended=true when
// a risky tool was suspended for approval (the caller must stop the loop).
func (a *Agent) runTools(ctx context.Context, convID, messageID, checkpointID int64, resp *llm.Response) (bool, error) {
	uses := toolUses(resp)
	for i := range uses { // bounded by len(uses)
		b := uses[i]
		tool, ok := a.reg.Get(b.ToolName)
		if !ok {
			a.feedToolError(ctx, convID, b.ToolUseID, fmt.Sprintf("unknown tool %q", b.ToolName))
			continue
		}
		if tool.Risk == RiskRisky {
			if err := a.suspendForApproval(ctx, convID, messageID, checkpointID, b); err != nil {
				return false, fmt.Errorf("suspend %q: %w", b.ToolName, err)
			}
			return true, nil
		}

		res, err := tool.Handler(withCheckpoint(ctx, checkpointID), b.Input)
		if err != nil {
			a.feedToolError(ctx, convID, b.ToolUseID, err.Error())
			continue
		}
		if _, e := a.store.CreateStep(ctx, gen.CreateAgentStepParams{
			MessageID: messageID,
			Ordinal:   int64(i),
			ToolName:  b.ToolName,
			ToolUseID: b.ToolUseID,
			Summary:   "",
			Risk:      string(tool.Risk),
			Status:    "done",
		}); e != nil {
			return false, fmt.Errorf("persist step %q: %w", b.ToolName, e)
		}
		a.feedToolResult(ctx, convID, b.ToolUseID, res, false)
	}
	return false, nil
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// loadHistory loads the conversation's messages and maps them to []llm.Message
// for replay. Assistant/user text and tool_use blocks replay verbatim; a
// persisted tool-result user message (per the convention above) is rebuilt as
// an llm.Message with ToolResults so the adapter emits a tool_result block.
func (a *Agent) loadHistory(ctx context.Context, convID int64) []llm.Message {
	if convID <= 0 {
		return nil
	}
	msgs, err := a.store.ListMessages(ctx, convID)
	if err != nil {
		return nil
	}
	out := make([]llm.Message, 0, len(msgs))
	for i := range msgs { // bounded by len(msgs)
		m := msgs[i]
		if tr, ok := toToolResult(m.Content); ok {
			out = append(out, llm.Message{Role: llm.RoleUser, ToolResults: []llm.ToolResult{tr}})
			continue
		}
		out = append(out, llm.Message{Role: llm.Role(m.Role), Content: m.Content})
	}
	return out
}

// toToolResult recognises a persisted tool-result block (per the convention)
// and converts it to an llm.ToolResult.
func toToolResult(blocks []llm.Block) (llm.ToolResult, bool) {
	if len(blocks) != 1 {
		return llm.ToolResult{}, false
	}
	b := blocks[0]
	switch b.ToolName {
	case toolResultMarker:
		return llm.ToolResult{ToolUseID: b.ToolUseID, Content: b.Text, IsError: false}, true
	case toolResultErrorMarker:
		return llm.ToolResult{ToolUseID: b.ToolUseID, Content: b.Text, IsError: true}, true
	default:
		return llm.ToolResult{}, false
	}
}

// feedToolResult persists a successful tool-result user message (per the
// convention) and publishes a tool_result event.
func (a *Agent) feedToolResult(ctx context.Context, convID int64, toolUseID string, res Result, isErr bool) {
	content := encodeResultJSON(res.JSON)
	a.persistToolResult(ctx, convID, toolUseID, content, isErr)
	a.events.Publish(convID, Event{Type: "tool_result", Data: map[string]any{
		"toolUseId": toolUseID, "render": res.Render, "result": res.JSON, "isError": isErr,
	}})
}

// feedToolError persists an is_error tool-result user message and publishes a
// tool_result event flagged as an error.
func (a *Agent) feedToolError(ctx context.Context, convID int64, toolUseID, msg string) {
	a.persistToolResult(ctx, convID, toolUseID, msg, true)
	a.events.Publish(convID, Event{Type: "tool_result", Data: map[string]any{
		"toolUseId": toolUseID, "error": msg, "isError": true,
	}})
}

// persistToolResult writes the tool-result user message per the convention. A
// persistence failure is non-fatal to the loop (best-effort), but it is
// surfaced as an error event so the turn is not silently corrupted.
func (a *Agent) persistToolResult(ctx context.Context, convID int64, toolUseID, content string, isErr bool) {
	marker := toolResultMarker
	if isErr {
		marker = toolResultErrorMarker
	}
	block := llm.Block{Type: llm.BlockToolUse, ToolUseID: toolUseID, ToolName: marker, Text: content}
	if _, err := a.store.CreateMessage(ctx, convID, "user", []llm.Block{block}, "{}"); err != nil {
		a.events.Publish(convID, Event{Type: "error", Data: "failed to persist tool result"})
	}
}

// persistAssistant stores the assistant response (raw blocks + token usage).
func (a *Agent) persistAssistant(ctx context.Context, convID int64, resp *llm.Response) (*Message, error) {
	if resp == nil {
		return nil, fmt.Errorf("persist assistant: nil response")
	}
	usage := encodeUsage(resp.Usage)
	msg, err := a.store.CreateMessage(ctx, convID, "assistant", resp.Content, usage)
	if err != nil {
		return nil, fmt.Errorf("persist assistant: %w", err)
	}
	return msg, nil
}

// suspendForApproval persists an awaiting step for a risky tool call and
// publishes an access_request event. No goroutine blocks; resume is Task 9.
func (a *Agent) suspendForApproval(ctx context.Context, convID, messageID, checkpointID int64, b llm.Block) error {
	if messageID <= 0 || checkpointID <= 0 {
		return fmt.Errorf("suspend: invalid messageID=%d checkpointID=%d", messageID, checkpointID)
	}
	if b.ToolUseID == "" {
		return fmt.Errorf("suspend: empty tool_use id")
	}
	expires := a.clock.Now().UTC().Add(time.Duration(a.cfg.AwaitTTLMinutes) * time.Minute).Format(time.RFC3339)
	step, err := a.store.CreateAwaitingStep(ctx, gen.CreateAwaitingStepParams{
		MessageID:      messageID,
		CheckpointID:   sqlInt64(checkpointID),
		Ordinal:        0,
		ToolName:       b.ToolName,
		ToolUseID:      b.ToolUseID,
		Summary:        fmt.Sprintf("Run %s", b.ToolName),
		Risk:           string(RiskRisky),
		PendingInput:   string(b.Input),
		AwaitExpiresAt: sqlString(expires),
	})
	if err != nil {
		return fmt.Errorf("suspend: persist awaiting step: %w", err)
	}
	a.events.Publish(convID, Event{Type: "access_request", Data: map[string]any{
		"stepId":    step.ID,
		"toolName":  b.ToolName,
		"toolUseId": b.ToolUseID,
		"summary":   fmt.Sprintf("Approve running %s?", b.ToolName),
		"input":     json.RawMessage(b.Input),
		"expiresAt": expires,
	}})
	return nil
}

// commitCheckpoint marks the turn's checkpoint committed.
func (a *Agent) commitCheckpoint(ctx context.Context, checkpointID int64) error {
	if checkpointID <= 0 {
		return fmt.Errorf("commit checkpoint: invalid id %d", checkpointID)
	}
	if err := a.store.UpdateCheckpointStatus(ctx, checkpointID, "committed"); err != nil {
		return fmt.Errorf("commit checkpoint: %w", err)
	}
	return nil
}

// finalText concatenates the text blocks of a response into the final message.
func finalText(resp *llm.Response) string {
	if resp == nil {
		return ""
	}
	out := ""
	for i := range resp.Content { // bounded by len(resp.Content)
		if resp.Content[i].Type == llm.BlockText {
			out += resp.Content[i].Text
		}
	}
	return out
}

// toolUses returns the tool_use blocks of a response.
func toolUses(resp *llm.Response) []llm.Block {
	if resp == nil {
		return nil
	}
	out := make([]llm.Block, 0, len(resp.Content))
	for i := range resp.Content { // bounded by len(resp.Content)
		if resp.Content[i].Type == llm.BlockToolUse {
			out = append(out, resp.Content[i])
		}
	}
	return out
}

// encodeResultJSON JSON-encodes a tool result's JSON payload for storage and
// replay. A marshal failure degrades to a small error envelope rather than
// dropping the result.
func encodeResultJSON(v any) string {
	if v == nil {
		return "null"
	}
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("{\"error\":\"encode result: %s\"}", err.Error())
	}
	return string(b)
}

// encodeUsage JSON-encodes token usage for the agent_message.token_usage column.
func encodeUsage(u llm.Usage) string {
	b, err := json.Marshal(u)
	if err != nil {
		return "{}"
	}
	return string(b)
}
