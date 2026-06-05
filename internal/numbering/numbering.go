package numbering

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// Config names a document-number sequence. Table/Column come ONLY from
// predefined package configs (Invoice/Estimate), never from request input, so
// building the query from them is safe.
type Config struct {
	Table  string
	Column string
	Prefix string // e.g. "INV-"
	Pad    int    // zero-pad width, e.g. 4
}

// Predefined configs used by the invoice/estimate domains.
var (
	Invoice  = Config{Table: "invoices", Column: "invoice_number", Prefix: "INV-", Pad: 4}
	Estimate = Config{Table: "estimates", Column: "estimate_number", Prefix: "EST-", Pad: 4}
)

// Next computes the next number for cfg WITHIN the given tx. The caller must
// INSERT a row using the returned number in the SAME tx, and wrap the whole tx
// in WithRetry to survive concurrent races under WAL.
func Next(ctx context.Context, tx *sql.Tx, cfg Config) (string, error) {
	if tx == nil {
		return "", fmt.Errorf("numbering: nil tx")
	}
	q := fmt.Sprintf(
		`SELECT COALESCE(MAX(CAST(substr(%s, %d) AS INTEGER)), 0) FROM %s WHERE %s LIKE ?`,
		cfg.Column, len(cfg.Prefix)+1, cfg.Table, cfg.Column,
	)
	var max int
	if err := tx.QueryRowContext(ctx, q, cfg.Prefix+"%").Scan(&max); err != nil {
		return "", fmt.Errorf("numbering next: %w", err)
	}
	return fmt.Sprintf("%s%0*d", cfg.Prefix, cfg.Pad, max+1), nil
}

// WithRetry runs fn up to attempts times, retrying on the transient errors that
// occur when concurrent creators race under WAL: UNIQUE collisions AND
// busy/locked/snapshot conflicts (SQLITE_BUSY_SNAPSHOT=517, not covered by
// busy_timeout for a deferred-tx upgrade). Non-retryable errors return at once.
func WithRetry(ctx context.Context, attempts int, fn func() error) error {
	if attempts < 1 {
		attempts = 1
	}
	var err error
	for i := 0; i < attempts; i++ {
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
