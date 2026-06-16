import { describe, it, expect } from 'vitest';
import { parseAgentFrame } from './stream';

describe('parseAgentFrame', () => {
	it('parses a plan frame into {type:"plan", steps:[...]}', () => {
		const raw = JSON.stringify({
			type: 'plan',
			data: [{ tool: 'x', summary: 's', risk: 'read' }]
		});
		const result = parseAgentFrame(raw);
		expect(result).toEqual({
			type: 'plan',
			steps: [{ tool: 'x', summary: 's', risk: 'read' }]
		});
	});

	it('parses a success tool_result frame (isError false)', () => {
		const raw = JSON.stringify({
			type: 'tool_result',
			data: { toolUseId: 't', render: 'table', result: [1, 2], isError: false }
		});
		const result = parseAgentFrame(raw);
		expect(result).toEqual({
			type: 'tool_result',
			toolUseId: 't',
			render: 'table',
			result: [1, 2],
			isError: false
		});
	});

	it('parses an error tool_result frame (isError true)', () => {
		const raw = JSON.stringify({
			type: 'tool_result',
			data: { toolUseId: 'u', error: 'boom', isError: true }
		});
		const result = parseAgentFrame(raw);
		expect(result).toEqual({
			type: 'tool_result',
			toolUseId: 'u',
			error: 'boom',
			isError: true
		});
	});

	it('parses a message_final frame — string data becomes {type,text}', () => {
		const raw = JSON.stringify({ type: 'message_final', data: 'hello' });
		const result = parseAgentFrame(raw);
		expect(result).toEqual({ type: 'message_final', text: 'hello' });
	});

	it('parses an error frame — string data becomes {type,message}', () => {
		const raw = JSON.stringify({ type: 'error', data: 'something went wrong' });
		const result = parseAgentFrame(raw);
		expect(result).toEqual({ type: 'error', message: 'something went wrong' });
	});

	it('parses a budget_exceeded frame — string data becomes {type,message}', () => {
		const raw = JSON.stringify({ type: 'budget_exceeded', data: 'token budget exceeded' });
		const result = parseAgentFrame(raw);
		expect(result).toEqual({ type: 'budget_exceeded', message: 'token budget exceeded' });
	});

	it('parses an access_request frame with all fields including numeric stepId', () => {
		const raw = JSON.stringify({
			type: 'access_request',
			data: {
				stepId: 3,
				toolName: 'query_db',
				toolUseId: 'tu-42',
				summary: 'runs a select',
				input: { sql: 'SELECT 1' },
				expiresAt: '2026-01-01T00:00:00Z'
			}
		});
		const result = parseAgentFrame(raw);
		expect(result).toEqual({
			type: 'access_request',
			stepId: 3,
			toolName: 'query_db',
			toolUseId: 'tu-42',
			summary: 'runs a select',
			input: { sql: 'SELECT 1' },
			expiresAt: '2026-01-01T00:00:00Z'
		});
	});

	it('parses a step_expired frame with numeric stepId', () => {
		const raw = JSON.stringify({
			type: 'step_expired',
			data: { stepId: 7, toolName: 'run_code' }
		});
		const result = parseAgentFrame(raw);
		expect(result).toEqual({ type: 'step_expired', stepId: 7, toolName: 'run_code' });
	});

	it('returns null for malformed JSON', () => {
		expect(parseAgentFrame('not json{')).toBeNull();
	});

	it('returns null for unknown type', () => {
		const raw = JSON.stringify({ type: 'unknown_type', data: {} });
		expect(parseAgentFrame(raw)).toBeNull();
	});

	it('returns null when type field is missing', () => {
		const raw = JSON.stringify({ data: 'hello' });
		expect(parseAgentFrame(raw)).toBeNull();
	});

	it('returns null when type is not a string', () => {
		const raw = JSON.stringify({ type: 42, data: 'hello' });
		expect(parseAgentFrame(raw)).toBeNull();
	});

	it('returns null for plan when data is not an array', () => {
		const raw = JSON.stringify({ type: 'plan', data: 'not-an-array' });
		expect(parseAgentFrame(raw)).toBeNull();
	});

	it('returns null for message_final when data is not a string', () => {
		const raw = JSON.stringify({ type: 'message_final', data: 123 });
		expect(parseAgentFrame(raw)).toBeNull();
	});

	it('returns null for access_request when stepId is not a number', () => {
		const raw = JSON.stringify({
			type: 'access_request',
			data: {
				stepId: 'not-a-number',
				toolName: 'x',
				toolUseId: 'y',
				summary: 'z',
				input: {},
				expiresAt: '2026-01-01T00:00:00Z'
			}
		});
		expect(parseAgentFrame(raw)).toBeNull();
	});

	it('returns null for tool_result when toolUseId is missing', () => {
		const raw = JSON.stringify({
			type: 'tool_result',
			data: { render: 'table', result: [], isError: false }
		});
		expect(parseAgentFrame(raw)).toBeNull();
	});
});
