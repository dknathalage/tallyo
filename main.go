package main

import (
	"context"
	"flag"
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
	appdb "github.com/dknathalage/tallyo/internal/db"
	httpapi "github.com/dknathalage/tallyo/internal/http"
	"github.com/dknathalage/tallyo/internal/realtime"
	"github.com/dknathalage/tallyo/internal/reqctx"
	"github.com/dknathalage/tallyo/internal/service"
	"github.com/dknathalage/tallyo/internal/taxrate"
	tallyoweb "github.com/dknathalage/tallyo/web"
)

// version is the build version. Source builds report "dev"; to stamp a release
// build, pass -ldflags="-X main.version=$(git describe --tags)".
var version = "dev"

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", slog.Any("error", err))
		os.Exit(1)
	}
}

// envOr returns the value of env var key, or def when it is unset/empty. Used to
// let flags default from the environment while remaining overridable on the CLI.
func envOr(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

// parseLevel maps a textual level (case-insensitive) to slog.Level, defaulting
// to info for empty or unrecognized input.
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

// setupLogger builds the root slog.Logger and installs it as the default. format
// is "json" (production) or "text" (dev); any other value falls back to json.
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

// overdueSweepInterval is how often the background sweeper flips due invoices.
const overdueSweepInterval = 1 * time.Hour

// runSweepOnce runs the overdue + recurring sweeps once, PER ACTIVE TENANT
// (spec §8). Suspended tenants are skipped by ActiveTenantIDs (it returns only
// status='active' tenants). Each tenant is swept under its own context carrying
// the tenant id (reqctx.WithTenant), so the tenant-scoped service methods, their
// SSE broadcasts, and the audit stamping all resolve to the right tenant. The
// sweep is a system action with no acting user, so audit user_id is NULL.
//
// A failure for one tenant is logged and the sweep continues with the next, so
// one tenant's data problem cannot stall every other tenant's sweep.
func runSweepOnce(inv *service.InvoiceService, rec *service.RecurringService, ag *agent.Agent, logger *slog.Logger) {
	tenantIDs, err := inv.ActiveTenantIDs(context.Background())
	if err != nil {
		logger.Error("sweep: list active tenants failed", slog.Any("error", err))
		return
	}
	for i := range tenantIDs { // bounded by len(tenantIDs)
		tid := tenantIDs[i]
		ctx := reqctx.WithTenant(context.Background(), tid)
		if rows, err := inv.MarkOverdueForTenant(ctx, tid); err != nil {
			logger.Error("overdue sweep failed", slog.Int64("tenant_id", tid), slog.Any("error", err))
		} else if len(rows) > 0 {
			logger.Info("overdue sweep", slog.Int64("tenant_id", tid), slog.Int("flipped", len(rows)))
		}
		if gens, err := rec.GenerateDueForTenant(ctx, tid); err != nil {
			logger.Error("recurring sweep failed", slog.Int64("tenant_id", tid), slog.Any("error", err))
		} else if len(gens) > 0 {
			logger.Info("recurring sweep", slog.Int64("tenant_id", tid), slog.Int("generated", len(gens)))
		}
	}
	// Agent expired-step + retention sweep (global, not per-tenant). nil when the
	// AI agent is disabled. A failure is logged and does not abort the sweep.
	if ag != nil {
		if err := ag.SweepExpired(context.Background()); err != nil {
			logger.Error("agent sweep failed", slog.Any("error", err))
		}
	}
}

// runSweeper runs the per-tenant sweeps on each tick until done is closed. It
// owns its single ticker and stops cleanly, so it never leaks a goroutine.
func runSweeper(inv *service.InvoiceService, rec *service.RecurringService, ag *agent.Agent, logger *slog.Logger, done <-chan struct{}) {
	ticker := time.NewTicker(overdueSweepInterval)
	defer ticker.Stop()
	for { // bounded by the done signal
		select {
		case <-done:
			return
		case <-ticker.C:
			runSweepOnce(inv, rec, ag, logger)
		}
	}
}

func run() error {
	port := flag.Int("port", 8080, "HTTP listen port")
	dataDir := flag.String("data-dir", "", "data directory for the SQLite database (default: OS app data dir)")
	secureCookie := flag.Bool("secure-cookie", false, "mark the session cookie Secure (HTTPS only)")
	showVersion := flag.Bool("version", false, "print the version and exit")
	logLevel := flag.String("log-level", envOr("LOG_LEVEL", "info"), "log level: debug|info|warn|error")
	logFormat := flag.String("log-format", envOr("LOG_FORMAT", "json"), "log format: json (production) | text (dev)")
	flag.Parse()

	if *showVersion {
		fmt.Println(version)
		return nil
	}

	logger := setupLogger(*logFormat, *logLevel)

	agentCfg := agent.Config{
		APIKey: envOr("ANTHROPIC_API_KEY", ""),
		Model:  envOr("ANTHROPIC_MODEL", ""),
		Effort: envOr("ANTHROPIC_EFFORT", ""),
	}
	if e := agentCfg.Effort; e != "" && !agent.ValidEffort(e) {
		logger.Warn("invalid ANTHROPIC_EFFORT; falling back to default",
			slog.String("value", e))
	}
	agentCfg = agentCfg.WithDefaults()
	// Skip the forced plan turn by default (cuts a round-trip, restores thinking);
	// set AGENT_SKIP_PLAN=0 to restore the plan phase + its UX preview.
	agentCfg.SkipPlan = envOr("AGENT_SKIP_PLAN", "1") != "0"
	if !agentCfg.Enabled() {
		logger.Warn("agent disabled: ANTHROPIC_API_KEY unset")
	}
	logger.Info("agent model configured",
		slog.String("model", agentCfg.Model), slog.String("effort", agentCfg.Effort))

	dir := *dataDir
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
	sm := auth.NewSessionManager(conn, *secureCookie)
	users := auth.NewUsers(conn)
	tenants := auth.NewTenants(conn)
	invites := auth.NewInvites(conn)
	bpSvc := service.NewBusinessProfileService(conn, hub)
	planManagerSvc := service.NewPlanManagerService(conn, hub)
	taxRateSvc := taxrate.NewService(conn, hub)
	participantSvc := service.NewParticipantService(conn, hub)
	customItemSvc := service.NewCustomItemService(conn, hub)
	supportCatalogSvc := service.NewSupportCatalogService(conn)
	catalogIngestSvc := service.NewCatalogIngestService(conn, hub)
	invoiceSvc := service.NewInvoiceService(conn, hub)
	shiftSvc := service.NewShiftService(conn, hub)
	estimateSvc := service.NewEstimateService(conn, hub)
	paymentSvc := service.NewPaymentService(conn, hub)
	recurringSvc := service.NewRecurringService(conn, hub)

	// AI agent (optional): the full service is only constructed when
	// ANTHROPIC_API_KEY is set. The HTTP handler is ALWAYS constructed and wired
	// (BUG 3): when disabled it is a guard-only handler (nil agent/budget,
	// enabled=false) so /api/agent/* routes are registered and return a clean 503
	// instead of falling through to the SPA catch-all (200 index.html). agentSvc
	// stays nil when disabled → the sweeper skips SweepExpired.
	var agentHandler *httpapi.AgentHandler
	var agentSvc *agent.Agent
	if agentCfg.Enabled() {
		agentStore := agent.NewStore(conn)
		agentEvents := agent.NewEvents()
		agentReg := agent.NewRegistry()
		agentCP := agent.NewCheckpoint(agentStore, conn)
		agentReg.Register(agent.NewListInvoicesTool(invoiceSvc))
		// Shifts lifecycle is the billing source of truth: there is exactly one
		// create_invoice tool, the shift-completeness-verified variant.
		agentReg.Register(agent.NewCreateInvoiceToolForShifts(invoiceSvc, shiftSvc, agentCP))
		agentReg.Register(agent.NewListParticipantShiftsToolWithCatalog(shiftSvc, supportCatalogSvc))
		agentReg.Register(agent.NewSearchCatalogueTool(supportCatalogSvc))
		agentBudget := agent.NewBudgetWallClock(agentStore, agentCfg)
		llmClient := llm.NewAnthropic(agentCfg.APIKey, agentCfg.Model, agentCfg.EffortFor())
		agentSvc = agent.NewAgent(agentCfg, llmClient, agentStore, agentReg, agentCP, agentEvents).
			WithBudget(agentBudget).
			WithRestore(agent.InvoiceRestoreFunc(invoiceSvc))
		agentHandler = httpapi.NewAgentHandler(agentSvc, agentBudget, true).
			WithShiftImport(shiftSvc, llmClient, agentCfg)
	} else {
		agentHandler = httpapi.NewAgentHandler(nil, nil, false)
	}

	assets, err := fs.Sub(tallyoweb.Build, "build")
	if err != nil {
		return fmt.Errorf("sub web build: %w", err)
	}

	if _, err := fs.Stat(assets, "200.html"); err != nil {
		return fmt.Errorf("embedded SPA missing 200.html — run `npm run build` in web/ before `go build`: %w", err)
	}

	deps := httpapi.Deps{
		Assets:          assets,
		Users:           users,
		Tenants:         tenants,
		Session:         sm,
		Signup:          httpapi.NewSignupHandler(sm, tenants, users),
		Auth:            httpapi.NewAuthHandler(sm, users, tenants),
		Invites:         httpapi.NewInviteHandler(invites, users),
		Events:          httpapi.NewEventsHandler(hub),
		BusinessProfile: httpapi.NewBusinessProfileHandler(bpSvc),
		PlanManagers:    httpapi.NewPlanManagerHandler(planManagerSvc),
		TaxRates:        taxrate.NewHandler(taxRateSvc),
		Participants:    httpapi.NewParticipantHandler(participantSvc),
		CustomItems:     httpapi.NewCustomItemHandler(customItemSvc),
		SupportCatalog:  httpapi.NewSupportCatalogHandler(supportCatalogSvc, catalogIngestSvc),
		Invoices:        httpapi.NewInvoiceHandler(invoiceSvc),
		Shifts:          httpapi.NewShiftHandler(shiftSvc),
		Estimates:       httpapi.NewEstimateHandler(estimateSvc),
		Payments:        httpapi.NewPaymentHandler(paymentSvc),
		Recurring:       httpapi.NewRecurringHandler(recurringSvc),
		Export:          httpapi.NewExportHandler(customItemSvc, invoiceSvc, estimateSvc),
		Agent:           agentHandler,
	}

	server := httpapi.NewServer(deps)

	// baseCtx is the parent of every request context. Cancelling it on shutdown
	// signals long-lived handlers (the SSE /api/events stream) to return so
	// srv.Shutdown can drain instead of blocking until its timeout.
	baseCtx, cancelBase := context.WithCancel(context.Background())
	defer cancelBase()
	srv := &http.Server{
		Addr:        fmt.Sprintf(":%d", *port),
		Handler:     server.Router,
		BaseContext: func(net.Listener) context.Context { return baseCtx },
	}

	// Run one per-tenant sweep at startup, then keep a background sweeper running
	// on an hourly tick. The done channel stops the goroutine on shutdown so it
	// does not leak.
	runSweepOnce(invoiceSvc, recurringSvc, agentSvc, logger)
	overdueDone := make(chan struct{})
	go runSweeper(invoiceSvc, recurringSvc, agentSvc, logger, overdueDone)
	defer close(overdueDone)

	errCh := make(chan error, 1)
	go func() {
		logger.Info("listening", slog.Int("port", *port))
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
