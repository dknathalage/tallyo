// Package httpx holds domain-agnostic HTTP helpers extracted from
// internal/http so they can be shared across slices without import cycles.
package httpx

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"github.com/dknathalage/tallyo/internal/apperr"
)

// maxRequestBody caps decoded request bodies to guard against unbounded input.
const maxRequestBody = 1 << 20 // 1 MiB

// WriteJSON serializes v as JSON with the given status code. An encode error
// cannot change the already-written status, so it is logged.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	if w == nil {
		slog.Error("httpx.WriteJSON: nil ResponseWriter")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("httpx.WriteJSON: encode failed", slog.Any("error", err))
	}
}

// WriteError writes a JSON error envelope {"error": msg} with the given status.
func WriteError(w http.ResponseWriter, status int, msg string) {
	WriteJSON(w, status, map[string]string{"error": msg})
}

// WriteValidationError writes a 422 with per-field detail when err carries
// validation failures (it satisfies apperr.Validation, e.g. *billing.ValidationError),
// returning true. When err is not a validation error it writes nothing and
// returns false, letting the caller fall through to its generic handling.
func WriteValidationError(w http.ResponseWriter, err error) bool {
	var v apperr.Validation
	if errors.As(err, &v) {
		WriteJSON(w, http.StatusUnprocessableEntity, map[string]any{
			"error":   "validation failed",
			"details": v.ValidationDetails(),
		})
		return true
	}
	return false
}

// WriteServiceError maps a service-layer error to its HTTP response and reports
// whether it wrote anything. A nil error writes nothing and returns false (the
// happy path falls through); every non-nil error is written and returns true:
//
//	apperr.ErrNotFound      -> 404 "not found"
//	apperr.ErrConflict      -> 409 "conflict"
//	apperr.Validation       -> 422 with per-field details
//	anything else           -> 500 "internal error"
//
// Handlers reduce their error handling to one line:
// `if httpx.WriteServiceError(w, err) { return }`.
func WriteServiceError(w http.ResponseWriter, err error) bool {
	switch {
	case err == nil:
		return false
	case errors.Is(err, apperr.ErrNotFound):
		WriteError(w, http.StatusNotFound, "not found")
	case errors.Is(err, apperr.ErrConflict):
		WriteError(w, http.StatusConflict, "conflict")
	case WriteValidationError(w, err):
		// already written
	default:
		WriteError(w, http.StatusInternalServerError, "internal error")
	}
	return true
}

// DecodeJSON reads the request body into dst, capping size and rejecting
// unknown fields. Both r and dst must be non-nil.
func DecodeJSON(r *http.Request, dst any) error {
	if r == nil || r.Body == nil {
		return fmt.Errorf("httpx.DecodeJSON: nil request or body")
	}
	if dst == nil {
		return fmt.Errorf("httpx.DecodeJSON: nil destination")
	}
	dec := json.NewDecoder(io.LimitReader(r.Body, maxRequestBody))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return fmt.Errorf("httpx.DecodeJSON: %w", err)
	}
	return nil
}
