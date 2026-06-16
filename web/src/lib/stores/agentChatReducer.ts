/**
 * Pure, framework-free turn-state reducer for the agent chat.
 *
 * Lives in a plain .ts module (not the .svelte.ts store) so it can be unit
 * tested under vitest without the Svelte runes compiler. The runes store
 * (agentChat.svelte.ts) re-exports everything here.
 *
 * `applyEvent` is PURE: it returns a NEW TurnState for every call and never
 * mutates its input. Side effects (network, status changes) live in the store.
 */

import type { AgentEvent, PlanStep } from '$lib/agent/events';
import type { PlanStepDTO } from '$lib/api/agent';

/** Normalize the event's free-string risk into the DTO's narrowed union. */
function normalizeRisk(risk: string): PlanStepDTO['risk'] {
	return risk === 'risky' || risk === 'meta' ? risk : 'read';
}

/** Project a stream PlanStep into the DTO shape used by the turn/UI. */
function toPlanStep(s: PlanStep): PlanStepDTO {
	return { tool: s.tool, summary: s.summary, risk: normalizeRisk(s.risk) };
}

/** A single tool result projected into a view-friendly shape. */
export interface ToolResultView {
	toolUseId: string;
	render?: string;
	result?: unknown;
	error?: string;
	isError: boolean;
}

/** A pending human-approval request distilled from an access_request event. */
export interface AccessRequestInfo {
	stepId: number;
	toolName: string;
	summary: string;
	input: unknown;
	expiresAt: string;
}

/** Accumulated state for the in-flight agent turn (one user→assistant cycle). */
export interface TurnState {
	plan?: PlanStepDTO[];
	toolResults: ToolResultView[];
	pendingAccess?: AccessRequestInfo | null;
	finalText?: string;
}

/** A fresh, empty turn. Always returns a new object. */
export function emptyTurn(): TurnState {
	return { toolResults: [] };
}

/**
 * Fold one AgentEvent into the turn state, returning a NEW TurnState.
 *
 * `error` and `budget_exceeded` are status-only concerns handled by the store;
 * the reducer returns the turn unchanged (same reference) for those.
 */
export function applyEvent(turn: TurnState, ev: AgentEvent): TurnState {
	if (turn === null || typeof turn !== 'object') {
		throw new Error('applyEvent: turn must be a TurnState object');
	}
	if (ev === null || typeof ev !== 'object') {
		throw new Error('applyEvent: ev must be an AgentEvent object');
	}

	switch (ev.type) {
		case 'plan':
			return { ...turn, plan: ev.steps.map(toPlanStep) };

		case 'tool_result': {
			const view: ToolResultView = ev.isError
				? { toolUseId: ev.toolUseId, error: ev.error, isError: true }
				: {
						toolUseId: ev.toolUseId,
						render: ev.render,
						result: ev.result,
						isError: false
					};
			return { ...turn, toolResults: [...turn.toolResults, view] };
		}

		case 'access_request':
			return {
				...turn,
				pendingAccess: {
					stepId: ev.stepId,
					toolName: ev.toolName,
					summary: ev.summary,
					input: ev.input,
					expiresAt: ev.expiresAt
				}
			};

		case 'step_expired':
			if (turn.pendingAccess != null && turn.pendingAccess.stepId === ev.stepId) {
				return { ...turn, pendingAccess: null };
			}
			return turn;

		case 'message_final':
			return { ...turn, finalText: ev.text };

		case 'error':
		case 'budget_exceeded':
			// Status/error are tracked by the store, not the turn.
			return turn;

		default:
			return turn;
	}
}
