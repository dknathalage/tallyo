package numbering

import (
	"context"
	"errors"
	"testing"
)

// TestWithRetrySucceedsFirstAttempt verifies fn that returns nil immediately is
// run exactly once and reports success.
func TestWithRetrySucceedsFirstAttempt(t *testing.T) {
	calls := 0
	err := WithRetry(context.Background(), 5, func() error {
		calls++
		return nil
	})
	if err != nil {
		t.Fatalf("WithRetry: err = %v, want nil", err)
	}
	if calls != 1 {
		t.Fatalf("WithRetry: calls = %d, want 1", calls)
	}
}

// TestWithRetryStopsOnNonRetryable verifies a non-retryable error returns at
// once without consuming further attempts.
func TestWithRetryStopsOnNonRetryable(t *testing.T) {
	want := errors.New("fatal: bad input")
	calls := 0
	err := WithRetry(context.Background(), 5, func() error {
		calls++
		return want
	})
	if !errors.Is(err, want) {
		t.Fatalf("WithRetry: err = %v, want %v", err, want)
	}
	if calls != 1 {
		t.Fatalf("WithRetry: calls = %d, want 1 (non-retryable must not retry)", calls)
	}
}

// TestWithRetryRetriesThenSucceeds verifies a retryable error is retried and a
// later success is reported, exercising the loop-continue branch.
func TestWithRetryRetriesThenSucceeds(t *testing.T) {
	calls := 0
	err := WithRetry(context.Background(), 5, func() error {
		calls++
		if calls < 3 {
			return errors.New("UNIQUE constraint failed: doc_test.number")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("WithRetry: err = %v, want nil", err)
	}
	if calls != 3 {
		t.Fatalf("WithRetry: calls = %d, want 3", calls)
	}
}

// TestWithRetryExhausts verifies that a persistently retryable error exhausts
// all attempts and the wrapped error preserves the underlying cause.
func TestWithRetryExhausts(t *testing.T) {
	underlying := errors.New("database is locked")
	calls := 0
	err := WithRetry(context.Background(), 3, func() error {
		calls++
		return underlying
	})
	if err == nil {
		t.Fatal("WithRetry: err = nil, want exhaustion error")
	}
	if !errors.Is(err, underlying) {
		t.Fatalf("WithRetry: err = %v, want wrap of %v", err, underlying)
	}
	if calls != 3 {
		t.Fatalf("WithRetry: calls = %d, want 3 (one per attempt)", calls)
	}
}

// TestWithRetryClampsAttempts verifies attempts < 1 is clamped to a single
// attempt rather than skipping fn entirely.
func TestWithRetryClampsAttempts(t *testing.T) {
	cases := []struct {
		name     string
		attempts int
	}{
		{"zero", 0},
		{"negative", -7},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			err := WithRetry(context.Background(), tc.attempts, func() error {
				calls++
				return nil
			})
			if err != nil {
				t.Fatalf("WithRetry: err = %v, want nil", err)
			}
			if calls != 1 {
				t.Fatalf("WithRetry: calls = %d, want 1 (clamped)", calls)
			}
		})
	}
}

// TestIsRetryable is a table-driven check of the transient-error classifier:
// UNIQUE/constraint/locked/busy substrings (case-insensitive) are retryable,
// everything else is fatal.
func TestIsRetryable(t *testing.T) {
	cases := []struct {
		name string
		err  string
		want bool
	}{
		{"unique lower", "unique constraint failed", true},
		{"unique upper", "UNIQUE constraint failed: x.number", true},
		{"constraint only", "FOREIGN KEY constraint failed", true},
		{"locked", "database is LOCKED", true},
		{"busy", "database is busy (SQLITE_BUSY)", true},
		{"busy snapshot", "Busy snapshot 517", true},
		{"not found", "no rows in result set", false},
		{"syntax", "near \"SELCT\": syntax error", false},
		{"empty", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := isRetryable(errors.New(tc.err))
			if got != tc.want {
				t.Fatalf("isRetryable(%q) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}
