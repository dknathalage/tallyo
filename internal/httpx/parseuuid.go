package httpx

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// ParseUUID reads the named path param and validates it as a UUID.
// Returns the canonical lowercase string and true, or "" and false.
func ParseUUID(r *http.Request, name string) (string, bool) {
	raw := chi.URLParam(r, name)
	if raw == "" {
		return "", false
	}
	u, err := uuid.Parse(raw)
	if err != nil {
		return "", false
	}
	return u.String(), true
}
