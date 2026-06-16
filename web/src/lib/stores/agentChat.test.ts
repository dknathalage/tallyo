import { describe, it, expect } from 'vitest';
import { applyEvent, emptyTurn, type TurnState } from './agentChatReducer';
import type { AgentEvent } from '$lib/agent/events';

describe('applyEvent reducer', () => {
	it('starts from an empty turn', () => {
		const turn = emptyTurn();
		expect(turn.plan).toBeUndefined();
		expect(turn.toolResults).toEqual([]);
		expect(turn.pendingAccess == null).toBe(true);
		expect(turn.finalText).toBeUndefined();
	});

	it('drives a full event sequence through to message_final', () => {
		let turn: TurnState = emptyTurn();

		// plan event -> turn.plan set
		const planEv: AgentEvent = {
			type: 'plan',
			steps: [{ tool: 'search', summary: 'find rows', risk: 'read' }]
		};
		turn = applyEvent(turn, planEv);
		expect(turn.plan).toBeDefined();
		expect(turn.plan).toHaveLength(1);
		expect(turn.plan?.[0].tool).toBe('search');

		// tool_result success -> one entry, isError false, with result
		const okEv: AgentEvent = {
			type: 'tool_result',
			toolUseId: 'tu-1',
			render: 'rendered text',
			result: { rows: 3 },
			isError: false
		};
		turn = applyEvent(turn, okEv);
		expect(turn.toolResults).toHaveLength(1);
		expect(turn.toolResults[0].isError).toBe(false);
		expect(turn.toolResults[0].toolUseId).toBe('tu-1');
		expect(turn.toolResults[0].render).toBe('rendered text');
		expect(turn.toolResults[0].result).toEqual({ rows: 3 });
		expect(turn.toolResults[0].error).toBeUndefined();

		// access_request -> pendingAccess set with stepId
		const accessEv: AgentEvent = {
			type: 'access_request',
			stepId: 42,
			toolName: 'delete_invoice',
			toolUseId: 'tu-2',
			summary: 'delete invoice #7',
			input: { id: 7 },
			expiresAt: '2026-06-16T00:00:00Z'
		};
		turn = applyEvent(turn, accessEv);
		expect(turn.pendingAccess).not.toBeNull();
		expect(turn.pendingAccess?.stepId).toBe(42);
		expect(turn.pendingAccess?.toolName).toBe('delete_invoice');
		expect(turn.pendingAccess?.summary).toBe('delete invoice #7');
		expect(turn.pendingAccess?.input).toEqual({ id: 7 });
		expect(turn.pendingAccess?.expiresAt).toBe('2026-06-16T00:00:00Z');

		// step_expired for that stepId -> pendingAccess cleared
		const expiredEv: AgentEvent = { type: 'step_expired', stepId: 42, toolName: 'delete_invoice' };
		turn = applyEvent(turn, expiredEv);
		expect(turn.pendingAccess == null).toBe(true);

		// another tool_result error -> second entry, isError true, with error msg
		const errEv: AgentEvent = {
			type: 'tool_result',
			toolUseId: 'tu-3',
			error: 'boom',
			isError: true
		};
		turn = applyEvent(turn, errEv);
		expect(turn.toolResults).toHaveLength(2);
		expect(turn.toolResults[1].isError).toBe(true);
		expect(turn.toolResults[1].error).toBe('boom');
		expect(turn.toolResults[1].render).toBeUndefined();
		expect(turn.toolResults[1].result).toBeUndefined();

		// message_final -> finalText set
		const finalEv: AgentEvent = { type: 'message_final', text: 'all done' };
		turn = applyEvent(turn, finalEv);
		expect(turn.finalText).toBe('all done');
	});

	it('does not clear pendingAccess when step_expired is for a different step', () => {
		let turn: TurnState = emptyTurn();
		turn = applyEvent(turn, {
			type: 'access_request',
			stepId: 1,
			toolName: 'x',
			toolUseId: 'tu',
			summary: 's',
			input: null,
			expiresAt: 'e'
		});
		turn = applyEvent(turn, { type: 'step_expired', stepId: 999, toolName: 'x' });
		expect(turn.pendingAccess?.stepId).toBe(1);
	});

	it('returns turn unchanged for error and budget_exceeded events', () => {
		const turn = emptyTurn();
		const afterError = applyEvent(turn, { type: 'error', message: 'nope' });
		expect(afterError).toBe(turn);
		const afterBudget = applyEvent(turn, { type: 'budget_exceeded', message: 'limit' });
		expect(afterBudget).toBe(turn);
	});

	it('is immutable: returns a new object and does not mutate the input', () => {
		const turn = emptyTurn();
		const snapshot = JSON.stringify(turn);

		const next = applyEvent(turn, {
			type: 'tool_result',
			toolUseId: 'tu-1',
			render: 'r',
			result: 1,
			isError: false
		});

		// new object identity
		expect(next).not.toBe(turn);
		// input untouched
		expect(turn.toolResults).toHaveLength(0);
		expect(JSON.stringify(turn)).toBe(snapshot);
		// the toolResults array itself is a fresh array, not the same reference
		expect(next.toolResults).not.toBe(turn.toolResults);
	});
});
