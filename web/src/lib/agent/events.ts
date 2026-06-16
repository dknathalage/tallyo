/**
 * Discriminated union of all agent SSE event types.
 *
 * The backend wraps every frame as {type, data}. These types represent the
 * NORMALIZED shape after unwrapping — i.e. what callers work with directly,
 * not the raw wire format.
 */

/** One step in an agent plan — mirrors PlanStepDTO from api/agent.ts. */
export interface PlanStep {
	tool: string;
	summary: string;
	risk: string;
}

/**
 * tool_result has two variants distinguished by `isError`.
 * Use `if (event.isError)` to narrow to the error branch.
 */
export type ToolResultEvent =
	| { type: 'tool_result'; toolUseId: string; render: string; result: unknown; isError: false }
	| { type: 'tool_result'; toolUseId: string; error: string; isError: true };

export type AgentEvent =
	| { type: 'plan'; steps: PlanStep[] }
	| ToolResultEvent
	| {
			type: 'access_request';
			stepId: number;
			toolName: string;
			toolUseId: string;
			summary: string;
			input: unknown;
			expiresAt: string;
	  }
	| { type: 'message_final'; text: string }
	| { type: 'error'; message: string }
	| { type: 'budget_exceeded'; message: string }
	| { type: 'step_expired'; stepId: number; toolName: string };
