package httpapi

import (
	"net/http"

	"github.com/dknathalage/tallyo/internal/httpx"
)

// Compatibility shims: implementations live in internal/httpx. Per-domain
// handlers drop these and call httpx.* directly when they move to slices.
// NOTE: WriteValidationError stays defined in respond.go (it depends on service).
var (
	WriteJSON            = httpx.WriteJSON
	WriteError           = httpx.WriteError
	DecodeJSON           = httpx.DecodeJSON
	Recover              = httpx.Recover
	RequestLogger        = httpx.RequestLogger
	RequireAuth          = httpx.RequireAuth
	RequireRole          = httpx.RequireRole
	RequirePlatformAdmin = httpx.RequirePlatformAdmin
	UserFrom             = httpx.UserFrom
	WithLogger           = httpx.WithLogger
	EnrichLogger         = httpx.EnrichLogger
	LoggerFrom           = httpx.LoggerFrom
	SPAHandler           = httpx.SPAHandler
)

func parseID(r *http.Request) (int64, bool) { return httpx.ParseID(r) }
