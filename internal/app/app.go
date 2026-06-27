// Package app is the composition root for the Tallyo server. It wires together
// all service, repository, and handler layers and starts the HTTP server with
// graceful shutdown. Flag parsing lives in main; this package only needs a
// fully-resolved Config to operate.
package app

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/dknathalage/tallyo/internal/auth"
	"github.com/dknathalage/tallyo/internal/businessprofile"
	"github.com/dknathalage/tallyo/internal/catalogue"
	"github.com/dknathalage/tallyo/internal/client"
	appdb "github.com/dknathalage/tallyo/internal/db"
	"github.com/dknathalage/tallyo/internal/estimate"
	"github.com/dknathalage/tallyo/internal/invoice"
	"github.com/dknathalage/tallyo/internal/payer"
	"github.com/dknathalage/tallyo/internal/session"
	"github.com/dknathalage/tallyo/internal/smarts"
	"github.com/dknathalage/tallyo/internal/subscription"
	"github.com/dknathalage/tallyo/internal/taxrate"
	tallyoweb "github.com/dknathalage/tallyo/web"
)

// Config holds the resolved (post-flag-parse) runtime configuration for the
// Tallyo server. All fields are already validated/defaulted by main before Run
// is called.
type Config struct {
	Port           int
	LogLevel       string
	LogFormat      string
	FeatureSmarts  bool // AI "Smarts" gate; still also requires ANTHROPIC_API_KEY
	FeatureInvites bool // gates inviting new users (create/revoke invites + UI)
}

// EnvOr returns the value of env var key, or def when it is unset/empty. Used
// to let flags default from the environment while remaining overridable on the
// CLI.
func EnvOr(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

// EnvBool returns the boolean value of env var key, or def when it is
// unset/empty or unparseable. Accepts 1/t/T/TRUE/true/0/f/false (ParseBool).
func EnvBool(key string, def bool) bool {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return def
	}
	return b
}

// parseLevel maps a textual level (case-insensitive) to slog.Level, defaulting
// to info for empty or unrecognised input.
func parseLevel(s string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// setupLogger builds the root slog.Logger and installs it as the default.
// format is "json" (production) or "text" (dev); any other value falls back to
// json.
func setupLogger(format, level string) *slog.Logger {
	opts := &slog.HandlerOptions{Level: parseLevel(level)}
	var h slog.Handler
	if strings.EqualFold(strings.TrimSpace(format), "text") {
		h = slog.NewTextHandler(os.Stderr, opts)
	} else {
		h = slog.NewJSONHandler(os.Stderr, opts)
	}
	l := slog.New(h)
	slog.SetDefault(l)
	return l
}

// Run wires all layers, starts the HTTP listener, and blocks until the process
// receives SIGINT/SIGTERM or the listener fails. It performs a 10-second
// graceful shutdown before returning. version is embedded in startup logs.
func Run(cfg Config, version string) error {
	logger := setupLogger(cfg.LogFormat, cfg.LogLevel)
	logger.Info("starting tallyo", slog.String("version", version), slog.Int("port", cfg.Port))

	apiKey := EnvOr("ANTHROPIC_API_KEY", "")
	smartsEnabled := cfg.FeatureSmarts && apiKey != ""
	if !cfg.FeatureSmarts {
		logger.Warn("smarts disabled: TALLYO_FEATURE_SMARTS off")
	} else if apiKey == "" {
		logger.Warn("smarts disabled: ANTHROPIC_API_KEY unset")
	}

	dsn := EnvOr("DATABASE_URL", "")
	if dsn == "" {
		return fmt.Errorf("DATABASE_URL is required (postgres connection string)")
	}

	// Single Postgres instance: the whole app — control tables (tenants, users,
	// sessions, audit) and every tenant's business data — lives in one database.
	// Tenancy is logical: each business row carries a tenant_id and every query
	// guards on it; reqctx carries the request's tenant for guards + audit.
	database, err := appdb.Open(dsn)
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	if err := appdb.Migrate(database); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	logger.Info("database connected")
	defer func() {
		if cerr := database.Close(); cerr != nil {
			logger.Error("close db failed", slog.Any("error", cerr))
		}
	}()

	verifier, err := auth.NewFirebaseVerifier(context.Background())
	if err != nil {
		return fmt.Errorf("firebase verifier: %w", err)
	}
	users := auth.NewUsers(database)
	tenants := auth.NewTenants(database)
	invites := auth.NewInvites(database)
	bpSvc := businessprofile.NewService(database)
	payerSvc := payer.NewService(database)
	taxRateSvc := taxrate.NewService(database)
	clientSvc := client.NewService(database)
	catalogueSvc := catalogue.NewService(database)
	sessionSvc := session.NewService(database, invoice.NewInvoices(database))
	invoiceSvc := invoice.NewService(database, sessionSvc)
	estimateSvc := estimate.NewService(database)
	paymentSvc := invoice.NewPaymentService(database)

	// AI "Smarts" (optional): construct the service only when ANTHROPIC_API_KEY is
	// set. The handler is always wired — when disabled it is a guard-only handler
	// whose routes return 503 instead of falling through to the SPA catch-all.
	var smartsHandler *smarts.Handler
	if smartsEnabled {
		llm := smarts.NewAnthropicClient(apiKey, EnvOr("ANTHROPIC_MODEL", ""), EnvOr("ANTHROPIC_EFFORT", ""))
		smartsSvc := smarts.NewService(llm, sessionSvc, catalogue.NewRepo(database), invoiceSvc, invoiceSvc, clientSvc)
		smartsHandler = smarts.NewHandler(smartsSvc, true)
	} else {
		smartsHandler = smarts.NewHandler(nil, false)
	}

	// SaaS billing (optional, behind BILLING_ENABLED). When off, the handler is
	// nil (routes unmounted) and ResolveTenant treats every tenant as entitled.
	// A misconfigured-but-enabled billing setup is fatal: better to fail at boot
	// than silently run unbilled.
	billingCfg := subscription.LoadConfig()
	var subHandler *subscription.Handler
	if billingCfg.Enabled {
		stripeClient, cerr := subscription.NewClient(billingCfg)
		if cerr != nil {
			return fmt.Errorf("billing enabled but misconfigured: %w", cerr)
		}
		subHandler = subscription.NewHandler(stripeClient, subscription.NewStore(database), tenants)
		logger.Info("billing enabled", slog.Int("trial_days", billingCfg.TrialDays))
	} else {
		logger.Warn("billing disabled: BILLING_ENABLED off — all tenants entitled")
	}

	assets, err := fs.Sub(tallyoweb.Build, "build")
	if err != nil {
		return fmt.Errorf("sub web build: %w", err)
	}

	if _, err := fs.Stat(assets, "200.html"); err != nil {
		return fmt.Errorf("embedded SPA missing 200.html — run `npm run build` in web/ before `go build`: %w", err)
	}

	deps := Deps{
		Assets:          assets,
		Users:           users,
		Tenants:         tenants,
		Verifier:        verifier,
		AuthConfig:      NewAuthConfigHandler(),
		Signup:          NewSignupHandler(tenants, users, provisionProfile(database)),
		Auth:            NewAuthHandler(users, tenants),
		Invites:         NewInviteHandler(invites, users),
		BusinessProfile: businessprofile.NewHandler(bpSvc),
		Payers:          payer.NewHandler(payerSvc),
		TaxRates:        taxrate.NewHandler(taxRateSvc),
		Clients:         client.NewHandler(clientSvc),
		Catalogue:       catalogue.NewHandler(catalogueSvc),
		Invoices:        invoice.NewHandler(invoiceSvc),
		Sessions:        session.NewHandler(sessionSvc),
		Estimates:       estimate.NewHandler(estimateSvc),
		Payments:        invoice.NewPaymentHandler(paymentSvc),
		Smarts:          smartsHandler,
		Subscription:    subHandler,
		BillingEnabled:  billingCfg.Enabled,
		// smarts is "on" only when both the gate and the API key allow it, so the
		// SPA hides AI affordances that would otherwise 503.
		Features: map[string]bool{
			"smarts":  smartsEnabled,
			"invites": cfg.FeatureInvites,
			"billing": billingCfg.Enabled,
		},
	}

	server := NewServer(deps)

	// baseCtx is the parent of every request context. Cancelling it on shutdown
	// signals any long-lived handlers to return so srv.Shutdown can drain instead
	// of blocking until its timeout.
	baseCtx, cancelBase := context.WithCancel(context.Background())
	defer cancelBase()
	srv := &http.Server{
		Addr:        fmt.Sprintf(":%d", cfg.Port),
		Handler:     server.Router,
		BaseContext: func(net.Listener) context.Context { return baseCtx },
	}

	errCh := make(chan error, 1)
	go func() {
		logger.Info("listening", slog.Int("port", cfg.Port), slog.String("version", version))
		if serveErr := srv.ListenAndServe(); serveErr != nil && serveErr != http.ErrServerClosed {
			errCh <- serveErr
			return
		}
		errCh <- nil
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-errCh:
		if err != nil {
			return fmt.Errorf("serve: %w", err)
		}
		return nil
	case <-stop:
		logger.Info("shutting down")
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		cancelBase() // release long-lived handlers so Shutdown can drain
		if err := srv.Shutdown(ctx); err != nil {
			return fmt.Errorf("shutdown: %w", err)
		}
		return nil
	}
}
