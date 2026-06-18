// Package httpapi holds the HTTP layer for the Tallyo web service: JSON
// read/write helpers, an embedded SPA static handler, and the chi server
// scaffold. Later tasks extend Deps and register /api routes.
package httpapi

import (
	"net/http"

	"github.com/dknathalage/tallyo/internal/httpx"
	"github.com/dknathalage/tallyo/internal/service"
)

// WriteValidationError, when err is (or wraps) a *service.ValidationError, writes
// a 422 envelope {"error": "...", "details": [{line, field, message}, ...]} and
// returns true. Otherwise it writes nothing and returns false, so callers can
// fall through to their generic error handling. J12 reads "details" to surface
// per-line, per-field messages inline in the invoice/estimate editor.
func WriteValidationError(w http.ResponseWriter, err error) bool {
	if w == nil {
		return false
	}
	ve, ok := service.AsValidationError(err)
	if !ok || ve == nil {
		return false
	}
	httpx.WriteJSON(w, http.StatusUnprocessableEntity, map[string]any{
		"error":   "validation failed",
		"details": ve.Errors,
	})
	return true
}
