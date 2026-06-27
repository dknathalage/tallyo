// Package numbering provides the single, tenant-scoped source of truth for
// document-number allocation (invoices, estimates) plus the retry wrapper that
// makes concurrent allocation safe on Postgres.
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
	"errors"
	"fmt"
	"math/rand"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
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
// occur when concurrent creators race: a UNIQUE collision (SQLSTATE 23505) from
// two creators picking the same next number, or a serialization failure
// (SQLSTATE 40001). Non-retryable errors return at once.
//
// Between retries it sleeps a short randomized backoff. Under Postgres' default
// READ COMMITTED isolation, contending creators all read the same MAX and pick
// the same next number; without jitter they would retry in lockstep and keep
// colliding. The jitter spreads them out so allocation converges. The sleep is
// bounded and honours ctx cancellation.
func WithRetry(ctx context.Context, attempts int, fn func() error) error {
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
		// Backoff before the next attempt (not after the last). Base grows with
		// the attempt index, plus jitter, capped low — numbering conflicts clear
		// in microseconds once creators desynchronize.
		if i < attempts-1 {
			d := time.Duration(i+1)*time.Millisecond + time.Duration(rand.Intn(2000))*time.Microsecond
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(d):
			}
		}
	}
	return fmt.Errorf("numbering: exhausted %d attempts: %w", attempts, err)
}

// isRetryable reports whether err is a transient Postgres conflict worth
// retrying: 23505 unique_violation or 40001 serialization_failure.
func isRetryable(err error) bool {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return false
	}
	return pgErr.Code == "23505" || pgErr.Code == "40001"
}
