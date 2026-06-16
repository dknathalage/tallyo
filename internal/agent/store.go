// Package agent holds the AI agent: its persistence (Store), tool registry,
// checkpointing, and the plan/execute loop. This file is the Store — the typed
// data-access layer over the agent_* tables (migration 00002). Every read is
// scoped to the acting tenant (reqctx.MustTenant) and every mutation flows
// through audit.WithTx so it is recorded, per the codebase invariant.
package agent

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/dknathalage/tallyo/internal/agent/llm"
	"github.com/dknathalage/tallyo/internal/audit"
	"github.com/dknathalage/tallyo/internal/db/gen"
	"github.com/dknathalage/tallyo/internal/reqctx"
)

// Conversation is the domain view of an agent_conversation row.
type Conversation struct {
	ID         int64  `json:"id"`
	TenantID   int64  `json:"tenantId"`
	UserID     int64  `json:"userId"`
	Title      string `json:"title"`
	CreatedAt  string `json:"createdAt"`
	UpdatedAt  string `json:"updatedAt"`
	ArchivedAt string `json:"archivedAt,omitempty"`
}

// Message is the domain view of an agent_message row. Content is decoded from
// the JSON-encoded []llm.Block stored in the content column.
type Message struct {
	ID             int64       `json:"id"`
	ConversationID int64       `json:"conversationId"`
	TenantID       int64       `json:"tenantId"`
	Role           string      `json:"role"`
	Content        []llm.Block `json:"content"`
	TokenUsage     string      `json:"tokenUsage"`
	CreatedAt      string      `json:"createdAt"`
	// CheckpointID is the id of the checkpoint opened against this message (the
	// plan/assistant message that owns a turn's revertable changes), or nil when
	// the message has no checkpoint. CheckpointStatus is the checkpoint's status
	// ('open' | 'committed' | 'reverted') and is empty when CheckpointID is nil.
	CheckpointID     *int64 `json:"checkpointId"`
	CheckpointStatus string `json:"checkpointStatus,omitempty"`
}

// Store is the tenant-scoped persistence layer for the agent domain. It wraps
// the shared *sql.DB and the sqlc-generated Queries (constructed per call, as in
// the repository layer). A nil db is a programmer error.
type Store struct {
	db *sql.DB
	q  *gen.Queries
}

// NewStore constructs a Store. A nil db is a programmer error.
func NewStore(db *sql.DB) *Store {
	if db == nil {
		panic("agent: NewStore requires a non-nil *sql.DB")
	}
	return &Store{db: db, q: gen.New(db)}
}

// today returns the current UTC calendar day in the YYYY-MM-DD form used as the
// agent_token_usage day key.
func today() string {
	return time.Now().UTC().Format("2006-01-02")
}

// now returns the current UTC instant in the RFC3339 form used by every
// timestamp column in the schema.
func now() string {
	return time.Now().UTC().Format(time.RFC3339)
}

// ---------------------------------------------------------------------------
// Conversations
// ---------------------------------------------------------------------------

// CreateConversation inserts a conversation owned by the acting tenant+user and
// audits the create. The user id is sourced from reqctx (the authenticated
// caller); a missing user id is recorded as a zero owner.
func (s *Store) CreateConversation(ctx context.Context, title string) (*Conversation, error) {
	tenantID := reqctx.MustTenant(ctx)
	userID, _ := reqctx.UserFrom(ctx)
	ts := now()

	var out *Conversation
	err := audit.WithTx(ctx, s.db, audit.Entry{}, func(tx *sql.Tx) error {
		row, e := gen.New(tx).CreateAgentConversation(ctx, gen.CreateAgentConversationParams{
			TenantID:  tenantID,
			UserID:    userID,
			Title:     title,
			CreatedAt: ts,
			UpdatedAt: ts,
		})
		if e != nil {
			return fmt.Errorf("insert conversation: %w", e)
		}
		out = toConversation(row)
		return audit.Log(ctx, tx, audit.Entry{
			EntityType: "agent_conversation", EntityID: row.ID, Action: "create",
		})
	})
	if err != nil {
		return nil, fmt.Errorf("create conversation: %w", err)
	}
	return out, nil
}

// GetConversation returns one conversation by id, scoped to the acting tenant.
// A missing row (or a different tenant's row) returns sql.ErrNoRows.
func (s *Store) GetConversation(ctx context.Context, id int64) (*Conversation, error) {
	tenantID := reqctx.MustTenant(ctx)
	row, err := s.q.GetAgentConversation(ctx, gen.GetAgentConversationParams{TenantID: tenantID, ID: id})
	if err != nil {
		return nil, err
	}
	return toConversation(row), nil
}

// GetConversationByMessage returns the conversation owning the given message id,
// scoped to the acting tenant. A missing row returns sql.ErrNoRows.
func (s *Store) GetConversationByMessage(ctx context.Context, messageID int64) (*Conversation, error) {
	tenantID := reqctx.MustTenant(ctx)
	row, err := s.q.GetConversationByMessage(ctx, gen.GetConversationByMessageParams{TenantID: tenantID, ID: messageID})
	if err != nil {
		return nil, err
	}
	return toConversation(row), nil
}

// ListConversations returns the acting tenant's conversations, newest activity
// first. The slice is non-nil (empty when there are none).
func (s *Store) ListConversations(ctx context.Context) ([]*Conversation, error) {
	tenantID := reqctx.MustTenant(ctx)
	rows, err := s.q.ListAgentConversations(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list conversations: %w", err)
	}
	out := make([]*Conversation, 0, len(rows))
	for i := range rows { // bounded by len(rows)
		out = append(out, toConversation(rows[i]))
	}
	return out, nil
}

// TouchConversation bumps a conversation's updated_at to now, scoped to tenant.
func (s *Store) TouchConversation(ctx context.Context, id int64) error {
	tenantID := reqctx.MustTenant(ctx)
	if err := s.q.TouchAgentConversation(ctx, gen.TouchAgentConversationParams{
		UpdatedAt: now(), TenantID: tenantID, ID: id,
	}); err != nil {
		return fmt.Errorf("touch conversation: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Messages
// ---------------------------------------------------------------------------

// CreateMessage appends a message (role + content blocks) to a conversation and
// audits the create. Content is JSON-encoded from []llm.Block into the TEXT
// column. tokenUsage is the JSON usage blob ("{}" when none).
func (s *Store) CreateMessage(ctx context.Context, conversationID int64, role string, content []llm.Block, tokenUsage string) (*Message, error) {
	tenantID := reqctx.MustTenant(ctx)
	encoded, err := encodeBlocks(content)
	if err != nil {
		return nil, fmt.Errorf("create message: %w", err)
	}
	if tokenUsage == "" {
		tokenUsage = "{}"
	}

	var out *Message
	err = audit.WithTx(ctx, s.db, audit.Entry{}, func(tx *sql.Tx) error {
		row, e := gen.New(tx).CreateAgentMessage(ctx, gen.CreateAgentMessageParams{
			ConversationID: conversationID,
			TenantID:       tenantID,
			Role:           role,
			Content:        encoded,
			TokenUsage:     tokenUsage,
			CreatedAt:      now(),
		})
		if e != nil {
			return fmt.Errorf("insert message: %w", e)
		}
		m, e := toMessage(row)
		if e != nil {
			return e
		}
		out = m
		return audit.Log(ctx, tx, audit.Entry{
			EntityType: "agent_message", EntityID: row.ID, Action: "create",
		})
	})
	if err != nil {
		return nil, fmt.Errorf("create message: %w", err)
	}
	return out, nil
}

// ListMessages returns a conversation's messages in chronological order, scoped
// to the acting tenant. The slice is non-nil (empty when there are none).
func (s *Store) ListMessages(ctx context.Context, conversationID int64) ([]*Message, error) {
	tenantID := reqctx.MustTenant(ctx)
	rows, err := s.q.ListAgentMessages(ctx, gen.ListAgentMessagesParams{
		TenantID: tenantID, ConversationID: conversationID,
	})
	if err != nil {
		return nil, fmt.Errorf("list messages: %w", err)
	}
	out := make([]*Message, 0, len(rows))
	for i := range rows { // bounded by len(rows)
		m, e := toListMessage(rows[i])
		if e != nil {
			return nil, fmt.Errorf("list messages: %w", e)
		}
		out = append(out, m)
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// Steps
// ---------------------------------------------------------------------------

// CreateStep inserts a step under a message and audits the create. Params are
// passed through verbatim; the tenant id is injected from ctx.
func (s *Store) CreateStep(ctx context.Context, p gen.CreateAgentStepParams) (gen.AgentStep, error) {
	tenantID := reqctx.MustTenant(ctx)
	p.TenantID = tenantID
	if p.CreatedAt == "" {
		p.CreatedAt = now()
	}
	var out gen.AgentStep
	err := audit.WithTx(ctx, s.db, audit.Entry{}, func(tx *sql.Tx) error {
		row, e := gen.New(tx).CreateAgentStep(ctx, p)
		if e != nil {
			return fmt.Errorf("insert step: %w", e)
		}
		out = row
		return audit.Log(ctx, tx, audit.Entry{
			EntityType: "agent_step", EntityID: row.ID, Action: "create",
		})
	})
	if err != nil {
		return gen.AgentStep{}, fmt.Errorf("create step: %w", err)
	}
	return out, nil
}

// CreateAwaitingStep inserts a step in the 'awaiting' state (a risky tool call
// pending user permission) and audits the create. The tenant id is injected.
func (s *Store) CreateAwaitingStep(ctx context.Context, p gen.CreateAwaitingStepParams) (gen.AgentStep, error) {
	tenantID := reqctx.MustTenant(ctx)
	p.TenantID = tenantID
	if p.CreatedAt == "" {
		p.CreatedAt = now()
	}
	var out gen.AgentStep
	err := audit.WithTx(ctx, s.db, audit.Entry{}, func(tx *sql.Tx) error {
		row, e := gen.New(tx).CreateAwaitingStep(ctx, p)
		if e != nil {
			return fmt.Errorf("insert awaiting step: %w", e)
		}
		out = row
		return audit.Log(ctx, tx, audit.Entry{
			EntityType: "agent_step", EntityID: row.ID, Action: "create",
		})
	})
	if err != nil {
		return gen.AgentStep{}, fmt.Errorf("create awaiting step: %w", err)
	}
	return out, nil
}

// UpdateStepStatus sets a step's status and result, scoped to tenant, auditing
// the update.
func (s *Store) UpdateStepStatus(ctx context.Context, id int64, status, result string) error {
	tenantID := reqctx.MustTenant(ctx)
	return audit.WithTx(ctx, s.db, audit.Entry{
		EntityType: "agent_step", EntityID: id, Action: "update",
		Changes: audit.Changes(map[string]any{"status": status}),
	}, func(tx *sql.Tx) error {
		if e := gen.New(tx).UpdateAgentStepStatus(ctx, gen.UpdateAgentStepStatusParams{
			Status: status, Result: result, TenantID: tenantID, ID: id,
		}); e != nil {
			return fmt.Errorf("update step status: %w", e)
		}
		return nil
	})
}

// GetStep returns one step by id, scoped to the acting tenant. A missing row
// returns sql.ErrNoRows.
func (s *Store) GetStep(ctx context.Context, id int64) (gen.AgentStep, error) {
	tenantID := reqctx.MustTenant(ctx)
	return s.q.GetAgentStep(ctx, gen.GetAgentStepParams{TenantID: tenantID, ID: id})
}

// ListSteps returns a message's steps in ordinal order, scoped to tenant.
func (s *Store) ListSteps(ctx context.Context, messageID int64) ([]gen.AgentStep, error) {
	tenantID := reqctx.MustTenant(ctx)
	rows, err := s.q.ListAgentSteps(ctx, gen.ListAgentStepsParams{TenantID: tenantID, MessageID: messageID})
	if err != nil {
		return nil, fmt.Errorf("list steps: %w", err)
	}
	if rows == nil {
		rows = []gen.AgentStep{}
	}
	return rows, nil
}

// ListExpiredAwaitingSteps returns awaiting steps whose await_expires_at is
// before the given cutoff (RFC3339). This is the global sweep path and is NOT
// tenant-scoped; the caller fences expired permissions per row's tenant_id.
func (s *Store) ListExpiredAwaitingSteps(ctx context.Context, cutoff string) ([]gen.AgentStep, error) {
	rows, err := s.q.ListExpiredAwaitingSteps(ctx, sql.NullString{String: cutoff, Valid: cutoff != ""})
	if err != nil {
		return nil, fmt.Errorf("list expired awaiting steps: %w", err)
	}
	if rows == nil {
		rows = []gen.AgentStep{}
	}
	return rows, nil
}

// ---------------------------------------------------------------------------
// Checkpoints
// ---------------------------------------------------------------------------

// CreateCheckpoint inserts a checkpoint for a message and audits the create.
func (s *Store) CreateCheckpoint(ctx context.Context, messageID int64, status string) (gen.AgentCheckpoint, error) {
	tenantID := reqctx.MustTenant(ctx)
	var out gen.AgentCheckpoint
	err := audit.WithTx(ctx, s.db, audit.Entry{}, func(tx *sql.Tx) error {
		row, e := gen.New(tx).CreateCheckpoint(ctx, gen.CreateCheckpointParams{
			MessageID: messageID, TenantID: tenantID, Status: status, CreatedAt: now(),
		})
		if e != nil {
			return fmt.Errorf("insert checkpoint: %w", e)
		}
		out = row
		return audit.Log(ctx, tx, audit.Entry{
			EntityType: "agent_checkpoint", EntityID: row.ID, Action: "create",
		})
	})
	if err != nil {
		return gen.AgentCheckpoint{}, fmt.Errorf("create checkpoint: %w", err)
	}
	return out, nil
}

// UpdateCheckpointStatus sets a checkpoint's status, scoped to tenant, auditing
// the update.
func (s *Store) UpdateCheckpointStatus(ctx context.Context, id int64, status string) error {
	tenantID := reqctx.MustTenant(ctx)
	return audit.WithTx(ctx, s.db, audit.Entry{
		EntityType: "agent_checkpoint", EntityID: id, Action: "update",
		Changes: audit.Changes(map[string]any{"status": status}),
	}, func(tx *sql.Tx) error {
		if e := gen.New(tx).UpdateCheckpointStatus(ctx, gen.UpdateCheckpointStatusParams{
			Status: status, TenantID: tenantID, ID: id,
		}); e != nil {
			return fmt.Errorf("update checkpoint status: %w", e)
		}
		return nil
	})
}

// MarkCheckpointReverted flips a checkpoint to 'reverted' and stamps reverted_at,
// scoped to tenant, auditing the update.
func (s *Store) MarkCheckpointReverted(ctx context.Context, id int64) error {
	tenantID := reqctx.MustTenant(ctx)
	return audit.WithTx(ctx, s.db, audit.Entry{
		EntityType: "agent_checkpoint", EntityID: id, Action: "revert",
	}, func(tx *sql.Tx) error {
		if e := gen.New(tx).MarkCheckpointReverted(ctx, gen.MarkCheckpointRevertedParams{
			RevertedAt: sql.NullString{String: now(), Valid: true}, TenantID: tenantID, ID: id,
		}); e != nil {
			return fmt.Errorf("mark checkpoint reverted: %w", e)
		}
		return nil
	})
}

// GetCheckpoint returns one checkpoint by id, scoped to tenant. A missing row
// returns sql.ErrNoRows.
func (s *Store) GetCheckpoint(ctx context.Context, id int64) (gen.AgentCheckpoint, error) {
	tenantID := reqctx.MustTenant(ctx)
	return s.q.GetCheckpoint(ctx, gen.GetCheckpointParams{TenantID: tenantID, ID: id})
}

// CreateCheckpointChange records one before/after row change under a checkpoint
// and audits the create. The tenant id is injected from ctx.
func (s *Store) CreateCheckpointChange(ctx context.Context, p gen.CreateCheckpointChangeParams) (gen.AgentCheckpointChange, error) {
	tenantID := reqctx.MustTenant(ctx)
	p.TenantID = tenantID
	if p.CreatedAt == "" {
		p.CreatedAt = now()
	}
	var out gen.AgentCheckpointChange
	err := audit.WithTx(ctx, s.db, audit.Entry{}, func(tx *sql.Tx) error {
		row, e := gen.New(tx).CreateCheckpointChange(ctx, p)
		if e != nil {
			return fmt.Errorf("insert checkpoint change: %w", e)
		}
		out = row
		return audit.Log(ctx, tx, audit.Entry{
			EntityType: "agent_checkpoint_change", EntityID: row.ID, Action: "create",
		})
	})
	if err != nil {
		return gen.AgentCheckpointChange{}, fmt.Errorf("create checkpoint change: %w", err)
	}
	return out, nil
}

// ListCheckpointChanges returns a checkpoint's changes in reverse-ordinal order
// (newest first — the order in which a revert replays them), scoped to tenant.
func (s *Store) ListCheckpointChanges(ctx context.Context, checkpointID int64) ([]gen.AgentCheckpointChange, error) {
	tenantID := reqctx.MustTenant(ctx)
	rows, err := s.q.ListCheckpointChanges(ctx, gen.ListCheckpointChangesParams{
		TenantID: tenantID, CheckpointID: checkpointID,
	})
	if err != nil {
		return nil, fmt.Errorf("list checkpoint changes: %w", err)
	}
	if rows == nil {
		rows = []gen.AgentCheckpointChange{}
	}
	return rows, nil
}

// ---------------------------------------------------------------------------
// Token usage / retention
// ---------------------------------------------------------------------------

// AddTokenUsage adds tokens to the acting tenant's running total for the given
// day (UPSERT). Pass today() for the current day.
func (s *Store) AddTokenUsage(ctx context.Context, day string, tokens int64) error {
	tenantID := reqctx.MustTenant(ctx)
	if err := s.q.AddTokenUsage(ctx, gen.AddTokenUsageParams{
		TenantID: tenantID, Day: day, Tokens: tokens,
	}); err != nil {
		return fmt.Errorf("add token usage: %w", err)
	}
	return nil
}

// GetTokenUsage returns the acting tenant's token total for the given day, or 0
// when there is no row.
func (s *Store) GetTokenUsage(ctx context.Context, day string) (int64, error) {
	tenantID := reqctx.MustTenant(ctx)
	total, err := s.q.GetTokenUsage(ctx, gen.GetTokenUsageParams{TenantID: tenantID, Day: day})
	if err != nil {
		return 0, fmt.Errorf("get token usage: %w", err)
	}
	return total, nil
}

// PruneCheckpointChanges deletes checkpoint-change rows created before cutoff
// (RFC3339). Global retention sweep; not tenant-scoped.
func (s *Store) PruneCheckpointChanges(ctx context.Context, cutoff string) error {
	if err := s.q.PruneCheckpointChanges(ctx, cutoff); err != nil {
		return fmt.Errorf("prune checkpoint changes: %w", err)
	}
	return nil
}

// PruneSteps deletes step rows created before cutoff (RFC3339). Global retention
// sweep; not tenant-scoped.
func (s *Store) PruneSteps(ctx context.Context, cutoff string) error {
	if err := s.q.PruneAgentSteps(ctx, cutoff); err != nil {
		return fmt.Errorf("prune steps: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// mapping helpers
// ---------------------------------------------------------------------------

// encodeBlocks JSON-encodes content blocks for the agent_message.content column.
// A nil slice encodes to an empty JSON array so the column shape is uniform.
func encodeBlocks(blocks []llm.Block) (string, error) {
	if blocks == nil {
		blocks = []llm.Block{}
	}
	b, err := json.Marshal(blocks)
	if err != nil {
		return "", fmt.Errorf("encode blocks: %w", err)
	}
	return string(b), nil
}

// decodeBlocks decodes the agent_message.content column back into blocks.
func decodeBlocks(s string) ([]llm.Block, error) {
	if s == "" {
		return []llm.Block{}, nil
	}
	var blocks []llm.Block
	if err := json.Unmarshal([]byte(s), &blocks); err != nil {
		return nil, fmt.Errorf("decode blocks: %w", err)
	}
	if blocks == nil {
		blocks = []llm.Block{}
	}
	return blocks, nil
}

// toConversation maps a generated conversation row to the domain shape.
func toConversation(r gen.AgentConversation) *Conversation {
	return &Conversation{
		ID:         r.ID,
		TenantID:   r.TenantID,
		UserID:     r.UserID,
		Title:      r.Title,
		CreatedAt:  r.CreatedAt,
		UpdatedAt:  r.UpdatedAt,
		ArchivedAt: r.ArchivedAt.String,
	}
}

// toMessage maps a generated message row to the domain shape, decoding content.
func toMessage(r gen.AgentMessage) (*Message, error) {
	blocks, err := decodeBlocks(r.Content)
	if err != nil {
		return nil, err
	}
	return &Message{
		ID:             r.ID,
		ConversationID: r.ConversationID,
		TenantID:       r.TenantID,
		Role:           r.Role,
		Content:        blocks,
		TokenUsage:     r.TokenUsage,
		CreatedAt:      r.CreatedAt,
	}, nil
}

// toListMessage maps a ListAgentMessages row (message joined with its optional
// checkpoint) to the domain shape, decoding content and surfacing the nullable
// checkpoint id + status. A NULL checkpoint id leaves CheckpointID nil and
// CheckpointStatus empty (most messages have no checkpoint).
func toListMessage(r gen.ListAgentMessagesRow) (*Message, error) {
	blocks, err := decodeBlocks(r.Content)
	if err != nil {
		return nil, err
	}
	m := &Message{
		ID:             r.ID,
		ConversationID: r.ConversationID,
		TenantID:       r.TenantID,
		Role:           r.Role,
		Content:        blocks,
		TokenUsage:     r.TokenUsage,
		CreatedAt:      r.CreatedAt,
	}
	if r.CheckpointID.Valid {
		id := r.CheckpointID.Int64
		m.CheckpointID = &id
		m.CheckpointStatus = r.CheckpointStatus.String
	}
	return m, nil
}
