// Package apperr holds the small set of cross-slice outcome errors that every
// service returns and the HTTP layer maps to a status code. It depends on
// nothing but the standard library, so both the generic httpx layer and every
// domain slice can import it without coupling httpx to a domain package (which
// is why these do not live in httpx or billing).
package apperr

import "errors"

// Sentinel outcomes a service returns and httpx.WriteServiceError maps:
//
//	ErrNotFound -> 404   a read/update/delete targeted a row that does not exist
//	ErrConflict -> 409   the request conflicts with current state
//
// Slices wrap these (fmt.Errorf("...: %w", apperr.ErrNotFound)) or return them
// directly; handlers match with errors.Is via WriteServiceError.
var (
	ErrNotFound = errors.New("not found")
	ErrConflict = errors.New("conflict")
)

// Validation is implemented by errors that carry field-level detail to surface
// as HTTP 422. billing.ValidationError satisfies it structurally (no import of
// this package required), keeping httpx free of any billing dependency.
type Validation interface {
	error
	// ValidationDetails returns the structured per-field failures to embed in
	// the 422 response body (e.g. []billing.FieldError).
	ValidationDetails() any
}

// FieldError is one structured, field-level validation failure, mirroring
// billing.FieldError's JSON shape so the 422 body is identical regardless of
// which type produced it. Line is the zero-based offending line (0 for a
// document-level field); Field names the field; Message is the human reason.
type FieldError struct {
	Line    int    `json:"line"`
	Field   string `json:"field"`
	Message string `json:"message"`
}

// ValidationError aggregates FieldErrors for the cheap required-field checks the
// simple CRUD slices run in their Input.Validate(). It satisfies Validation, so
// httpx.WriteServiceError maps it to a 422 with per-field detail.
//
// Slices that already import billing use billing.ValidationError; the simple
// slices (taxrate, client) cannot import billing without a package-test import
// cycle (billing's own tests import taxrate and client), so they use this
// equivalent. Both render the identical response body.
type ValidationError struct {
	Errors []FieldError `json:"errors"`
}

// ValidationDetails returns the per-field failures, satisfying Validation.
func (e *ValidationError) ValidationDetails() any { return e.Errors }

// Error renders the aggregated failures as one string.
func (e *ValidationError) Error() string {
	if e == nil || len(e.Errors) == 0 {
		return "validation failed"
	}
	return "validation failed"
}
