// Package numbering provides the single, tenant-scoped source of truth for
// document-number allocation (invoices, estimates) plus the retry wrapper that
// makes concurrent allocation safe under WAL.
//
// # One implementation
//
// Numbering is per tenant: uniqueness is (tenant_id, number) and sequences
// restart at 0001 for each tenant (spec §8). Allocation has two halves:
//
//  1. The tenant-scoped MAX query (sqlc-generated, e.g. MaxInvoiceNumberLike)
//     filters WHERE tenant_id = ? and returns the highest numeric suffix used.
//  2. Format turns (prefix, max) into the next padded number.
//
// Repositories call Format inside their create transaction and wrap the whole
// transaction in WithRetry, so a UNIQUE(tenant_id, number) collision from a
// concurrent creator is retried (re-reading MAX) until it succeeds. There is no
// other numbering path: the former schema-coupled Config/Next helper (which
// referenced columns that no longer exist) was removed.
package numbering

import (
	"context"
	"fmt"
	"strings"
)

// pad is the zero-pad width for every document number (e.g. INV-0001).
const pad = 4

// Format builds the next document number from a prefix and the current maximum
// numeric suffix in use for the tenant. max is the value returned by the
// tenant-scoped MAX query; the next number is max+1, zero-padded. Callers must
// run this inside a transaction wrapped by WithRetry so a concurrent collision
// on UNIQUE(tenant_id, number) re-reads max and retries.
func Format(prefix string, max int64) string {
	return fmt.Sprintf("%s%0*d", prefix, pad, max+1)
}

// WithRetry runs fn up to attempts times, retrying on the transient errors that
// occur when concurrent creators race under WAL: UNIQUE collisions AND
// busy/locked/snapshot conflicts (SQLITE_BUSY_SNAPSHOT=517, not covered by
// busy_timeout for a deferred-tx upgrade). Non-retryable errors return at once.
func WithRetry(ctx context.Context, attempts int, fn func() error) error {
	_ = ctx // reserved for future cancellation-aware retry; kept for call-site stability
	if attempts < 1 {
		attempts = 1
	}
	var err error
	for i := 0; i < attempts; i++ { // bounded by attempts
		err = fn()
		if err == nil {
			return nil
		}
		if !isRetryable(err) {
			return err
		}
	}
	return fmt.Errorf("numbering: exhausted %d attempts: %w", attempts, err)
}

func isRetryable(err error) bool {
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "unique") || strings.Contains(s, "constraint") ||
		strings.Contains(s, "locked") || strings.Contains(s, "busy")
}
