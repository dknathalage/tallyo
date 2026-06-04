package httpapi

import (
	"io/fs"
	"net/http"

	"github.com/go-chi/chi/v5"
)

// Deps holds the dependencies required to build the HTTP server. It is
// intentionally minimal for now; later tasks add services, the session
// manager, and the realtime hub. Assets is the embedded SPA build sub-FS and
// is required.
type Deps struct {
	// Assets is the file system serving the built SPA (index/200.html, _app/...).
	Assets fs.FS
}

// Server wraps the configured chi router.
type Server struct {
	Router chi.Router
}

// NewServer builds the HTTP server: a /healthz probe, a place for future /api
// routes, and the SPA static handler mounted last as the catch-all. Panics if
// deps.Assets is nil since the server cannot serve the UI without it.
func NewServer(deps Deps) *Server {
	if deps.Assets == nil {
		panic("httpapi.NewServer: deps.Assets is required")
	}

	r := chi.NewRouter()

	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		if _, err := w.Write([]byte("ok")); err != nil {
			return
		}
	})

	// TODO(later tasks): mount the /api subrouter here, before the SPA
	// catch-all, e.g. r.Mount("/api", apiRouter(deps)).

	// SPA static handler must be registered last as the catch-all so that
	// /healthz and /api take precedence.
	r.Handle("/*", SPAHandler(deps.Assets))

	return &Server{Router: r}
}
