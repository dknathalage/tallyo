// Package httpapi holds the HTTP layer for the Tallyo web service: JSON
// read/write helpers, an embedded SPA static handler, and the chi server
// scaffold. Later tasks extend Deps and register /api routes.
package httpapi

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
)

// maxRequestBody caps decoded request bodies to guard against unbounded input.
const maxRequestBody = 1 << 20 // 1 MiB

// WriteJSON serializes v as JSON with the given status code. An encode error
// cannot change the already-written status, so it is logged.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	if w == nil {
		slog.Error("httpapi.WriteJSON: nil ResponseWriter")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("httpapi.WriteJSON: encode failed", slog.Any("error", err))
	}
}

// WriteError writes a JSON error envelope {"error": msg} with the given status.
func WriteError(w http.ResponseWriter, status int, msg string) {
	WriteJSON(w, status, map[string]string{"error": msg})
}

// DecodeJSON reads the request body into dst, capping size and rejecting
// unknown fields. Both r and dst must be non-nil.
func DecodeJSON(r *http.Request, dst any) error {
	if r == nil || r.Body == nil {
		return fmt.Errorf("httpapi.DecodeJSON: nil request or body")
	}
	if dst == nil {
		return fmt.Errorf("httpapi.DecodeJSON: nil destination")
	}
	dec := json.NewDecoder(io.LimitReader(r.Body, maxRequestBody))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return fmt.Errorf("httpapi.DecodeJSON: %w", err)
	}
	return nil
}
