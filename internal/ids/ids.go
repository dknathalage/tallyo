// Package ids mints the application's single identifier type: a time-ordered
// UUIDv7 string. Every row id and every generated id goes through New() — there
// is exactly one id convention in the codebase.
package ids

import "github.com/google/uuid"

// New returns a fresh UUIDv7 as a 36-char string. UUIDv7 is time-ordered, so
// ids sort chronologically (preserving the old ORDER BY id behaviour).
func New() string {
	return uuid.Must(uuid.NewV7()).String()
}
