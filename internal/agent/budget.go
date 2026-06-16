package agent

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/dknathalage/tallyo/internal/agent/llm"
)

// rateLimiter is a per-user sliding-window rate limiter. It tracks the
// timestamps of recent messages per user and rejects calls that would exceed
// cfg.RatePerMinute within any rolling 60-second window.
type rateLimiter struct {
	mu        sync.Mutex
	clk       clock
	rateLimit int
	// timestamps maps userID → ordered slice of call times (oldest first).
	timestamps map[int64][]time.Time
}

func newRateLimiter(clk clock, ratePerMinute int) *rateLimiter {
	if ratePerMinute <= 0 {
		ratePerMinute = 1
	}
	return &rateLimiter{
		clk:        clk,
		rateLimit:  ratePerMinute,
		timestamps: make(map[int64][]time.Time),
	}
}

// allow returns true and records the call if the user has not exceeded the
// rate limit; returns false (without recording) when the limit would be
// exceeded.
func (r *rateLimiter) allow(userID int64) bool {
	if userID <= 0 {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	now := r.clk.Now()
	cutoff := now.Add(-time.Minute)

	// Prune timestamps older than the rolling window.
	ts := r.timestamps[userID]
	kept := ts[:0]
	for i := range ts { // bounded by len(ts)
		if ts[i].After(cutoff) {
			kept = append(kept, ts[i])
		}
	}

	if len(kept) >= r.rateLimit {
		r.timestamps[userID] = kept
		return false
	}
	r.timestamps[userID] = append(kept, now)
	return true
}

// Budget enforces per-tenant daily token caps and per-user message rate limits.
//
// Token accounting:
//
//	Total = InputTokens + OutputTokens + CacheReadTokens + CacheWriteTokens.
//
// All four counters are included because cache reads and writes still represent
// real compute cost billed against the tenant's usage.
type Budget struct {
	store *Store
	cfg   Config
	clock clock
	rl    *rateLimiter
}

// NewBudget constructs a Budget. A nil store or clock is a programmer error.
func NewBudget(store *Store, cfg Config, clk clock) *Budget {
	if store == nil {
		panic("agent: NewBudget requires a non-nil Store")
	}
	if clk == nil {
		panic("agent: NewBudget requires a non-nil clock")
	}
	return &Budget{
		store: store,
		cfg:   cfg.WithDefaults(),
		clock: clk,
		rl:    newRateLimiter(clk, cfg.WithDefaults().RatePerMinute),
	}
}

// NewBudgetWallClock constructs a Budget backed by the real wall clock. It is
// the production entry point (main.go), keeping the unexported clock interface
// internal while letting callers outside the package build a Budget.
func NewBudgetWallClock(store *Store, cfg Config) *Budget {
	return NewBudget(store, cfg, wallClock{})
}

// today returns the current UTC date in YYYY-MM-DD form for the budget key.
func (b *Budget) today() string {
	return b.clock.Now().UTC().Format("2006-01-02")
}

// Add accumulates the token spend for this turn into the per-tenant daily
// usage row. Total = Input + Output + CacheRead + CacheWrite.
func (b *Budget) Add(ctx context.Context, u llm.Usage) error {
	if ctx == nil {
		return fmt.Errorf("budget.Add: nil context")
	}
	total := u.InputTokens + u.OutputTokens + u.CacheReadTokens + u.CacheWriteTokens
	if total <= 0 {
		return nil // nothing to record
	}
	day := b.today()
	if err := b.store.AddTokenUsage(ctx, day, total); err != nil {
		return fmt.Errorf("budget.Add: %w", err)
	}
	return nil
}

// Exceeded reports whether the tenant has consumed at least DailyTokenBudget
// tokens today. Returns false when DailyTokenBudget is zero (no cap).
func (b *Budget) Exceeded(ctx context.Context) (bool, error) {
	if ctx == nil {
		return false, fmt.Errorf("budget.Exceeded: nil context")
	}
	if b.cfg.DailyTokenBudget <= 0 {
		return false, nil // no cap configured
	}
	total, err := b.store.GetTokenUsage(ctx, b.today())
	if err != nil {
		return false, fmt.Errorf("budget.Exceeded: %w", err)
	}
	return total >= b.cfg.DailyTokenBudget, nil
}

// AllowMessage returns true when the user has not exceeded the per-minute
// message rate limit. The per-user sliding window is tracked in memory (reset
// on process restart). This method is intended for the HTTP layer (Task 12);
// it is NOT called inside Execute.
//
// Returns false for invalid userIDs (≤ 0) or when the rate limit is reached.
func (b *Budget) AllowMessage(ctx context.Context, userID int64) bool {
	if ctx == nil {
		return false
	}
	if userID <= 0 {
		return false
	}
	return b.rl.allow(userID)
}
