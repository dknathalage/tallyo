package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/dknathalage/tallyo/internal/db/gen"
)

// ErrStepResolved signals a Decide on a step that is no longer awaiting. A
// second decision on the same step is a no-op error (idempotency), never a
// re-run of the risky tool.
var ErrStepResolved = errors.New("agent: step already resolved")

// Decide resolves an awaiting risky-tool step and resumes the execute loop.
//
// When allow is true the step's tool runs (under its captured checkpoint, so a
// write records its change), its result is fed back as a tool_result, and the
// step is marked done. When allow is false the step is marked denied and an
// is_error tool_result ("user denied") is fed. Either way the loop is resumed
// from exactly where it suspended: because the tool_result was just persisted,
// loadHistory now ends with it and the model continues under AUTO tool choice
// (not a forced re-plan).
//
// Decide is idempotent: a decision on a step that is no longer awaiting returns
// ErrStepResolved without re-running anything.
func (a *Agent) Decide(ctx context.Context, stepID int64, allow bool) error {
	if stepID <= 0 {
		return fmt.Errorf("decide: invalid stepID %d", stepID)
	}

	step, err := a.store.GetStep(ctx, stepID)
	if err != nil {
		return fmt.Errorf("decide: load step %d: %w", stepID, err)
	}
	if step.Status != "awaiting" {
		// Idempotency: a second decision must not re-run the risky tool.
		return ErrStepResolved
	}
	if step.ToolUseID == "" {
		return fmt.Errorf("decide: step %d has no tool_use id", stepID)
	}
	checkpointID := step.CheckpointID.Int64
	if !step.CheckpointID.Valid || checkpointID <= 0 {
		return fmt.Errorf("decide: step %d has no checkpoint", stepID)
	}

	conv, err := a.store.GetConversationByMessage(ctx, step.MessageID)
	if err != nil {
		return fmt.Errorf("decide: load conversation for message %d: %w", step.MessageID, err)
	}
	convID := conv.ID

	if err := a.resolveStep(ctx, convID, step, allow); err != nil {
		// The tool_result feed (or status update) failed: the resume window
		// would be corrupt. Publish, commit the checkpoint, and bail without
		// resuming the loop into a gapped history.
		a.events.Publish(convID, Event{Type: "error", Data: "failed to resolve permission decision"})
		if cErr := a.commitCheckpoint(ctx, checkpointID); cErr != nil {
			return fmt.Errorf("decide: resolve step %d: %w (commit: %v)", stepID, err, cErr)
		}
		return fmt.Errorf("decide: resolve step %d: %w", stepID, err)
	}

	// Resume the SAME loop: the tool_result is now the last message in history,
	// so Execute continues under auto tool choice from the suspend point.
	if err := a.Execute(ctx, convID, checkpointID, step.MessageID); err != nil {
		return fmt.Errorf("decide: resume execute: %w", err)
	}
	return nil
}

// resolveStep applies the decision to the awaiting step: it runs (allow) or
// denies (deny) the risky tool, marks the step terminal, and feeds the matching
// tool_result. Any persistence/feed error is returned so Decide aborts before
// resuming into a corrupt window.
func (a *Agent) resolveStep(ctx context.Context, convID int64, step gen.AgentStep, allow bool) error {
	checkpointID := step.CheckpointID.Int64

	if !allow {
		if err := a.store.UpdateStepStatus(ctx, step.ID, "denied", ""); err != nil {
			return fmt.Errorf("mark denied: %w", err)
		}
		if err := a.feedToolError(ctx, convID, step.ToolUseID, "user denied"); err != nil {
			return fmt.Errorf("feed denial: %w", err)
		}
		return nil
	}

	tool, ok := a.reg.Get(step.ToolName)
	if !ok {
		if err := a.store.UpdateStepStatus(ctx, step.ID, "error", ""); err != nil {
			return fmt.Errorf("mark error (unknown tool): %w", err)
		}
		if err := a.feedToolError(ctx, convID, step.ToolUseID, fmt.Sprintf("unknown tool %q", step.ToolName)); err != nil {
			return fmt.Errorf("feed unknown tool: %w", err)
		}
		return nil
	}

	// This is where the approved risky write actually happens — under the
	// captured checkpoint so the tool records its change for revert.
	res, runErr := tool.Handler(withCheckpoint(ctx, checkpointID), json.RawMessage(step.PendingInput))
	if runErr != nil {
		if err := a.store.UpdateStepStatus(ctx, step.ID, "error", runErr.Error()); err != nil {
			return fmt.Errorf("mark error: %w", err)
		}
		if err := a.feedToolError(ctx, convID, step.ToolUseID, runErr.Error()); err != nil {
			return fmt.Errorf("feed handler error: %w", err)
		}
		return nil
	}

	if err := a.store.UpdateStepStatus(ctx, step.ID, "done", encodeResultJSON(res.JSON)); err != nil {
		return fmt.Errorf("mark done: %w", err)
	}
	if err := a.feedToolResult(ctx, convID, step.ToolUseID, res, false); err != nil {
		return fmt.Errorf("feed result: %w", err)
	}
	return nil
}
