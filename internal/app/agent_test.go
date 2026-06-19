package app

import (
	"bufio"
	"encoding/json"
	"github.com/dknathalage/tallyo/internal/httpx"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/dknathalage/tallyo/internal/agent"
	"github.com/dknathalage/tallyo/internal/agent/llm"
	"github.com/dknathalage/tallyo/internal/auth"
	"github.com/dknathalage/tallyo/internal/invoice"
	"github.com/dknathalage/tallyo/internal/realtime"
	"github.com/dknathalage/tallyo/internal/shift"
	"github.com/go-chi/chi/v5"
)

// newAgentServer wires the agent routes behind httpx.RequireAuth over a single
// migrated DB seeded with an owner ("o@x.com" / "password1"). The supplied Fake
// llm scripts the plan/execute loop. enabled toggles the 503 gate; cfg overrides
// the agent config (zero value → defaults). Returns the server and the agent
// store (for direct assertions) plus the shared conn.
func newAgentServer(t *testing.T, fake llm.Client, enabled bool, cfg agent.Config) (*httptest.Server, *agent.Store) {
	t.Helper()
	conn := openMigratedDB(t, "agent.db")
	users, _, _ := seedTenantOwner(t, conn)

	sm := auth.NewSessionManager(conn, false)
	authH := NewAuthHandler(sm, users, auth.NewTenants(conn))

	hub := realtime.NewHub()
	store := agent.NewStore(conn)
	inv := invoice.NewService(conn, hub, shift.NewShifts(conn))
	cp := agent.NewCheckpoint(store, conn)

	reg := agent.NewRegistry()
	reg.Register(agent.NewListInvoicesTool(inv))
	reg.Register(agent.NewCreateInvoiceTool(inv, cp))

	events := agent.NewEvents()
	cfg.APIKey = "test"
	cfg = cfg.WithDefaults()
	budget := agent.NewBudget(store, cfg, clockNow{})
	ag := agent.NewAgent(cfg, fake, store, reg, cp, events).
		WithBudget(budget).
		WithRestore(agent.InvoiceRestoreFunc(inv))

	agentH := agent.NewAgentHandler(ag, budget, enabled)

	router := chi.NewRouter()
	router.Route("/api", func(api chi.Router) {
		api.Post("/auth/login", authH.Login)
		api.Group(func(pr chi.Router) {
			pr.Use(httpx.RequireAuth(sm, users, auth.NewTenants(conn)))
			pr.Post("/agent/conversations", agentH.CreateConversation)
			pr.Get("/agent/conversations", agentH.ListConversations)
			pr.Get("/agent/conversations/{id}/messages", agentH.ListMessages)
			pr.Post("/agent/conversations/{id}/messages", agentH.SendMessage)
			pr.Get("/agent/conversations/{id}/stream", agentH.Stream)
			pr.Post("/agent/steps/{id}/decision", agentH.Decide)
			pr.Post("/agent/checkpoints/{id}/revert", agentH.Revert)
		})
	})

	srv := httptest.NewServer(sm.LoadAndSave(router))
	t.Cleanup(srv.Close)
	return srv, store
}

// clockNow is a real-time clock for the budget in tests (rate limiter uses it).
type clockNow struct{}

func (clockNow) Now() time.Time { return time.Now() }

// createConv posts a conversation and returns its id.
func createConv(t *testing.T, c *http.Client, base, title string) int64 {
	t.Helper()
	resp := postJSON(t, c, base+"/api/agent/conversations", `{"title":"`+title+`"}`)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create conversation: want 201 got %d", resp.StatusCode)
	}
	var out struct {
		ID    int64  `json:"id"`
		Title string `json:"title"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode conversation: %v", err)
	}
	if out.ID <= 0 {
		t.Fatalf("create conversation: bad id %d", out.ID)
	}
	return out.ID
}

func TestAgentCreateConversation(t *testing.T) {
	srv, _ := newAgentServer(t, llm.NewFake(), true, agent.Config{})
	c := loggedInClient(t, srv.URL)
	id := createConv(t, c, srv.URL, "Hello")
	if id <= 0 {
		t.Fatalf("expected positive conversation id, got %d", id)
	}
}

func TestAgentCreateConversationUnauthed401(t *testing.T) {
	srv, _ := newAgentServer(t, llm.NewFake(), true, agent.Config{})
	c := jarClient(t) // no login
	resp := postJSON(t, c, srv.URL+"/api/agent/conversations", `{"title":"x"}`)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthed create: want 401 got %d", resp.StatusCode)
	}
}

func TestAgentDisabled503(t *testing.T) {
	srv, _ := newAgentServer(t, llm.NewFake(), false, agent.Config{})
	c := loggedInClient(t, srv.URL)
	resp := postJSON(t, c, srv.URL+"/api/agent/conversations", `{"title":"x"}`)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("disabled handler: want 503 got %d", resp.StatusCode)
	}
}

func TestAgentListConversationsTenantScoping(t *testing.T) {
	// Tenant A and B share the same DB but different sessions. A conversation
	// created by A must not appear in B's list.
	conn := openMigratedDB(t, "agent.db")

	// Seed tenant A (the standard owner) and a second tenant B.
	usersA, _, _ := seedTenantOwner(t, conn)
	tenants := auth.NewTenants(conn)
	hash, err := auth.HashPassword("password1")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	tnB, err := tenants.Create(t.Context(), "Beta")
	if err != nil {
		t.Fatalf("create tenant B: %v", err)
	}
	if _, err := usersA.Create(t.Context(), tnB.ID, "b@x.com", hash, "", "owner", true); err != nil {
		t.Fatalf("create owner B: %v", err)
	}

	sm := auth.NewSessionManager(conn, false)
	authH := NewAuthHandler(sm, usersA, auth.NewTenants(conn))
	hub := realtime.NewHub()
	store := agent.NewStore(conn)
	inv := invoice.NewService(conn, hub, shift.NewShifts(conn))
	cp := agent.NewCheckpoint(store, conn)
	reg := agent.NewRegistry()
	reg.Register(agent.NewListInvoicesTool(inv))
	events := agent.NewEvents()
	cfg := agent.Config{APIKey: "test"}.WithDefaults()
	budget := agent.NewBudget(store, cfg, clockNow{})
	ag := agent.NewAgent(cfg, llm.NewFake(), store, reg, cp, events).WithBudget(budget)
	agentH := agent.NewAgentHandler(ag, budget, true)

	router := chi.NewRouter()
	router.Route("/api", func(api chi.Router) {
		api.Post("/auth/login", authH.Login)
		api.Group(func(pr chi.Router) {
			pr.Use(httpx.RequireAuth(sm, usersA, auth.NewTenants(conn)))
			pr.Post("/agent/conversations", agentH.CreateConversation)
			pr.Get("/agent/conversations", agentH.ListConversations)
		})
	})
	srv := httptest.NewServer(sm.LoadAndSave(router))
	t.Cleanup(srv.Close)

	// A creates a conversation.
	ca := jarClient(t)
	respA := login(t, ca, srv.URL, "o@x.com", "password1")
	_ = respA.Body.Close()
	convID := createConv(t, ca, srv.URL, "A only")

	// B logs in and lists: must not see A's conversation.
	cb := jarClient(t)
	respB := login(t, cb, srv.URL, "b@x.com", "password1")
	_ = respB.Body.Close()
	resp := get(t, cb, srv.URL+"/api/agent/conversations")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list B: want 200 got %d", resp.StatusCode)
	}
	var convs []struct {
		ID int64 `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&convs); err != nil {
		t.Fatalf("decode B list: %v", err)
	}
	for _, cv := range convs {
		if cv.ID == convID {
			t.Fatalf("tenant B saw tenant A's conversation %d", convID)
		}
	}
}

// TestAgentSendMessageAsyncRunsLoop drives the real async path: a scripted
// plan→read→end_turn Fake. SendMessage returns 202; the test subscribes to the
// SSE stream and waits for the message_final event, then confirms the final
// assistant message is persisted in the history.
func TestAgentSendMessageAsyncRunsLoop(t *testing.T) {
	fake := llm.NewFake(
		// 1: forced plan
		llm.Response{StopReason: llm.StopToolUse, Content: []llm.Block{{
			Type: llm.BlockToolUse, ToolUseID: "tu_plan", ToolName: "propose_plan",
			Input: json.RawMessage(`{"steps":[{"tool":"list_invoices","summary":"list","risk":"read"}]}`),
		}}},
		// 2: execute turn 1 — read tool
		llm.Response{StopReason: llm.StopToolUse, Content: []llm.Block{{
			Type: llm.BlockToolUse, ToolUseID: "tu_list", ToolName: "list_invoices",
			Input: json.RawMessage(`{}`),
		}}},
		// 3: execute turn 2 — end turn
		llm.Response{StopReason: llm.StopEndTurn, Content: []llm.Block{{
			Type: llm.BlockText, Text: "here are your invoices",
		}}},
	)
	srv, _ := newAgentServer(t, fake, true, agent.Config{})
	c := loggedInClient(t, srv.URL)
	convID := createConv(t, c, srv.URL, "Run")

	// Open the SSE stream BEFORE sending so we don't miss the message_final.
	streamURL := srv.URL + "/api/agent/conversations/" + strconv.FormatInt(convID, 10) + "/stream"
	sreq, _ := http.NewRequest("GET", streamURL, nil)
	sresp, err := c.Do(sreq)
	if err != nil {
		t.Fatalf("open stream: %v", err)
	}
	defer func() { _ = sresp.Body.Close() }()
	if sresp.StatusCode != http.StatusOK {
		t.Fatalf("stream: want 200 got %d", sresp.StatusCode)
	}
	// Give the handler a moment to subscribe before we trigger the loop.
	time.Sleep(50 * time.Millisecond)

	// Send a message → 202.
	resp := postJSON(t, c, srv.URL+"/api/agent/conversations/"+strconv.FormatInt(convID, 10)+"/messages", `{"text":"list my invoices"}`)
	if resp.StatusCode != http.StatusAccepted {
		_ = resp.Body.Close()
		t.Fatalf("send message: want 202 got %d", resp.StatusCode)
	}
	_ = resp.Body.Close()

	// Drain the SSE stream until we see message_final.
	if !waitForFrame(t, sresp.Body, `"message_final"`, 3*time.Second) {
		t.Fatalf("did not receive message_final SSE frame")
	}

	// Poll the message history until the final assistant text is persisted.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if historyHasText(t, c, srv.URL, convID, "here are your invoices") {
			return // success
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("final assistant message not found in history")
}

// TestAgentSendMessageRateLimited429 exhausts the per-user rate limit (1/min)
// then confirms the second send returns 429.
func TestAgentSendMessageRateLimited429(t *testing.T) {
	// A Fake that immediately ends the turn keeps each accepted run trivial.
	fake := llm.NewFake(
		llm.Response{StopReason: llm.StopToolUse, Content: []llm.Block{{
			Type: llm.BlockToolUse, ToolUseID: "tu_plan", ToolName: "propose_plan",
			Input: json.RawMessage(`{"steps":[]}`),
		}}},
		llm.Response{StopReason: llm.StopEndTurn, Content: []llm.Block{{Type: llm.BlockText, Text: "done"}}},
	)
	srv, _ := newAgentServer(t, fake, true, agent.Config{RatePerMinute: 1})
	c := loggedInClient(t, srv.URL)
	convID := createConv(t, c, srv.URL, "RL")

	url := srv.URL + "/api/agent/conversations/" + strconv.FormatInt(convID, 10) + "/messages"
	first := postJSON(t, c, url, `{"text":"one"}`)
	_ = first.Body.Close()
	if first.StatusCode != http.StatusAccepted {
		t.Fatalf("first send: want 202 got %d", first.StatusCode)
	}
	second := postJSON(t, c, url, `{"text":"two"}`)
	defer func() { _ = second.Body.Close() }()
	if second.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("second send (rate limited): want 429 got %d", second.StatusCode)
	}
}

func TestAgentDecideValidation(t *testing.T) {
	srv, _ := newAgentServer(t, llm.NewFake(), true, agent.Config{})
	c := loggedInClient(t, srv.URL)
	// Invalid decision value → 400 (step lookup not reached for a bad body).
	resp := postJSON(t, c, srv.URL+"/api/agent/steps/1/decision", `{"decision":"maybe"}`)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad decision: want 400 got %d", resp.StatusCode)
	}
}

// --- small helpers ---------------------------------------------------------

// waitForFrame reads SSE lines until one contains needle or the timeout elapses.
func waitForFrame(t *testing.T, body interface{ Read([]byte) (int, error) }, needle string, timeout time.Duration) bool {
	t.Helper()
	reader := bufio.NewReader(body)
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		line, err := reader.ReadString('\n')
		if strings.Contains(line, needle) {
			return true
		}
		if err != nil {
			return false
		}
	}
	return false
}

func historyHasText(t *testing.T, c *http.Client, base string, convID int64, text string) bool {
	t.Helper()
	resp := get(t, c, base+"/api/agent/conversations/"+strconv.FormatInt(convID, 10)+"/messages")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list messages: want 200 got %d", resp.StatusCode)
	}
	var msgs []struct {
		Role    string `json:"role"`
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&msgs); err != nil {
		t.Fatalf("decode messages: %v", err)
	}
	for _, m := range msgs {
		for _, b := range m.Content {
			if b.Text == text {
				return true
			}
		}
	}
	return false
}
