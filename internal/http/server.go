package httpapi

import (
	"io/fs"
	"net/http"

	"github.com/alexedwards/scs/v2"
	"github.com/dknathalage/tallyo/internal/auth"
	"github.com/go-chi/chi/v5"
)

// Deps holds the dependencies required to build the HTTP server. It is
// intentionally minimal for now; later tasks add more services and the realtime
// hub. Assets is the embedded SPA build sub-FS and is required.
type Deps struct {
	// Assets is the file system serving the built SPA (index/200.html, _app/...).
	Assets fs.FS

	// Setup, when non-nil, serves the first-run setup routes under /api.
	Setup *SetupHandler

	// Session, when non-nil, wraps the router so sessions load and save per
	// request. Required for the authenticated routes below to function.
	Session *scs.SessionManager

	// Users backs the auth-guard's user-exists recheck. Required when Auth is set.
	Users *auth.UsersRepo

	// Auth, when non-nil, serves login/logout/me under /api.
	Auth *AuthHandler

	// Invites, when non-nil, serves invite creation (owner-only) plus public
	// invite validation and acceptance under /api.
	Invites *InviteHandler
}

// Server wraps the configured chi router.
type Server struct {
	Router chi.Router
}

// NewServer builds the HTTP server: a /healthz probe, /api routes (setup, auth),
// and the SPA static handler mounted last as the catch-all. Panics if
// deps.Assets is nil since the server cannot serve the UI without it.
func NewServer(deps Deps) *Server {
	if deps.Assets == nil {
		panic("httpapi.NewServer: deps.Assets is required")
	}

	r := chi.NewRouter()
	r.Use(Recover)
	r.Use(RequestLogger)
	// LoadAndSave must wrap any route that reads or writes the session. It is
	// harmless on session-free routes (/healthz, SPA).
	if deps.Session != nil {
		r.Use(deps.Session.LoadAndSave)
	}

	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		if _, err := w.Write([]byte("ok")); err != nil {
			return
		}
	})

	// /api subrouter, mounted before the SPA catch-all so it takes precedence.
	r.Route("/api", func(api chi.Router) {
		if deps.Setup != nil {
			api.Get("/setup/status", deps.Setup.Status)
			api.Post("/setup", deps.Setup.CreateOwner)
		}
		if deps.Auth != nil {
			api.Post("/auth/login", deps.Auth.Login)
			api.Post("/auth/logout", deps.Auth.Logout)
		}
		// Public invite routes: the invitee is not logged in, so Validate and
		// Accept must sit outside the RequireAuth group.
		if deps.Invites != nil {
			api.Get("/invites/{token}", deps.Invites.Validate)
			api.Post("/invites/{token}/accept", deps.Invites.Accept)
		}
		// Authenticated /api group. Only registered when there is at least one
		// protected route, since RequireAuth requires non-nil Session and Users.
		if deps.Auth != nil || deps.Invites != nil {
			api.Group(func(pr chi.Router) {
				pr.Use(RequireAuth(deps.Session, deps.Users))
				if deps.Auth != nil {
					pr.Get("/auth/me", deps.Auth.Me)
				}
				if deps.Invites != nil {
					pr.Post("/invites", deps.Invites.Create)
				}
			})
		}
	})

	// SPA static handler must be registered last as the catch-all so that
	// /healthz and /api take precedence.
	r.Handle("/*", SPAHandler(deps.Assets))

	return &Server{Router: r}
}
