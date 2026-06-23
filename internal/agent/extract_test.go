package agent

// Tests for timesheet text → structured shift extraction (ExtractShifts). The
// unit test scripts a forced emit_shifts tool_use via the fake llm client and
// asserts the four reference nursing-note days are parsed with correct dates and
// quantities, plus that invalid drafts are dropped. A gated live test parses the
// real message.txt against the Anthropic API.

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/dknathalage/tallyo/internal/agent/llm"
)

// emitShiftsResponse builds an llm.Response carrying a single forced emit_shifts
// tool_use whose input encodes the given drafts.
func emitShiftsResponse(t *testing.T, drafts []ShiftDraft) llm.Response {
	t.Helper()
	in, err := json.Marshal(map[string]any{"shifts": drafts})
	if err != nil {
		t.Fatalf("marshal emit_shifts input: %v", err)
	}
	return llm.Response{
		StopReason: llm.StopToolUse,
		Content: []llm.Block{{
			Type:      llm.BlockToolUse,
			ToolUseID: "toolu_extract_1",
			ToolName:  emitShiftsTool,
			Input:     json.RawMessage(in),
		}},
	}
}

// TestExtractShiftsParsesFourDays scripts the four reference days and asserts
// ExtractShifts returns them with the right dates, hours and km, forcing the
// emit_shifts tool on the request.
func TestExtractShiftsParsesFourDays(t *testing.T) {
	want := []ShiftDraft{
		{ClientName: "Tania", ServiceDate: "2026-06-09", Hours: 7.0, Km: 36, Note: "self care"},
		{ClientName: "Tania", ServiceDate: "2026-06-10", Hours: 5.5, Km: 12, Note: "self care"},
		{ClientName: "Tania", ServiceDate: "2026-06-11", Hours: 7.0, Km: 64, Note: "self care"},
		{ClientName: "Tania", ServiceDate: "2026-06-12", Hours: 5.5, Km: 38, Note: "self care"},
	}
	fake := llm.NewFake(emitShiftsResponse(t, want))

	got, err := ExtractShifts(context.Background(), fake, "model-x", "high", "some timesheet text")
	if err != nil {
		t.Fatalf("ExtractShifts: %v", err)
	}
	if len(got) != 4 {
		t.Fatalf("got %d shifts, want 4", len(got))
	}
	for i := range got { // bounded by len(got)
		if got[i].ServiceDate != want[i].ServiceDate {
			t.Errorf("shift %d date = %q, want %q", i, got[i].ServiceDate, want[i].ServiceDate)
		}
		if got[i].Hours != want[i].Hours {
			t.Errorf("shift %d hours = %v, want %v", i, got[i].Hours, want[i].Hours)
		}
		if got[i].Km != want[i].Km {
			t.Errorf("shift %d km = %v, want %v", i, got[i].Km, want[i].Km)
		}
	}

	// The request must force the emit_shifts tool over an untrusted-fenced message.
	if len(fake.Requests) != 1 {
		t.Fatalf("llm called %d times, want 1", len(fake.Requests))
	}
	req := fake.Requests[0]
	if req.ToolChoice.ForceTool != emitShiftsTool {
		t.Errorf("ForceTool = %q, want %q", req.ToolChoice.ForceTool, emitShiftsTool)
	}
	if len(req.Messages) != 1 || len(req.Messages[0].Content) != 1 {
		t.Fatalf("unexpected request message shape: %+v", req.Messages)
	}
}

// TestExtractShiftsDropsInvalid asserts a non-ISO date and a negative quantity
// are dropped, keeping only the valid drafts.
func TestExtractShiftsDropsInvalid(t *testing.T) {
	scripted := []ShiftDraft{
		{ClientName: "Tania", ServiceDate: "2026-06-09", Hours: 7.0, Km: 36},
		{ClientName: "Tania", ServiceDate: "09/06/2026", Hours: 5.5, Km: 12}, // non-ISO → dropped
		{ClientName: "Tania", ServiceDate: "2026-06-11", Hours: -1, Km: 64},  // negative → dropped
	}
	fake := llm.NewFake(emitShiftsResponse(t, scripted))

	got, err := ExtractShifts(context.Background(), fake, "model-x", "", "text")
	if err != nil {
		t.Fatalf("ExtractShifts: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d shifts, want 1 (two dropped)", len(got))
	}
	if got[0].ServiceDate != "2026-06-09" {
		t.Fatalf("kept shift date = %q, want 2026-06-09", got[0].ServiceDate)
	}
}

// TestExtractShiftsNoneIsError asserts that when no valid shift survives, an
// error is returned (an empty extraction is a failure, not a silent success).
func TestExtractShiftsNoneIsError(t *testing.T) {
	fake := llm.NewFake(emitShiftsResponse(t, []ShiftDraft{
		{ClientName: "Tania", ServiceDate: "not-a-date"},
	}))
	if _, err := ExtractShifts(context.Background(), fake, "m", "", "text"); err == nil {
		t.Fatal("expected an error when no valid shift is extracted")
	}
}

// TestExtractShiftsEmptyText rejects blank input before any model call.
func TestExtractShiftsEmptyText(t *testing.T) {
	fake := llm.NewFake()
	if _, err := ExtractShifts(context.Background(), fake, "m", "", "   "); err == nil {
		t.Fatal("expected an error for empty text")
	}
	if fake.Calls() != 0 {
		t.Fatalf("blank input must not call the model; calls = %d", fake.Calls())
	}
}

// TestExtractShiftsLive parses the real timesheet message (message.txt at the
// repo root) against the Anthropic API and asserts the four reference days come
// back with the right dates and quantities. Gated like the live agent test: runs
// only when RUN_LIVE_AGENT is set AND a key is available; otherwise skipped.
func TestExtractShiftsLive(t *testing.T) {
	if os.Getenv("RUN_LIVE_AGENT") == "" {
		t.Skip("set RUN_LIVE_AGENT=1 to run the live extraction test")
	}
	env := liveEnv(t)
	apiKey := env["ANTHROPIC_API_KEY"]
	if apiKey == "" {
		t.Skip("ANTHROPIC_API_KEY not set (env or repo-root .env); skipping live extraction test")
	}

	text := readRepoFile(t, filepath.Join("test_data", "message.txt"))
	if text == "" {
		text = readRepoFile(t, "message.txt") // fallback to repo root
	}
	if text == "" {
		t.Skip("message.txt not found (test_data/ or repo root); skipping live extraction test")
	}

	cfg := Config{APIKey: apiKey, Model: env["ANTHROPIC_MODEL"], Effort: env["ANTHROPIC_EFFORT"]}.WithDefaults()
	client := llm.NewAnthropic(cfg.APIKey, cfg.Model, cfg.EffortFor())

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	got, err := ExtractShifts(ctx, client, cfg.Model, cfg.EffortFor(), text)
	if err != nil {
		t.Fatalf("ExtractShifts (live): %v", err)
	}
	if len(got) != 4 {
		t.Fatalf("got %d shifts, want 4 (Tania, 9–12 Jun)", len(got))
	}

	// Expected per-day hours/km from the reference timesheet.
	wantKm := map[string]float64{"2026-06-09": 36, "2026-06-10": 12, "2026-06-11": 64, "2026-06-12": 38}
	wantHr := map[string]float64{"2026-06-09": 7.0, "2026-06-10": 5.5, "2026-06-11": 7.0, "2026-06-12": 5.5}
	for i := range got { // bounded by len(got)
		d := got[i]
		t.Logf("shift: %s hours=%.1f km=%.1f client=%q", d.ServiceDate, d.Hours, d.Km, d.ClientName)
		if wk, ok := wantKm[d.ServiceDate]; !ok || d.Km != wk {
			t.Errorf("%s km = %.1f, want %.1f", d.ServiceDate, d.Km, wk)
		}
		if wh, ok := wantHr[d.ServiceDate]; !ok || d.Hours != wh {
			t.Errorf("%s hours = %.1f, want %.1f", d.ServiceDate, d.Hours, wh)
		}
	}
}

// readRepoFile reads a file from the repo root (walking up from the test's
// working directory like loadDotenv). Returns "" when not found.
func readRepoFile(t *testing.T, name string) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		return ""
	}
	for i := 0; i < 6; i++ { // bounded climb to repo root
		candidate := filepath.Join(dir, name)
		if b, readErr := os.ReadFile(candidate); readErr == nil {
			return string(b)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return ""
}
