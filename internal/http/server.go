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
	BusinessProfile *BusinessProfileHandler

	// PlanManagers, when non-nil, serves the auth-gated plan-manager CRUD plus
	// bulk-delete routes under /api/plan-managers.
	PlanManagers *PlanManagerHandler

	// TaxRates, when non-nil, serves the auth-gated tax-rate CRUD routes under
	// /api/tax-rates.
	TaxRates *TaxRateHandler

	// Participants, when non-nil, serves the auth-gated participant CRUD plus
	// bulk-delete routes under /api/participants.
	Participants *ParticipantHandler

	// CustomItems, when non-nil, serves the auth-gated per-tenant custom-item
	// CRUD plus bulk-delete routes under /api/custom-items.
	CustomItems *CustomItemHandler

	// SupportCatalog, when non-nil, serves the auth-gated read-only GLOBAL NDIS
	// Support Catalogue routes under /api/support-catalog.
	SupportCatalog *SupportCatalogHandler

	// Invoices, when non-nil, serves the auth-gated invoice CRUD, status,
	// bulk routes, plus the per-participant stats route under /api.
	Invoices *InvoiceHandler

	// Notes, when non-nil, serves the auth-gated per-participant journal notes:
	// list under /api/participants/{id}/notes, plus note CRUD and the bill-link
	// route under /api/notes.
	Notes *NoteHandler

	// Shifts, when non-nil, serves the auth-gated shift lifecycle routes: the
	// per-participant shift list under /api/participants/{id}/shifts, the
	// tenant-wide list, billing suggestions and to-record prompts, plus shift
	// CRUD and the status-transition route under /api/shifts.
	Shifts *ShiftHandler

	// Estimates, when non-nil, serves the auth-gated estimate CRUD, status,
	// duplicate, bulk, and convert-to-invoice routes under /api/estimates.
	Estimates *EstimateHandler

	// Payments, when non-nil, serves the auth-gated per-invoice payment list
	// and create routes plus payment deletion under /api.
	Payments *PaymentHandler

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
		if deps.Auth != nil || deps.Invites != nil || deps.Events != nil || deps.BusinessProfile != nil || deps.PlanManagers != nil || deps.TaxRates != nil || deps.Participants != nil || deps.CustomItems != nil || deps.SupportCatalog != nil || deps.Invoices != nil || deps.Notes != nil || deps.Shifts != nil || deps.Estimates != nil || deps.Payments != nil || deps.Recurring != nil || deps.Export != nil || deps.Agent != nil {
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
					// Business settings: all roles may read; owner/admin may edit.
					pr.Get("/business-profile", deps.BusinessProfile.Get)
					pr.With(RequireRole("owner", "admin")).Put("/business-profile", deps.BusinessProfile.Put)
				}
				if deps.PlanManagers != nil {
					pr.Get("/plan-managers", deps.PlanManagers.List)
					pr.Post("/plan-managers", deps.PlanManagers.Create)
					pr.Post("/plan-managers/bulk-delete", deps.PlanManagers.BulkDelete)
					pr.Get("/plan-managers/{id}", deps.PlanManagers.Get)
					pr.Put("/plan-managers/{id}", deps.PlanManagers.Update)
					pr.Delete("/plan-managers/{id}", deps.PlanManagers.Delete)
				}
				if deps.TaxRates != nil {
					pr.Get("/tax-rates", deps.TaxRates.List)
					pr.Post("/tax-rates", deps.TaxRates.Create)
					pr.Get("/tax-rates/{id}", deps.TaxRates.Get)
					pr.Put("/tax-rates/{id}", deps.TaxRates.Update)
					pr.Delete("/tax-rates/{id}", deps.TaxRates.Delete)
				}
				if deps.Participants != nil {
					pr.Get("/participants", deps.Participants.List)
					pr.Post("/participants", deps.Participants.Create)
					pr.Post("/participants/bulk-delete", deps.Participants.BulkDelete)
					pr.Get("/participants/{id}", deps.Participants.Get)
					pr.Put("/participants/{id}", deps.Participants.Update)
					pr.Delete("/participants/{id}", deps.Participants.Delete)
				}
				if deps.CustomItems != nil {
					pr.Get("/custom-items", deps.CustomItems.List)
					pr.Post("/custom-items", deps.CustomItems.Create)
					pr.Post("/custom-items/bulk-delete", deps.CustomItems.BulkDelete)
					pr.Get("/custom-items/{id}", deps.CustomItems.Get)
					pr.Put("/custom-items/{id}", deps.CustomItems.Update)
					pr.Delete("/custom-items/{id}", deps.CustomItems.Delete)
				}
				// SupportCatalog is the GLOBAL NDIS catalogue. Reads are open to
				// any authenticated tenant user; the XLSX ingest (write) is gated
				// to platform admins (spec §5).
				if deps.SupportCatalog != nil {
					pr.Get("/support-catalog/versions", deps.SupportCatalog.ListVersions)
					pr.Get("/support-catalog/versions/{id}/items", deps.SupportCatalog.ListItems)
					pr.Get("/support-catalog/items/{itemId}/prices", deps.SupportCatalog.ListPrices)
					pr.With(RequirePlatformAdmin).Post("/support-catalog/versions", deps.SupportCatalog.Ingest)
				}
				if deps.Invoices != nil {
					pr.Get("/invoices", deps.Invoices.List)
					pr.Post("/invoices", deps.Invoices.Create)
					pr.Post("/invoices/bulk-delete", deps.Invoices.BulkDelete)
					pr.Post("/invoices/bulk-status", deps.Invoices.BulkStatus)
					pr.Get("/invoices/{id}", deps.Invoices.Get)
					pr.Put("/invoices/{id}", deps.Invoices.Update)
					pr.Delete("/invoices/{id}", deps.Invoices.Delete)
					pr.Post("/invoices/{id}/status", deps.Invoices.Status)
					pr.Get("/invoices/{id}/pdf", deps.Invoices.Pdf)
					pr.Get("/participants/{id}/stats", deps.Invoices.ParticipantStats)
				}
				if deps.Notes != nil {
					pr.Get("/participants/{id}/notes", deps.Notes.ListForParticipant)
					pr.Post("/notes", deps.Notes.Create)
					pr.Post("/notes/bill", deps.Notes.Bill)
					pr.Get("/notes/{id}", deps.Notes.Get)
					pr.Put("/notes/{id}", deps.Notes.Update)
					pr.Delete("/notes/{id}", deps.Notes.Delete)
				}
				if deps.Shifts != nil {
					pr.Get("/participants/{id}/shifts", deps.Shifts.ListForParticipant)
					pr.Get("/shifts", deps.Shifts.List)
					pr.Get("/shifts/suggestions", deps.Shifts.Suggestions)
					pr.Get("/shifts/to-record", deps.Shifts.ToRecord)
					pr.Post("/shifts", deps.Shifts.Create)
					pr.Get("/shifts/{id}", deps.Shifts.Get)
					pr.Put("/shifts/{id}", deps.Shifts.Update)
					pr.Delete("/shifts/{id}", deps.Shifts.Delete)
					pr.Post("/shifts/{id}/status", deps.Shifts.UpdateStatus)
				}
				if deps.Estimates != nil {
					pr.Get("/estimates", deps.Estimates.List)
					pr.Post("/estimates", deps.Estimates.Create)
					pr.Post("/estimates/bulk-delete", deps.Estimates.BulkDelete)
					pr.Post("/estimates/bulk-status", deps.Estimates.BulkStatus)
					pr.Get("/estimates/{id}", deps.Estimates.Get)
					pr.Put("/estimates/{id}", deps.Estimates.Update)
					pr.Delete("/estimates/{id}", deps.Estimates.Delete)
					pr.Post("/estimates/{id}/status", deps.Estimates.Status)
					pr.Post("/estimates/{id}/duplicate", deps.Estimates.Duplicate)
					pr.Get("/estimates/{id}/pdf", deps.Estimates.Pdf)
					pr.Post("/estimates/{id}/convert", deps.Estimates.Convert)
				}
				if deps.Payments != nil {
					pr.Get("/invoices/{id}/payments", deps.Payments.ListForInvoice)
					pr.Post("/invoices/{id}/payments", deps.Payments.Create)
					pr.Delete("/payments/{id}", deps.Payments.Delete)
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
					pr.Post("/participants/{id}/draft-invoice", deps.Agent.DraftInvoiceFromNotes)
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
