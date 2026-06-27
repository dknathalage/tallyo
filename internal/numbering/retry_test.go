package numbering

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

// pgErr builds a *pgconn.PgError carrying the given SQLSTATE, matching what the
// pgx driver returns for a constraint/serialization conflict.
func pgErr(code string) error {
	return &pgconn.PgError{Code: code}
}

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
			return pgErr("23505") // unique_violation
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
	underlying := pgErr("40001") // serialization_failure
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
// pgconn SQLSTATEs 23505 (unique_violation) and 40001 (serialization_failure)
// are retryable; any other SQLSTATE or a non-pg error is fatal.
func TestIsRetryable(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"unique violation", pgErr("23505"), true},
		{"serialization failure", pgErr("40001"), true},
		{"foreign key violation", pgErr("23503"), false},
		{"not null violation", pgErr("23502"), false},
		{"syntax error", pgErr("42601"), false},
		{"non-pg error", errors.New("no rows in result set"), false},
		{"nil-ish plain", errors.New(""), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := isRetryable(tc.err)
			if got != tc.want {
				t.Fatalf("isRetryable(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}
