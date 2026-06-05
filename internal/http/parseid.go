package httpapi

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
)

// parseID reads the {id} path param as int64.
func parseID(r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		return 0, false
	}
	return id, true
}
