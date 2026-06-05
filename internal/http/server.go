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

	// Events, when non-nil, serves the auth-gated SSE stream at GET /api/events.
	Events *EventsHandler

	// BusinessProfile, when non-nil, serves the auth-gated GET/PUT singleton
	// business profile at /api/business-profile.
	BusinessProfile *BusinessProfileHandler

	// RateTiers, when non-nil, serves the auth-gated rate-tier CRUD routes
	// under /api/rate-tiers.
	RateTiers *RateTierHandler

	// Payers, when non-nil, serves the auth-gated payer CRUD plus bulk-delete
	// routes under /api/payers.
	Payers *PayerHandler

	// TaxRates, when non-nil, serves the auth-gated tax-rate CRUD routes under
	// /api/tax-rates.
	TaxRates *TaxRateHandler

	// Clients, when non-nil, serves the auth-gated client CRUD plus bulk-delete
	// routes under /api/clients.
	Clients *ClientHandler

	// Catalog, when non-nil, serves the auth-gated catalog CRUD, categories,
	// bulk-delete, and per-item tier-rate sub-routes under /api/catalog.
	Catalog *CatalogHandler

	// Invoices, when non-nil, serves the auth-gated invoice CRUD, status,
	// duplicate, bulk routes, plus the per-client stats route under /api.
	Invoices *InvoiceHandler

	// Estimates, when non-nil, serves the auth-gated estimate CRUD, status,
	// duplicate, bulk, and convert-to-invoice routes under /api/estimates.
	Estimates *EstimateHandler

	// Payments, when non-nil, serves the auth-gated per-invoice payment list
	// and create routes plus payment deletion under /api.
	Payments *PaymentHandler

	// Recurring, when non-nil, serves the auth-gated recurring-template CRUD
	// plus the generate route under /api/recurring.
	Recurring *RecurringHandler
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
		if deps.Auth != nil || deps.Invites != nil || deps.Events != nil || deps.BusinessProfile != nil || deps.RateTiers != nil || deps.Payers != nil || deps.TaxRates != nil || deps.Clients != nil || deps.Catalog != nil || deps.Invoices != nil || deps.Estimates != nil || deps.Payments != nil || deps.Recurring != nil {
			api.Group(func(pr chi.Router) {
				pr.Use(RequireAuth(deps.Session, deps.Users))
				if deps.Auth != nil {
					pr.Get("/auth/me", deps.Auth.Me)
				}
				if deps.Invites != nil {
					pr.Post("/invites", deps.Invites.Create)
				}
				if deps.Events != nil {
					pr.Get("/events", deps.Events.Stream)
				}
				if deps.BusinessProfile != nil {
					pr.Get("/business-profile", deps.BusinessProfile.Get)
					pr.Put("/business-profile", deps.BusinessProfile.Put)
				}
				if deps.RateTiers != nil {
					pr.Get("/rate-tiers", deps.RateTiers.List)
					pr.Post("/rate-tiers", deps.RateTiers.Create)
					pr.Get("/rate-tiers/{id}", deps.RateTiers.Get)
					pr.Put("/rate-tiers/{id}", deps.RateTiers.Update)
					pr.Delete("/rate-tiers/{id}", deps.RateTiers.Delete)
				}
				if deps.Payers != nil {
					pr.Get("/payers", deps.Payers.List)
					pr.Post("/payers", deps.Payers.Create)
					pr.Post("/payers/bulk-delete", deps.Payers.BulkDelete)
					pr.Get("/payers/{id}", deps.Payers.Get)
					pr.Put("/payers/{id}", deps.Payers.Update)
					pr.Delete("/payers/{id}", deps.Payers.Delete)
				}
				if deps.TaxRates != nil {
					pr.Get("/tax-rates", deps.TaxRates.List)
					pr.Post("/tax-rates", deps.TaxRates.Create)
					pr.Get("/tax-rates/{id}", deps.TaxRates.Get)
					pr.Put("/tax-rates/{id}", deps.TaxRates.Update)
					pr.Delete("/tax-rates/{id}", deps.TaxRates.Delete)
				}
				if deps.Clients != nil {
					pr.Get("/clients", deps.Clients.List)
					pr.Post("/clients", deps.Clients.Create)
					pr.Post("/clients/bulk-delete", deps.Clients.BulkDelete)
					pr.Get("/clients/{id}", deps.Clients.Get)
					pr.Put("/clients/{id}", deps.Clients.Update)
					pr.Delete("/clients/{id}", deps.Clients.Delete)
				}
				if deps.Catalog != nil {
					pr.Get("/catalog", deps.Catalog.List)
					pr.Post("/catalog", deps.Catalog.Create)
					pr.Get("/catalog/categories", deps.Catalog.Categories)
					pr.Post("/catalog/bulk-delete", deps.Catalog.BulkDelete)
					pr.Get("/catalog/{id}", deps.Catalog.Get)
					pr.Put("/catalog/{id}", deps.Catalog.Update)
					pr.Delete("/catalog/{id}", deps.Catalog.Delete)
					pr.Get("/catalog/{id}/rates", deps.Catalog.GetRates)
					pr.Put("/catalog/{id}/rates/{tierId}", deps.Catalog.SetRate)
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
					pr.Post("/invoices/{id}/duplicate", deps.Invoices.Duplicate)
					pr.Get("/invoices/{id}/pdf", deps.Invoices.Pdf)
					pr.Get("/clients/{id}/stats", deps.Invoices.ClientStats)
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
			})
		}
	})

	// SPA static handler must be registered last as the catch-all so that
	// /healthz and /api take precedence.
	r.Handle("/*", SPAHandler(deps.Assets))

	return &Server{Router: r}
}
