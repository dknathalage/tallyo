package agent

import (
	"context"
	"fmt"
	"time"

	"github.com/dknathalage/tallyo/internal/db/gen"
	"github.com/dknathalage/tallyo/internal/reqctx"
)

// retentionWindow is how long terminal agent step / checkpoint-change rows are
// kept before the retention sweep prunes them.
const retentionWindow = 30 * 24 * time.Hour

// SweepExpired is the global background sweep for the agent feature. It fences
// awaiting risky-tool steps whose await_expires_at has passed, then prunes old
// step / checkpoint-change rows past the retention window.
//
// For each expired awaiting step it: marks the step denied, commits its
// checkpoint (so any prior allowed changes in the same turn stand rather than
// being rolled back), and publishes a step_expired event on the step's
// conversation. A failure for one step is returned to the caller after the loop
// finishes the remaining steps, so one bad row cannot stall the whole sweep.
//
// This is a system action with no acting user; each step is processed under a
// context carrying ITS OWN tenant_id so the tenant-scoped store mutations and
// audit stamping resolve correctly.
func (a *Agent) SweepExpired(ctx context.Context) error {
	if ctx == nil {
		return fmt.Errorf("sweep: nil context")
	}
	now := a.clock.Now().UTC()
	cutoff := now.Format(time.RFC3339)

	steps, err := a.store.ListExpiredAwaitingSteps(ctx, cutoff)
	if err != nil {
		return fmt.Errorf("sweep: list expired: %w", err)
	}

	var firstErr error
	for i := range steps { // bounded by len(steps)
		if err := a.expireStep(ctx, steps[i]); err != nil {
			// Log-and-continue: record the first failure but keep sweeping so
			// one tenant's bad row cannot block the rest.
			if firstErr == nil {
				firstErr = err
			}
		}
	}

	if err := a.pruneRetention(ctx, now); err != nil {
		if firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// expireStep denies one expired awaiting step under its own tenant context,
// commits its checkpoint, and publishes a step_expired event.
func (a *Agent) expireStep(ctx context.Context, step gen.AgentStep) error {
	if step.TenantID <= 0 {
		return fmt.Errorf("sweep: step %d has no tenant", step.ID)
	}
	// Scope the mutations + audit to the step's tenant (this is a global sweep,
	// so the incoming ctx is not tenant-bound).
	tctx := reqctx.WithTenant(ctx, step.TenantID)

	if err := a.store.UpdateStepStatus(tctx, step.ID, "denied", ""); err != nil {
		return fmt.Errorf("sweep: deny step %d: %w", step.ID, err)
	}
	if step.CheckpointID.Valid && step.CheckpointID.Int64 > 0 {
		if err := a.commitCheckpoint(tctx, step.CheckpointID.Int64); err != nil {
			return fmt.Errorf("sweep: commit checkpoint %d: %w", step.CheckpointID.Int64, err)
		}
	}

	conv, err := a.store.GetConversationByMessage(tctx, step.MessageID)
	if err != nil {
		return fmt.Errorf("sweep: load conversation for message %d: %w", step.MessageID, err)
	}
	a.events.Publish(conv.ID, Event{Type: "step_expired", Data: map[string]any{
		"stepId":   step.ID,
		"toolName": step.ToolName,
	}})
	return nil
}

// pruneRetention deletes terminal step and checkpoint-change rows older than the
// retention window. The prune is purely time-based for v1 (it does not exclude
// rows tied to still-open checkpoints); the window is long enough that an open
// turn is never that old in practice.
func (a *Agent) pruneRetention(ctx context.Context, now time.Time) error {
	cutoff := now.Add(-retentionWindow).Format(time.RFC3339)
	if err := a.store.PruneCheckpointChanges(ctx, cutoff); err != nil {
		return fmt.Errorf("sweep: prune checkpoint changes: %w", err)
	}
	if err := a.store.PruneSteps(ctx, cutoff); err != nil {
		return fmt.Errorf("sweep: prune steps: %w", err)
	}
	return nil
}
