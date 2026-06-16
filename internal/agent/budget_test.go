package agent

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/dknathalage/tallyo/internal/agent/llm"
	appdb "github.com/dknathalage/tallyo/internal/db"
	"github.com/dknathalage/tallyo/internal/reqctx"
)

// fakeClock is a settable clock for deterministic tests.
type fakeClock struct {
	now time.Time
}

func (f *fakeClock) Now() time.Time { return f.now }

func (f *fakeClock) advance(d time.Duration) { f.now = f.now.Add(d) }

// newTestBudget opens a fresh temp DB, seeds a tenant+user, and returns a
// Budget with the given config + fakeClock, plus an authed context.
func newTestBudget(t *testing.T, cfg Config, clk *fakeClock) (*Budget, context.Context) {
	t.Helper()
	conn, err := appdb.Open(filepath.Join(t.TempDir(), "budget.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	if err := appdb.Migrate(conn); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	tenantID, userID := seedTenantUser(t, conn)
	ctx := reqctx.WithUser(reqctx.WithTenant(context.Background(), tenantID), userID)
	store := NewStore(conn)
	budget := NewBudget(store, cfg, clk)
	return budget, ctx
}

// TestBudgetExceeded verifies the daily cap: usage below budget → not exceeded;
// usage at or above budget → exceeded.
func TestBudgetExceeded(t *testing.T) {
	clk := &fakeClock{now: time.Date(2026, 6, 16, 12, 0, 0, 0, time.UTC)}
	cfg := Config{DailyTokenBudget: 100, RatePerMinute: 20}
	budget, ctx := newTestBudget(t, cfg, clk)

	// Add 60 tokens (input=30, output=30).
	u1 := llm.Usage{InputTokens: 30, OutputTokens: 30}
	if err := budget.Add(ctx, u1); err != nil {
		t.Fatalf("Add #1: %v", err)
	}
	exceeded, err := budget.Exceeded(ctx)
	if err != nil {
		t.Fatalf("Exceeded #1: %v", err)
	}
	if exceeded {
		t.Fatal("Exceeded after 60 tokens with budget=100; want false")
	}

	// Add another 60 tokens (total = 120 ≥ 100).
	u2 := llm.Usage{InputTokens: 30, OutputTokens: 30}
	if err := budget.Add(ctx, u2); err != nil {
		t.Fatalf("Add #2: %v", err)
	}
	exceeded, err = budget.Exceeded(ctx)
	if err != nil {
		t.Fatalf("Exceeded #2: %v", err)
	}
	if !exceeded {
		t.Fatal("Exceeded after 120 tokens with budget=100; want true")
	}
}

// TestBudgetPerDay verifies that usage on day A does not count against day B,
// and that the day-A total is still retrievable.
func TestBudgetPerDay(t *testing.T) {
	clk := &fakeClock{now: time.Date(2026, 6, 16, 23, 0, 0, 0, time.UTC)}
	cfg := Config{DailyTokenBudget: 50, RatePerMinute: 20}
	budget, ctx := newTestBudget(t, cfg, clk)

	// Day A: add 60 tokens → exceeded.
	dayA := clk.now.UTC().Format("2006-01-02")
	if err := budget.Add(ctx, llm.Usage{InputTokens: 60}); err != nil {
		t.Fatalf("Add day A: %v", err)
	}
	exceeded, err := budget.Exceeded(ctx)
	if err != nil {
		t.Fatalf("Exceeded day A: %v", err)
	}
	if !exceeded {
		t.Fatal("want exceeded on day A after 60 tokens with budget=50")
	}

	// Advance clock to day B (next day).
	clk.advance(2 * time.Hour) // 01:00 next day

	// Day B: fresh slate → not exceeded.
	exceeded, err = budget.Exceeded(ctx)
	if err != nil {
		t.Fatalf("Exceeded day B: %v", err)
	}
	if exceeded {
		t.Fatal("want NOT exceeded on day B (fresh day)")
	}

	// Day A total is still 60 in the store.
	total, err := budget.store.GetTokenUsage(ctx, dayA)
	if err != nil {
		t.Fatalf("GetTokenUsage day A: %v", err)
	}
	if total != 60 {
		t.Fatalf("day A total = %d, want 60", total)
	}
}

// TestRateLimitPerUser verifies the per-user sliding-window rate limit.
func TestRateLimitPerUser(t *testing.T) {
	clk := &fakeClock{now: time.Date(2026, 6, 16, 12, 0, 0, 0, time.UTC)}
	cfg := Config{RatePerMinute: 3, DailyTokenBudget: 2_000_000}
	budget, ctx := newTestBudget(t, cfg, clk)

	const user1 = int64(1)
	const user2 = int64(2)

	// user1: first 3 calls allowed within the same minute.
	for i := 1; i <= 3; i++ {
		if !budget.AllowMessage(ctx, user1) {
			t.Fatalf("call %d for user1: expected allowed", i)
		}
	}

	// user1: 4th call within the same minute is denied.
	if budget.AllowMessage(ctx, user1) {
		t.Fatal("4th call for user1 within same minute: expected denied")
	}

	// user2 is unaffected by user1's usage.
	if !budget.AllowMessage(ctx, user2) {
		t.Fatal("user2 first call: expected allowed (independent of user1)")
	}

	// Advance clock past the 60-second window; user1 is allowed again.
	clk.advance(61 * time.Second)
	if !budget.AllowMessage(ctx, user1) {
		t.Fatal("user1 after 61s: expected allowed (window has reset)")
	}

	// Invalid userID is always denied.
	if budget.AllowMessage(ctx, 0) {
		t.Fatal("userID=0: expected denied")
	}
	if budget.AllowMessage(ctx, -1) {
		t.Fatal("userID=-1: expected denied")
	}
}

// TestBudgetCacheTokensCounted verifies that cache tokens count toward the
// daily total (all four Usage fields are summed).
func TestBudgetCacheTokensCounted(t *testing.T) {
	clk := &fakeClock{now: time.Date(2026, 6, 16, 10, 0, 0, 0, time.UTC)}
	cfg := Config{DailyTokenBudget: 100, RatePerMinute: 20}
	budget, ctx := newTestBudget(t, cfg, clk)

	u := llm.Usage{
		InputTokens:      20,
		OutputTokens:     20,
		CacheReadTokens:  30,
		CacheWriteTokens: 31, // total = 101 ≥ 100
	}
	if err := budget.Add(ctx, u); err != nil {
		t.Fatalf("Add: %v", err)
	}
	exceeded, err := budget.Exceeded(ctx)
	if err != nil {
		t.Fatalf("Exceeded: %v", err)
	}
	if !exceeded {
		t.Fatal("want exceeded when cache tokens push total over budget")
	}
}
