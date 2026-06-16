package agent

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/dknathalage/tallyo/internal/agent/llm"
	appdb "github.com/dknathalage/tallyo/internal/db"
	"github.com/dknathalage/tallyo/internal/reqctx"
	"github.com/google/uuid"
)

func TestStoreConversationAndMessageRoundTrip(t *testing.T) {
	s := newTestStore(t)
	tenantID, userID := seedTenantUser(t, s.db)
	ctx := reqctx.WithUser(reqctx.WithTenant(context.Background(), tenantID), userID)

	conv, err := s.CreateConversation(ctx, "First chat")
	if err != nil {
		t.Fatalf("CreateConversation: %v", err)
	}
	if conv.ID == 0 {
		t.Fatalf("CreateConversation: got id 0")
	}

	got, err := s.GetConversation(ctx, conv.ID)
	if err != nil {
		t.Fatalf("GetConversation: %v", err)
	}
	if got.ID != conv.ID || got.Title != "First chat" {
		t.Fatalf("GetConversation = %+v, want id=%d title=First chat", got, conv.ID)
	}

	blocks := []llm.Block{
		{Type: llm.BlockText, Text: "hello"},
		{Type: llm.BlockToolUse, ToolUseID: "tu_1", ToolName: "list_invoices", Input: json.RawMessage(`{"x":1}`)},
	}
	msg, err := s.CreateMessage(ctx, conv.ID, "assistant", blocks, "{}")
	if err != nil {
		t.Fatalf("CreateMessage: %v", err)
	}
	if msg.ID == 0 {
		t.Fatalf("CreateMessage: got id 0")
	}

	msgs, err := s.ListMessages(ctx, conv.ID)
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("ListMessages: got %d, want 1", len(msgs))
	}
	if len(msgs[0].Content) != 2 {
		t.Fatalf("content round-trip: got %d blocks, want 2", len(msgs[0].Content))
	}
	if msgs[0].Content[0].Text != "hello" || msgs[0].Content[1].ToolName != "list_invoices" {
		t.Fatalf("content round-trip mismatch: %+v", msgs[0].Content)
	}
}

func TestStoreListMessagesSurfacesCheckpoint(t *testing.T) {
	s := newTestStore(t)
	tenantID, userID := seedTenantUser(t, s.db)
	ctx := reqctx.WithUser(reqctx.WithTenant(context.Background(), tenantID), userID)

	conv, err := s.CreateConversation(ctx, "chat")
	if err != nil {
		t.Fatalf("CreateConversation: %v", err)
	}

	// A plan/assistant message that owns a checkpoint.
	planMsg, err := s.CreateMessage(ctx, conv.ID, "assistant", []llm.Block{{Type: llm.BlockText, Text: "plan"}}, "{}")
	if err != nil {
		t.Fatalf("CreateMessage plan: %v", err)
	}
	// A plain user message with no checkpoint.
	userMsg, err := s.CreateMessage(ctx, conv.ID, "user", []llm.Block{{Type: llm.BlockText, Text: "hi"}}, "{}")
	if err != nil {
		t.Fatalf("CreateMessage user: %v", err)
	}

	chk, err := s.CreateCheckpoint(ctx, planMsg.ID, "committed")
	if err != nil {
		t.Fatalf("CreateCheckpoint: %v", err)
	}

	msgs, err := s.ListMessages(ctx, conv.ID)
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("ListMessages: got %d, want 2", len(msgs))
	}

	byID := map[int64]*Message{}
	for _, m := range msgs {
		byID[m.ID] = m
	}

	got := byID[planMsg.ID]
	if got == nil {
		t.Fatalf("plan message missing from list")
	}
	if got.CheckpointID == nil {
		t.Fatalf("plan message CheckpointID = nil, want %d", chk.ID)
	}
	if *got.CheckpointID != chk.ID {
		t.Fatalf("plan message CheckpointID = %d, want %d", *got.CheckpointID, chk.ID)
	}
	if got.CheckpointStatus != "committed" {
		t.Fatalf("plan message CheckpointStatus = %q, want committed", got.CheckpointStatus)
	}

	plain := byID[userMsg.ID]
	if plain == nil {
		t.Fatalf("user message missing from list")
	}
	if plain.CheckpointID != nil {
		t.Fatalf("user message CheckpointID = %d, want nil", *plain.CheckpointID)
	}
	if plain.CheckpointStatus != "" {
		t.Fatalf("user message CheckpointStatus = %q, want empty", plain.CheckpointStatus)
	}
}

func TestStoreListMessagesCheckpointTenantScoped(t *testing.T) {
	s := newTestStore(t)
	tenantA, userA := seedTenantUser(t, s.db)
	ctxA := reqctx.WithUser(reqctx.WithTenant(context.Background(), tenantA), userA)

	conv, err := s.CreateConversation(ctxA, "A chat")
	if err != nil {
		t.Fatalf("CreateConversation: %v", err)
	}
	msg, err := s.CreateMessage(ctxA, conv.ID, "assistant", []llm.Block{{Type: llm.BlockText, Text: "plan"}}, "{}")
	if err != nil {
		t.Fatalf("CreateMessage: %v", err)
	}
	if _, err := s.CreateCheckpoint(ctxA, msg.ID, "committed"); err != nil {
		t.Fatalf("CreateCheckpoint: %v", err)
	}

	// Tenant A sees the checkpoint on its own message.
	msgs, err := s.ListMessages(ctxA, conv.ID)
	if err != nil {
		t.Fatalf("ListMessages A: %v", err)
	}
	if len(msgs) != 1 || msgs[0].CheckpointID == nil {
		t.Fatalf("tenant A expected one message with a checkpoint, got %+v", msgs)
	}
}

func TestStoreTokenUsageAccumulates(t *testing.T) {
	s := newTestStore(t)
	tenantID, userID := seedTenantUser(t, s.db)
	ctx := reqctx.WithUser(reqctx.WithTenant(context.Background(), tenantID), userID)

	day := "2026-06-16"
	if err := s.AddTokenUsage(ctx, day, 60); err != nil {
		t.Fatalf("AddTokenUsage #1: %v", err)
	}
	if err := s.AddTokenUsage(ctx, day, 60); err != nil {
		t.Fatalf("AddTokenUsage #2: %v", err)
	}
	total, err := s.GetTokenUsage(ctx, day)
	if err != nil {
		t.Fatalf("GetTokenUsage: %v", err)
	}
	if total != 120 {
		t.Fatalf("GetTokenUsage = %d, want 120", total)
	}
}

func TestStoreConversationCrossTenantNotFound(t *testing.T) {
	s := newTestStore(t)
	tenantA, userA := seedTenantUser(t, s.db)
	ctxA := reqctx.WithUser(reqctx.WithTenant(context.Background(), tenantA), userA)

	conv, err := s.CreateConversation(ctxA, "tenant A chat")
	if err != nil {
		t.Fatalf("CreateConversation: %v", err)
	}

	tenantB, userB := seedTenantUser(t, s.db)
	ctxB := reqctx.WithUser(reqctx.WithTenant(context.Background(), tenantB), userB)

	_, err = s.GetConversation(ctxB, conv.ID)
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("cross-tenant GetConversation: got %v, want sql.ErrNoRows", err)
	}
}

// newTestStore opens a temp DB, migrates it, and returns a Store over it.
func newTestStore(t *testing.T) *Store {
	t.Helper()
	conn, err := appdb.Open(filepath.Join(t.TempDir(), "agent.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	if err := appdb.Migrate(conn); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	return NewStore(conn)
}

// seedTenantUser inserts a tenant and a user so audit FK constraints hold.
func seedTenantUser(t *testing.T, conn *sql.DB) (tenantID, userID int64) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := conn.Exec(
		`INSERT INTO tenants (uuid, name, status, created_at, updated_at) VALUES (?, 'Acme', 'active', ?, ?)`,
		uuid.NewString(), now, now)
	if err != nil {
		t.Fatalf("seed tenant: %v", err)
	}
	tenantID, _ = res.LastInsertId()
	res, err = conn.Exec(
		`INSERT INTO users (uuid, tenant_id, email, password_hash, name, role, created_at, updated_at)
		 VALUES (?, ?, ?, 'x', 'Owner', 'owner', ?, ?)`,
		uuid.NewString(), tenantID, uuid.NewString()+"@acme.test", now, now)
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}
	userID, _ = res.LastInsertId()
	return tenantID, userID
}
