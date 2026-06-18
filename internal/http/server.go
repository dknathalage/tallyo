package httpapi

import (
	"io/fs"
	"net/http"

	"github.com/alexedwards/scs/v2"
	"github.com/dknathalage/tallyo/internal/auth"
	"github.com/dknathalage/tallyo/internal/businessprofile"
	"github.com/dknathalage/tallyo/internal/catalog"
	"github.com/dknathalage/tallyo/internal/customitem"
	"github.com/dknathalage/tallyo/internal/estimate"
	"github.com/dknathalage/tallyo/internal/invoice"
	"github.com/dknathalage/tallyo/internal/participant"
	"github.com/dknathalage/tallyo/internal/planmanager"
	"github.com/dknathalage/tallyo/internal/shift"
	"github.com/dknathalage/tallyo/internal/taxrate"
	"github.com/go-chi/chi/v5"
)

// Deps holds the dependencies required to build the HTTP server. It is
// intentionally minimal for now; later tasks add more services and the realtime
// hub. Assets is the embedded SPA build sub-FS and is required.
type Deps struct {
	// Assets is the file system serving the built SPA (index/200.html, _app/...).
	Assets fs.FS

	// Signup, when non-nil, serves the public self-serve tenant signup route.
	Signup *SignupHandler

	// Session, when non-nil, wraps the router so sessions load and save per
	// request. Required for the authenticated routes below to function.
	Session *scs.SessionManager

	// Users backs the auth-guard's user-exists recheck. Required when Auth is set.
	Users *auth.UsersRepo

	// Tenants backs the auth-guard's suspended-tenant recheck. Required when
	// any authenticated route is registered.
	Tenants *auth.TenantsRepo

	// Auth, when non-nil, serves login/logout/me under /api.
	Auth *AuthHandler

	// Invites, when non-nil, serves invite creation (owner-only) plus public
	// invite validation and acceptance under /api.
	Invites *InviteHandler

	// Events, when non-nil, serves the auth-gated SSE stream at GET /api/events.
	Events *EventsHandler

	// BusinessProfile, when non-nil, serves the auth-gated GET/PUT singleton
	// business profile at /api/business-profile.
	BusinessProfile *businessprofile.Handler

	// PlanManagers, when non-nil, serves the auth-gated plan-manager CRUD plus
	// bulk-delete routes under /api/plan-managers.
	PlanManagers *planmanager.Handler

	// TaxRates, when non-nil, serves the auth-gated tax-rate CRUD routes under
	// /api/tax-rates.
	TaxRates *taxrate.Handler

	// Participants, when non-nil, serves the auth-gated participant CRUD plus
	// bulk-delete routes under /api/participants.
	Participants *participant.Handler

	// CustomItems, when non-nil, serves the auth-gated per-tenant custom-item
	// CRUD plus bulk-delete routes under /api/custom-items.
	CustomItems *customitem.Handler

	// SupportCatalog, when non-nil, serves the auth-gated read-only GLOBAL NDIS
	// Support Catalogue routes under /api/support-catalog.
	SupportCatalog *catalog.Handler

	// Invoices, when non-nil, serves the auth-gated invoice CRUD, status,
	// bulk routes, plus the per-participant stats route under /api.
	Invoices *invoice.Handler

	// Shifts, when non-nil, serves the auth-gated shift lifecycle routes: the
	// per-participant shift list under /api/participants/{id}/shifts, the
	// tenant-wide list, billing suggestions and to-record prompts, plus shift
	// CRUD and the status-transition route under /api/shifts.
	Shifts *shift.Handler

	// Estimates, when non-nil, serves the auth-gated estimate CRUD, status,
	// duplicate, bulk, and convert-to-invoice routes under /api/estimates.
	Estimates *estimate.Handler

	// Payments, when non-nil, serves the auth-gated per-invoice payment list
	// and create routes plus payment deletion under /api.
	Payments *invoice.PaymentHandler

	// Recurring, when non-nil, serves the auth-gated recurring-template CRUD
	// plus the generate route under /api/recurring.
	Recurring *RecurringHandler

	// Export, when non-nil, serves the auth-gated CSV/Excel export routes under
	// /api/export.
	Export *ExportHandler

	// Agent, when non-nil, serves the auth-gated AI agent routes under
	// /api/agent: conversation create/list, message history, async message
	// send, permission decisions, checkpoint revert, and the per-conversation
	// SSE stream. Every route 503s when the agent is disabled.
	Agent *AgentHandler
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
		// Authenticated /api group. Only registered when there is at least one
		// protected route, since RequireAuth requires non-nil Session and Users.
		if deps.Auth != nil || deps.Invites != nil || deps.Events != nil || deps.BusinessProfile != nil || deps.PlanManagers != nil || deps.TaxRates != nil || deps.Participants != nil || deps.CustomItems != nil || deps.SupportCatalog != nil || deps.Invoices != nil || deps.Shifts != nil || deps.Estimates != nil || deps.Payments != nil || deps.Recurring != nil || deps.Export != nil || deps.Agent != nil {
			api.Group(func(pr chi.Router) {
				pr.Use(RequireAuth(deps.Session, deps.Users, deps.Tenants))
				if deps.Auth != nil {
					pr.Get("/auth/me", deps.Auth.Me)
				}
				if deps.Invites != nil {
					// User management is owner/admin only (spec §3.2).
					pr.With(RequireRole("owner", "admin")).Post("/invites", deps.Invites.Create)
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
					pr.Get("/recurring", deps.Recurring.List)
					pr.Post("/recurring", deps.Recurring.Create)
					pr.Get("/recurring/{id}", deps.Recurring.Get)
					pr.Put("/recurring/{id}", deps.Recurring.Update)
					pr.Delete("/recurring/{id}", deps.Recurring.Delete)
					pr.Post("/recurring/{id}/generate", deps.Recurring.Generate)
				}
				if deps.Export != nil {
					pr.Get("/export/catalog", deps.Export.Catalog)
					pr.Get("/export/invoices", deps.Export.Invoices)
					pr.Get("/export/estimates", deps.Export.Estimates)
				}
				if deps.Agent != nil {
					pr.Post("/agent/conversations", deps.Agent.CreateConversation)
					pr.Get("/agent/conversations", deps.Agent.ListConversations)
					pr.Get("/agent/conversations/{id}/messages", deps.Agent.ListMessages)
					pr.Post("/agent/conversations/{id}/messages", deps.Agent.SendMessage)
					pr.Get("/agent/conversations/{id}/stream", deps.Agent.Stream)
					pr.Post("/participants/{id}/draft-invoice", deps.Agent.DraftInvoiceFromShifts)
					pr.Post("/shifts/import", deps.Agent.ImportShifts)
					pr.Post("/agent/steps/{id}/decision", deps.Agent.Decide)
					pr.Post("/agent/checkpoints/{id}/revert", deps.Agent.Revert)
				}
			})
		}
	})

	// SPA static handler must be registered last as the catch-all so that
	// /healthz and /api take precedence.
	r.Handle("/*", SPAHandler(deps.Assets))

	return &Server{Router: r}
}
