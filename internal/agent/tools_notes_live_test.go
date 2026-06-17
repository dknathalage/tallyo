package agent

// Live, real-API acceptance test for the notes → invoice AI workflow. It drives
// the ACTUAL agent loop against the Anthropic Messages API — plan, then
// list_participant_notes → search_catalogue (auto-run reads) → create_invoice
// (risky, suspended then approved via Decide) — and asserts the model
// reproduces the reference invoice (Invoice-2638907B.pdf): 8 GST-free lines,
// total $1905.76, billed from the four nursing-note days.
//
// It runs only when explicitly opted in via RUN_LIVE_AGENT (non-empty) AND an
// Anthropic key is available (ANTHROPIC_API_KEY in the environment, or in the
// repo-root .env); otherwise it is skipped, so the normal `go test ./...` gate
// stays hermetic and offline. To run it:
//
//	RUN_LIVE_AGENT=1 go test ./internal/agent/ -run TestAgentDraftsInvoiceFromNotesLive -v
//
// It makes real model calls and consumes tokens.

import (
	"bufio"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dknathalage/tallyo/internal/agent/llm"
	"github.com/dknathalage/tallyo/internal/realtime"
	"github.com/dknathalage/tallyo/internal/reqctx"
	"github.com/dknathalage/tallyo/internal/service"
)

func TestAgentDraftsInvoiceFromNotesLive(t *testing.T) {
	if os.Getenv("RUN_LIVE_AGENT") == "" {
		t.Skip("set RUN_LIVE_AGENT=1 to run the live agent test")
	}
	env := liveEnv(t)
	apiKey := env["ANTHROPIC_API_KEY"]
	if apiKey == "" {
		t.Skip("ANTHROPIC_API_KEY not set (env or repo-root .env); skipping live AI test")
	}

	// Seed the same fixture the deterministic chain test uses: tenant, participant
	// Tania with a plan window, a catalogue version with the two reference codes,
	// and the four nursing-note days stored as journal notes.
	conn, tenantID, participantID := noteToolsFixture(t)
	ctx := reqctx.WithTenant(context.Background(), tenantID)
	notes := service.NewNoteService(conn, realtime.NewHub())
	seedReferenceNotes(t, notes, ctx, participantID)

	// Wire the REAL agent: store, the full read+write tool set the production
	// server registers, a checkpoint, events, and a live Anthropic client.
	cfg := Config{
		APIKey: apiKey,
		Model:  env["ANTHROPIC_MODEL"],
		Effort: env["ANTHROPIC_EFFORT"],
	}.WithDefaults()
	cfg.MaxIterations = 30
	cfg.SkipPlan = true // measure the efficient path: no forced plan turn

	store := NewStore(conn)
	cp := NewCheckpoint(store, conn)
	invoiceSvc := service.NewInvoiceService(conn, realtime.NewHub())
	reg := NewRegistry()
	reg.Register(NewListInvoicesTool(invoiceSvc))
	reg.Register(NewCreateInvoiceToolVerified(invoiceSvc, notes, cp))
	reg.Register(NewListParticipantNotesToolWithCatalog(notes, service.NewSupportCatalogService(conn)))
	reg.Register(NewSearchCatalogueTool(service.NewSupportCatalogService(conn)))

	client := llm.NewAnthropic(cfg.APIKey, cfg.Model, cfg.EffortFor())
	ag := NewAgent(cfg, client, store, reg, cp, NewEvents()).
		WithRestore(InvoiceRestoreFunc(invoiceSvc))

	// Bound the whole turn so a hung API call fails the test rather than hangs.
	runCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	conv, err := store.CreateConversation(runCtx, "Live notes→invoice")
	if err != nil {
		t.Fatalf("CreateConversation: %v", err)
	}

	// The agent has no participant-lookup tool, so the participant id is given
	// explicitly; everything else (codes, rates, quantities) the model must work
	// out from the notes and the catalogue.
	prompt := strings.Join([]string{
		"Draft an NDIS invoice for participant id ", itoa(participantID),
		" (Tania Hangevelled) for all supports delivered between 2026-06-09 and 2026-06-12,",
		" based on their journal notes. Read the notes, then for each activity look up the",
		" correct NDIS support item code and price for that service date from the catalogue,",
		" and create the invoice for my approval.",
	}, "")

	t.Logf("model=%s effort=%s", cfg.Model, cfg.EffortFor())
	if err := ag.Start(runCtx, conv.ID, prompt); err != nil {
		t.Fatalf("agent Start: %v", err)
	}

	// Approve every risky step the model proposes (it should propose exactly one
	// create_invoice). Decide resumes the loop synchronously; loop until no more
	// awaiting steps remain. Bounded so a misbehaving model can't spin forever.
	const farFuture = "2999-01-01T00:00:00Z"
	approvals := 0
	for iter := 0; iter < 6; iter++ { // bounded
		steps, sErr := store.ListExpiredAwaitingSteps(runCtx, farFuture)
		if sErr != nil {
			t.Fatalf("list awaiting steps: %v", sErr)
		}
		if len(steps) == 0 {
			break
		}
		if dErr := ag.Decide(runCtx, steps[0].ID, true); dErr != nil {
			t.Fatalf("approve step %d (%s): %v", steps[0].ID, steps[0].ToolName, dErr)
		}
		approvals++
	}
	if approvals == 0 {
		t.Fatalf("the model never proposed a create_invoice write to approve")
	}

	// The AI should have created exactly one invoice matching the reference PDF.
	invoices, err := invoiceSvc.List(ctx)
	if err != nil {
		t.Fatalf("list invoices: %v", err)
	}
	if len(invoices) != 1 {
		t.Fatalf("invoices created = %d, want 1", len(invoices))
	}
	inv, err := invoiceSvc.Get(ctx, invoices[0].ID)
	if err != nil {
		t.Fatalf("get invoice: %v", err)
	}

	t.Logf("AI invoice: %d line(s), subtotal %.2f, tax %.2f, total %.2f",
		len(inv.LineItems), inv.Subtotal, inv.Tax, inv.Total)
	for i := range inv.LineItems { // bounded by len(inv.LineItems)
		li := inv.LineItems[i]
		t.Logf("  line: %s %s qty=%.2f unit=%.2f total=%.2f gstFree=%v",
			li.ServiceDate, li.Code, li.Quantity, li.UnitPrice, li.LineTotal, li.GstFree)
	}

	// Accuracy assertions (Errorf, not Fatalf, so the full invoice is reported on
	// any mismatch): 8 GST-free lines, total $1905.76 — matching the PDF.
	if len(inv.LineItems) != 8 {
		t.Errorf("line count = %d, want 8 (transport + self-care per day × 4 days)", len(inv.LineItems))
	}
	if inv.Tax != 0 {
		t.Errorf("tax = %.2f, want 0 (NDIS supports are GST-free)", inv.Tax)
	}
	if inv.Total != 1905.76 {
		t.Errorf("total = %.2f, want 1905.76 (Invoice-2638907B.pdf)", inv.Total)
	}
}

// liveEnv returns the Anthropic config values, preferring the process
// environment and falling back to the repo-root .env file (searched upward from
// the test's working directory). Missing keys map to "".
func liveEnv(t *testing.T) map[string]string {
	t.Helper()
	keys := []string{"ANTHROPIC_API_KEY", "ANTHROPIC_MODEL", "ANTHROPIC_EFFORT"}
	out := make(map[string]string, len(keys))
	for i := range keys { // bounded by len(keys)
		out[keys[i]] = os.Getenv(keys[i])
	}
	if out["ANTHROPIC_API_KEY"] != "" {
		return out
	}
	dotenv := loadDotenv(t)
	for i := range keys { // bounded by len(keys)
		if out[keys[i]] == "" {
			out[keys[i]] = dotenv[keys[i]]
		}
	}
	return out
}

// loadDotenv parses the nearest .env walking up from the working directory (the
// package dir under `go test`). It returns an empty map when none is found; a
// parse is best-effort (KEY=VALUE lines, # comments, optional surrounding
// quotes). It never fails the test — a missing .env just yields a skip upstream.
func loadDotenv(t *testing.T) map[string]string {
	t.Helper()
	out := map[string]string{}
	dir, err := os.Getwd()
	if err != nil {
		return out
	}
	var path string
	for i := 0; i < 6; i++ { // bounded climb to repo root
		candidate := filepath.Join(dir, ".env")
		if _, statErr := os.Stat(candidate); statErr == nil {
			path = candidate
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	if path == "" {
		return out
	}
	f, err := os.Open(path)
	if err != nil {
		return out
	}
	defer func() { _ = f.Close() }()
	sc := bufio.NewScanner(f)
	for sc.Scan() { // bounded by file length
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		v = strings.Trim(strings.TrimSpace(v), `"'`)
		out[k] = v
	}
	return out
}

// itoa renders a positive int64 without importing strconv into the test's hot
// path; bounded by the number of decimal digits.
func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 { // bounded by digit count (≤19)
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
