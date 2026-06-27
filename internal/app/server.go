package app

import (
	"io/fs"
	"net/http"

	"github.com/dknathalage/tallyo/internal/auth"
	"github.com/dknathalage/tallyo/internal/businessprofile"
	"github.com/dknathalage/tallyo/internal/catalogue"
	"github.com/dknathalage/tallyo/internal/client"
	"github.com/dknathalage/tallyo/internal/estimate"
	"github.com/dknathalage/tallyo/internal/httpx"
	"github.com/dknathalage/tallyo/internal/invoice"
	"github.com/dknathalage/tallyo/internal/payer"
	"github.com/dknathalage/tallyo/internal/session"
	"github.com/dknathalage/tallyo/internal/smarts"
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
	Verifier        auth.TokenVerifier       // verifies Firebase bearer tokens (RequireAuth)
	AuthConfig      *AuthConfigHandler       // public GET /api/auth/config
	Signup          *SignupHandler           // Bearer-authed self-serve tenant signup
	Users           *auth.UsersRepo          // backs ResolveTenant's membership lookup
	Tenants         *auth.TenantsRepo        // backs the auth-guard's suspended-tenant recheck
	Auth            *AuthHandler             // session/me
	Invites         *InviteHandler           // invite create (owner-only) + public validate/accept
	BusinessProfile *businessprofile.Handler // singleton business profile
	Payers          *payer.Handler           // payer CRUD + bulk-delete
	TaxRates        *taxrate.Handler         // tax-rate CRUD
	Clients         *client.Handler          // client CRUD + bulk-delete
	Catalogue       *catalogue.Handler       // per-tenant catalogue CRUD + bulk-delete + owner/admin import
	Invoices        *invoice.Handler         // invoice CRUD, status, bulk, per-client stats
	Sessions        *session.Handler         // session lifecycle, billing suggestions, CRUD
	Estimates       *estimate.Handler        // estimate CRUD, status, duplicate, bulk, convert
	Payments        *invoice.PaymentHandler  // per-invoice payment list/create + delete
	Smarts          *smarts.Handler          // AI "Smarts" routes (503 when AI disabled)
	Features        map[string]bool          // feature-gate state exposed at GET /api/features
	BillingEnabled  bool                     // when true, ResolveTenant computes entitlement and write routes are gated
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

	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		if _, err := w.Write([]byte("ok")); err != nil {
			return
		}
	})

	// /api subrouter, mounted before the SPA catch-all so it takes precedence.
	// The per-field nil guards let tests build NewServer with a subset of deps
	// (a handler's Routes() is called at registration time, so a nil one panics).
	r.Route("/api", func(api chi.Router) {
		// Public config: the SPA fetches this on boot to init Firebase and decide
		// which sign-in buttons to render. No auth.
		if deps.AuthConfig != nil {
			api.Get("/auth/config", deps.AuthConfig.Config)
		}
		// Public invite VALIDATION: the invitee may not have an account yet.
		// Acceptance is Bearer-authed (it needs the uid), so it lives in the
		// authed group below.
		if deps.Invites != nil {
			api.Get("/invites/{token}", deps.Invites.Validate)
		}
		if deps.Verifier == nil {
			return // no authenticated routes without a token verifier
		}
		// Tenant-AGNOSTIC authed routes: a valid bearer token (uid) is enough; no
		// tenant is resolved. Powers bootstrap, signup, invite-accept and the
		// tenant switcher.
		api.Group(func(pr chi.Router) {
			pr.Use(httpx.RequireAuth(deps.Verifier))
			if deps.Signup != nil {
				pr.Post("/signup", deps.Signup.Signup)
			}
			if deps.Auth != nil {
				pr.Get("/auth/session", deps.Auth.Session)
			}
			if deps.Invites != nil {
				pr.Post("/invites/{token}/accept", deps.Invites.Accept)
			}
		})
		// Tenant-SCOPED routes: the {tenantUUID} segment is authorized against
		// the verified uid by ResolveTenant, which attaches the per-tenant
		// tenant id + user + role to the context.
		api.Route("/t/{tenantUUID}", func(pr chi.Router) {
			pr.Use(httpx.RequireAuth(deps.Verifier))
			pr.Use(httpx.ResolveTenant(deps.Users, deps.Tenants, deps.BillingEnabled))
			if deps.Auth != nil {
				pr.Get("/auth/me", deps.Auth.Me)
			}
			if deps.Invites != nil && deps.Features["invites"] {
				// User management is owner/admin only (spec §3.2).
				pr.With(httpx.RequireRole("owner", "admin")).Post("/invites", deps.Invites.Create)
				pr.With(httpx.RequireRole("owner", "admin")).Delete("/invites/{inviteUUID}", deps.Invites.Revoke)
			}
			if deps.BusinessProfile != nil {
				deps.BusinessProfile.Routes(pr)
			}
			if deps.Payers != nil {
				deps.Payers.Routes(pr)
			}
			if deps.TaxRates != nil {
				deps.TaxRates.Routes(pr)
			}
			if deps.Clients != nil {
				deps.Clients.Routes(pr)
			}
			// Catalogue is the per-tenant catalogue. Reads + CRUD are open to any
			// authenticated tenant user; the upload-and-map import (write) is gated
			// to owner/admin within the handler's Routes.
			if deps.Catalogue != nil {
				deps.Catalogue.Routes(pr)
			}
			if deps.Invoices != nil {
				deps.Invoices.Routes(pr)
			}
			if deps.Sessions != nil {
				deps.Sessions.Routes(pr)
			}
			if deps.Estimates != nil {
				deps.Estimates.Routes(pr)
			}
			if deps.Payments != nil {
				deps.Payments.Routes(pr)
			}
			if deps.Smarts != nil {
				deps.Smarts.Routes(pr)
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
