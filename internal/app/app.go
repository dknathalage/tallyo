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
	"strings"
	"syscall"
	"time"

	"github.com/dknathalage/tallyo/internal/agent"
	"github.com/dknathalage/tallyo/internal/agent/llm"
	"github.com/dknathalage/tallyo/internal/auth"
	"github.com/dknathalage/tallyo/internal/businessprofile"
	"github.com/dknathalage/tallyo/internal/catalog"
	"github.com/dknathalage/tallyo/internal/customitem"
	appdb "github.com/dknathalage/tallyo/internal/db"
	"github.com/dknathalage/tallyo/internal/estimate"
	"github.com/dknathalage/tallyo/internal/export"
	"github.com/dknathalage/tallyo/internal/invoice"
	"github.com/dknathalage/tallyo/internal/participant"
	"github.com/dknathalage/tallyo/internal/planmanager"
	"github.com/dknathalage/tallyo/internal/realtime"
	"github.com/dknathalage/tallyo/internal/recurring"
	"github.com/dknathalage/tallyo/internal/shift"
	"github.com/dknathalage/tallyo/internal/taxrate"
	tallyoweb "github.com/dknathalage/tallyo/web"
)

// Config holds the resolved (post-flag-parse) runtime configuration for the
// Tallyo server. All fields are already validated/defaulted by main before Run
// is called.
type Config struct {
	Port         int
	DataDir      string // empty → resolved from OS app-data dir
	SecureCookie bool
	LogLevel     string
	LogFormat    string
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
	if !agentCfg.Enabled() {
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

	conn, err := appdb.Open(filepath.Join(dir, "tallyo-go.db"))
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	defer func() {
		if cerr := conn.Close(); cerr != nil {
			logger.Error("close db failed", slog.Any("error", cerr))
		}
	}()

	if err := appdb.Migrate(conn); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}

	hub := realtime.NewHub()
	sm := auth.NewSessionManager(conn, cfg.SecureCookie)
	users := auth.NewUsers(conn)
	tenants := auth.NewTenants(conn)
	invites := auth.NewInvites(conn)
	bpSvc := businessprofile.NewService(conn, hub)
	planManagerSvc := planmanager.NewService(conn, hub)
	taxRateSvc := taxrate.NewService(conn, hub)
	participantSvc := participant.NewService(conn, hub)
	customItemSvc := customitem.NewService(conn, hub)
	supportCatalogSvc := catalog.NewService(conn)
	catalogIngestSvc := catalog.NewIngestService(conn, hub)
	shiftSvc := shift.NewService(conn, hub, invoice.NewInvoices(conn))
	invoiceSvc := invoice.NewService(conn, hub, shift.NewShifts(conn))
	estimateSvc := estimate.NewService(conn, hub)
	paymentSvc := invoice.NewPaymentService(conn, hub)
	recurringSvc := recurring.NewService(conn, hub)

	// AI "Smarts" (optional): the service is only constructed when
	// ANTHROPIC_API_KEY is set. The HTTP handler is ALWAYS constructed and wired:
	// when disabled it is a guard-only handler (nil smarts, enabled=false) so the
	// Smart routes are registered and return a clean 503 instead of falling
	// through to the SPA catch-all (200 index.html).
	var smartsHandler *agent.SmartsHandler
	if agentCfg.APIKey != "" {
		llmClient := llm.NewAnthropic(agentCfg.APIKey, agentCfg.Model, agentCfg.EffortFor())
		smarts := agent.NewSmarts(agentCfg, llmClient, invoiceSvc, shiftSvc, supportCatalogSvc)
		smartsHandler = agent.NewSmartsHandler(smarts, true)
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
		Signup:          NewSignupHandler(sm, tenants, users),
		Auth:            NewAuthHandler(sm, users, tenants),
		Invites:         NewInviteHandler(invites, users),
		Events:          realtime.NewEventsHandler(hub),
		BusinessProfile: businessprofile.NewHandler(bpSvc),
		PlanManagers:    planmanager.NewHandler(planManagerSvc),
		TaxRates:        taxrate.NewHandler(taxRateSvc),
		Participants:    participant.NewHandler(participantSvc),
		CustomItems:     customitem.NewHandler(customItemSvc),
		SupportCatalog:  catalog.NewHandler(supportCatalogSvc, catalogIngestSvc),
		Invoices:        invoice.NewHandler(invoiceSvc),
		Shifts:          shift.NewHandler(shiftSvc),
		Estimates:       estimate.NewHandler(estimateSvc),
		Payments:        invoice.NewPaymentHandler(paymentSvc),
		Recurring:       recurring.NewHandler(recurringSvc),
		Export:          export.NewHandler(customItemSvc, invoiceSvc, estimateSvc),
		Smarts:          smartsHandler,
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
	runSweepOnce(invoiceSvc, recurringSvc, logger)
	overdueDone := make(chan struct{})
	go runSweeper(invoiceSvc, recurringSvc, logger, overdueDone)
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
