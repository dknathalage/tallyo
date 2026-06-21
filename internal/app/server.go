package app

import (
	"io/fs"
	"net/http"

	"github.com/alexedwards/scs/v2"
	"github.com/dknathalage/tallyo/internal/agent"
	"github.com/dknathalage/tallyo/internal/auth"
	"github.com/dknathalage/tallyo/internal/businessprofile"
	"github.com/dknathalage/tallyo/internal/catalog"
	"github.com/dknathalage/tallyo/internal/customitem"
	"github.com/dknathalage/tallyo/internal/estimate"
	"github.com/dknathalage/tallyo/internal/httpx"
	"github.com/dknathalage/tallyo/internal/invoice"
	"github.com/dknathalage/tallyo/internal/participant"
	"github.com/dknathalage/tallyo/internal/planmanager"
	"github.com/dknathalage/tallyo/internal/realtime"
	"github.com/dknathalage/tallyo/internal/recurring"
	"github.com/dknathalage/tallyo/internal/shift"
	"github.com/dknathalage/tallyo/internal/taxrate"
	"github.com/go-chi/chi/v5"
)

// Deps holds the dependencies required to build the HTTP server. Every field is
// populated by the composition root (see app.go); Assets is the only one whose
// absence is fatal (NewServer panics). The handler fields self-register their
// routes under /api — public ones (Signup/Auth/Invites) outside the auth group,
// the rest inside it.
type Deps struct {
	Assets          fs.FS                    // embedded SPA build sub-FS (index/200.html, _app/...)
	Signup          *SignupHandler           // public self-serve tenant signup
	Session         *scs.SessionManager      // loads/saves the session per request
	Users           *auth.UsersRepo          // backs the auth-guard's user-exists recheck
	Tenants         *auth.TenantsRepo        // backs the auth-guard's suspended-tenant recheck
	Auth            *AuthHandler             // login/logout/me
	Invites         *InviteHandler           // invite create (owner-only) + public validate/accept
	Events          *realtime.EventsHandler  // SSE stream at GET /api/events
	BusinessProfile *businessprofile.Handler // singleton business profile
	PlanManagers    *planmanager.Handler     // plan-manager CRUD + bulk-delete
	TaxRates        *taxrate.Handler         // tax-rate CRUD
	Participants    *participant.Handler     // participant CRUD + bulk-delete
	CustomItems     *customitem.Handler      // per-tenant custom-item CRUD + bulk-delete
	SupportCatalog  *catalog.Handler         // read-only global NDIS catalogue (+ admin ingest)
	Invoices        *invoice.Handler         // invoice CRUD, status, bulk, per-participant stats
	Shifts          *shift.Handler           // shift lifecycle, billing suggestions, CRUD
	Estimates       *estimate.Handler        // estimate CRUD, status, duplicate, bulk, convert
	Payments        *invoice.PaymentHandler  // per-invoice payment list/create + delete
	Recurring       *recurring.Handler       // recurring-template CRUD + generate
	Smarts          *agent.SmartsHandler     // one-shot AI "Smart" routes (503 when AI disabled)
	Features        map[string]bool          // feature-gate state exposed at GET /api/features
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
		panic("app.NewServer: deps.Assets is required")
	}

	r := chi.NewRouter()
	r.Use(httpx.Recover)
	r.Use(httpx.RequestLogger)
	// LoadAndSave must wrap any route that reads or writes the session. It is
	// harmless on session-free routes (/healthz, SPA). Guarded so the static-only
	// NewServer construction used in unit tests works without a session manager.
	if deps.Session != nil {
		r.Use(deps.Session.LoadAndSave)
	}

	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		if _, err := w.Write([]byte("ok")); err != nil {
			return
		}
	})

	// /api subrouter, mounted before the SPA catch-all so it takes precedence.
	// The per-field nil guards let tests build NewServer with a subset of deps
	// (a handler's Routes() is called at registration time, so a nil one panics).
	r.Route("/api", func(api chi.Router) {
		if deps.Signup != nil {
			api.Post("/signup", deps.Signup.Signup)
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
		if deps.Session == nil {
			return // no authenticated routes without a session manager
		}
		// Tenant-AGNOSTIC authed routes: a valid session (email) is enough; no
		// tenant is resolved. Powers bootstrap + the tenant switcher.
		api.Group(func(pr chi.Router) {
			pr.Use(httpx.RequireSession(deps.Session))
			if deps.Auth != nil {
				pr.Get("/auth/session", deps.Auth.Session)
			}
		})
		// Tenant-SCOPED routes: the {tenantUUID} segment is authorized against
		// the session email by ResolveTenant, which attaches the per-tenant
		// tenant id + user + role to the context.
		api.Route("/t/{tenantUUID}", func(pr chi.Router) {
			pr.Use(httpx.RequireSession(deps.Session))
			pr.Use(httpx.ResolveTenant(deps.Users, deps.Tenants))
			if deps.Auth != nil {
				pr.Get("/auth/me", deps.Auth.Me)
			}
			if deps.Invites != nil {
				// User management is owner/admin only (spec §3.2).
				pr.With(httpx.RequireRole("owner", "admin")).Post("/invites", deps.Invites.Create)
			}
			if deps.Events != nil {
				pr.Get("/events", deps.Events.Stream)
			}
			if deps.BusinessProfile != nil {
				deps.BusinessProfile.Routes(pr)
			}
			if deps.PlanManagers != nil {
				deps.PlanManagers.Routes(pr)
			}
			if deps.TaxRates != nil {
				deps.TaxRates.Routes(pr)
			}
			if deps.Participants != nil {
				deps.Participants.Routes(pr)
			}
			if deps.CustomItems != nil {
				deps.CustomItems.Routes(pr)
			}
			// SupportCatalog is the GLOBAL NDIS catalogue. Reads are open to
			// any authenticated tenant user; the XLSX ingest (write) is gated
			// to platform admins (spec §5).
			if deps.SupportCatalog != nil {
				deps.SupportCatalog.Routes(pr)
			}
			if deps.Invoices != nil {
				deps.Invoices.Routes(pr)
			}
			if deps.Shifts != nil {
				deps.Shifts.Routes(pr)
			}
			if deps.Estimates != nil {
				deps.Estimates.Routes(pr)
			}
			if deps.Payments != nil {
				deps.Payments.Routes(pr)
			}
			if deps.Recurring != nil {
				deps.Recurring.Routes(pr)
			}
			if deps.Smarts != nil {
				pr.Post("/shifts/import", deps.Smarts.ImportShifts)
			}
			if deps.Features != nil {
				pr.Get("/features", func(w http.ResponseWriter, _ *http.Request) {
					httpx.WriteJSON(w, http.StatusOK, deps.Features)
				})
			}
		})
	})

	// SPA static handler must be registered last as the catch-all so that
	// /healthz and /api take precedence.
	r.Handle("/*", httpx.SPAHandler(deps.Assets))

	return &Server{Router: r}
}
