package app

import (
	"context"
	"log/slog"
	"time"

	"github.com/dknathalage/tallyo/internal/agent"
	"github.com/dknathalage/tallyo/internal/invoice"
	"github.com/dknathalage/tallyo/internal/recurring"
	"github.com/dknathalage/tallyo/internal/reqctx"
)

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
func runSweepOnce(inv *invoice.Service, rec *recurring.Service, ag *agent.Agent, logger *slog.Logger) {
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
func runSweeper(inv *invoice.Service, rec *recurring.Service, ag *agent.Agent, logger *slog.Logger, done <-chan struct{}) {
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
