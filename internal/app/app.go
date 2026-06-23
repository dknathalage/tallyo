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
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/dknathalage/tallyo/internal/agent"
	"github.com/dknathalage/tallyo/internal/agent/llm"
	"github.com/dknathalage/tallyo/internal/auth"
	"github.com/dknathalage/tallyo/internal/businessprofile"
	"github.com/dknathalage/tallyo/internal/catalog"
	"github.com/dknathalage/tallyo/internal/client"
	"github.com/dknathalage/tallyo/internal/customitem"
	appdb "github.com/dknathalage/tallyo/internal/db"
	"github.com/dknathalage/tallyo/internal/estimate"
	"github.com/dknathalage/tallyo/internal/invoice"
	"github.com/dknathalage/tallyo/internal/planmanager"
	"github.com/dknathalage/tallyo/internal/realtime"
	"github.com/dknathalage/tallyo/internal/recurring"
	"github.com/dknathalage/tallyo/internal/shift"
	"github.com/dknathalage/tallyo/internal/taxrate"
	"github.com/dknathalage/tallyo/internal/tenantdb"
	tallyoweb "github.com/dknathalage/tallyo/web"
)

// Config holds the resolved (post-flag-parse) runtime configuration for the
// Tallyo server. All fields are already validated/defaulted by main before Run
// is called.
type Config struct {
	Port         int
	DataDir      string // empty → DATA_DIR env, else ./data
	SecureCookie bool
	LogLevel     string
	LogFormat    string
	FeatureAgent bool // AI "Smarts" gate; still also requires ANTHROPIC_API_KEY
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

	agentCfg := agent.Config{
		APIKey: EnvOr("ANTHROPIC_API_KEY", ""),
		Model:  EnvOr("ANTHROPIC_MODEL", ""),
		Effort: EnvOr("ANTHROPIC_EFFORT", ""),
	}
	if e := agentCfg.Effort; e != "" && !agent.ValidEffort(e) {
		logger.Warn("invalid ANTHROPIC_EFFORT; falling back to default",
			slog.String("value", e))
	}
	agentCfg = agentCfg.WithDefaults()
	if !cfg.FeatureAgent {
		logger.Warn("agent disabled: TALLYO_FEATURE_AGENT off")
	} else if !agentCfg.Enabled() {
		logger.Warn("agent disabled: ANTHROPIC_API_KEY unset")
	}
	logger.Info("agent model configured",
		slog.String("model", agentCfg.Model), slog.String("effort", agentCfg.Effort))

	dir := cfg.DataDir
	if dir == "" {
		d, err := appdb.DataDir()
		if err != nil {
			return fmt.Errorf("data dir: %w", err)
		}
		dir = d
	}

	// DB-per-tenant: one shared control DB (registry, auth, sessions, catalogue)
	// and one SQLite file per tenant, opened on demand by the registry. tdb is the
	// per-request routing handle for tenant-plane repositories; control is the
	// shared handle for control-plane repositories.
	control, err := appdb.Open(filepath.Join(dir, "control.db"))
	if err != nil {
		return fmt.Errorf("open control db: %w", err)
	}
	if err := appdb.MigrateControl(control); err != nil {
		return fmt.Errorf("migrate control: %w", err)
	}
	reg := tenantdb.New(control, dir)
	defer func() {
		if cerr := reg.Close(); cerr != nil {
			logger.Error("close db failed", slog.Any("error", cerr))
		}
	}()
	tdb := reg.Tenant()

	hub := realtime.NewHub()
	sm := auth.NewSessionManager(control, cfg.SecureCookie)
	users := auth.NewUsers(control)
	tenants := auth.NewTenants(control)
	invites := auth.NewInvites(control)
	bpSvc := businessprofile.NewService(tdb, hub)
	planManagerSvc := planmanager.NewService(tdb, hub)
	taxRateSvc := taxrate.NewService(tdb, hub)
	clientSvc := client.NewService(tdb, hub)
	customItemSvc := customitem.NewService(tdb, hub)
	supportCatalogSvc := catalog.NewService(tdb)
	catalogIngestSvc := catalog.NewIngestService(tdb, hub)
	shiftSvc := shift.NewService(tdb, control, hub, invoice.NewInvoices(tdb))
	invoiceSvc := invoice.NewService(tdb, control, hub, shiftSvc)
	estimateSvc := estimate.NewService(tdb, control, hub)
	paymentSvc := invoice.NewPaymentService(tdb, hub)
	recurringSvc := recurring.NewService(tdb, hub)

	// AI "Smarts" (optional): the service is only constructed when
	// ANTHROPIC_API_KEY is set. The HTTP handler is ALWAYS constructed and wired:
	// when disabled it is a guard-only handler (nil smarts, enabled=false) so the
	// Smart routes are registered and return a clean 503 instead of falling
	// through to the SPA catch-all (200 index.html).
	var smartsHandler *agent.SmartsHandler
	var shiftDivider shift.ShiftDivider // nil when AI is disabled → /divide 503s
	if cfg.FeatureAgent && agentCfg.APIKey != "" {
		llmClient := llm.NewAnthropic(agentCfg.APIKey, agentCfg.Model, agentCfg.EffortFor())
		smarts := agent.NewSmarts(agentCfg, llmClient, shiftSvc, supportCatalogSvc)
		smartsHandler = agent.NewSmartsHandler(smarts, true)
		shiftDivider = smarts
	} else {
		smartsHandler = agent.NewSmartsHandler(nil, false)
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
		Session:         sm,
		Signup:          NewSignupHandler(sm, tenants, users, provisionProfile(reg)),
		Auth:            NewAuthHandler(sm, users, tenants),
		Invites:         NewInviteHandler(invites, users),
		Events:          realtime.NewEventsHandler(hub),
		BusinessProfile: businessprofile.NewHandler(bpSvc),
		PlanManagers:    planmanager.NewHandler(planManagerSvc),
		TaxRates:        taxrate.NewHandler(taxRateSvc),
		Clients:         client.NewHandler(clientSvc),
		CustomItems:     customitem.NewHandler(customItemSvc),
		SupportCatalog:  catalog.NewHandler(supportCatalogSvc, catalogIngestSvc),
		Invoices:        invoice.NewHandler(invoiceSvc),
		Shifts:          shift.NewHandler(shiftSvc, shiftDivider),
		Estimates:       estimate.NewHandler(estimateSvc),
		Payments:        invoice.NewPaymentHandler(paymentSvc),
		Recurring:       recurring.NewHandler(recurringSvc),
		Smarts:          smartsHandler,
		// agent is "on" only when both the gate and the API key allow it, so the
		// SPA hides AI affordances that would otherwise 503.
		Features: map[string]bool{
			"agent": cfg.FeatureAgent && agentCfg.APIKey != "",
		},
	}

	server := NewServer(deps)

	// baseCtx is the parent of every request context. Cancelling it on shutdown
	// signals long-lived handlers (the SSE /api/events stream) to return so
	// srv.Shutdown can drain instead of blocking until its timeout.
	baseCtx, cancelBase := context.WithCancel(context.Background())
	defer cancelBase()
	srv := &http.Server{
		Addr:        fmt.Sprintf(":%d", cfg.Port),
		Handler:     server.Router,
		BaseContext: func(net.Listener) context.Context { return baseCtx },
	}

	// Run one per-tenant sweep at startup, then keep a background sweeper running
	// on an hourly tick. The done channel stops the goroutine on shutdown so it
	// does not leak.
	runSweepOnce(tenants.ActiveTenantIDs, invoiceSvc, recurringSvc, logger)
	overdueDone := make(chan struct{})
	go runSweeper(tenants.ActiveTenantIDs, invoiceSvc, recurringSvc, logger, overdueDone)
	defer close(overdueDone)

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
		cancelBase() // release long-lived SSE handlers so Shutdown can drain
		if err := srv.Shutdown(ctx); err != nil {
			return fmt.Errorf("shutdown: %w", err)
		}
		return nil
	}
}
