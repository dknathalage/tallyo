/**
 * Per-conversation agent SSE stream client.
 *
 * The backend endpoint is GET /api/agent/conversations/{id}/stream.
 * Each SSE frame's `data` is a JSON object: {type: string, data: unknown}.
 * We normalize to AgentEvent (discriminated union) and call onEvent.
 *
 * Design mirrors web/src/lib/realtime/events.ts:
 * - Guard typeof window before touching EventSource (SSR-safe)
 * - Do NOT close on error; the browser auto-reconnects
 * - Call onOpen on "open" so callers can resync (no server-side replay)
 */

import type { AgentEvent, PlanStep } from './events';

/** Raw wire frame as sent by the Go backend: {type, data}. */
interface WireFrame {
	type: string;
	data: unknown;
}

/**
 * Parse a raw SSE `event.data` string into a normalized AgentEvent.
 * Pure and total — never throws; returns null on any invalid input.
 */
export function parseAgentFrame(raw: string): AgentEvent | null {
	if (typeof raw !== 'string' || raw.length === 0) return null;

	let frame: WireFrame;
	try {
		const parsed: unknown = JSON.parse(raw);
		if (parsed === null || typeof parsed !== 'object' || Array.isArray(parsed)) return null;
		const obj = parsed as Record<string, unknown>;
		if (typeof obj.type !== 'string') return null;
		frame = { type: obj.type, data: obj.data };
	} catch {
		// Malformed JSON — ignore.
		return null;
	}

	const { type, data } = frame;

	switch (type) {
		case 'plan': {
			if (!Array.isArray(data)) return null;
			// Bounded by the number of steps in the plan; safe to iterate fully.
			const steps: PlanStep[] = [];
			for (const item of data) {
				if (item === null || typeof item !== 'object' || Array.isArray(item)) return null;
				const s = item as Record<string, unknown>;
				if (typeof s.tool !== 'string') return null;
				if (typeof s.summary !== 'string') return null;
				if (typeof s.risk !== 'string') return null;
				steps.push({ tool: s.tool, summary: s.summary, risk: s.risk });
			}
			return { type: 'plan', steps };
		}

		case 'tool_result': {
			if (data === null || typeof data !== 'object' || Array.isArray(data)) return null;
			const d = data as Record<string, unknown>;
			if (typeof d.toolUseId !== 'string') return null;
			if (d.isError === true) {
				if (typeof d.error !== 'string') return null;
				return { type: 'tool_result', toolUseId: d.toolUseId, error: d.error, isError: true };
			}
			if (d.isError === false) {
				if (typeof d.render !== 'string') return null;
				return {
					type: 'tool_result',
					toolUseId: d.toolUseId,
					render: d.render,
					result: d.result,
					isError: false
				};
			}
			return null;
		}

		case 'access_request': {
			if (data === null || typeof data !== 'object' || Array.isArray(data)) return null;
			const d = data as Record<string, unknown>;
			if (typeof d.stepId !== 'number') return null;
			if (typeof d.toolName !== 'string') return null;
			if (typeof d.toolUseId !== 'string') return null;
			if (typeof d.summary !== 'string') return null;
			if (typeof d.expiresAt !== 'string') return null;
			return {
				type: 'access_request',
				stepId: d.stepId,
				toolName: d.toolName,
				toolUseId: d.toolUseId,
				summary: d.summary,
				input: d.input,
				expiresAt: d.expiresAt
			};
		}

		case 'message_final': {
			if (typeof data !== 'string') return null;
			return { type: 'message_final', text: data };
		}

		case 'error': {
			if (typeof data !== 'string') return null;
			return { type: 'error', message: data };
		}

		case 'budget_exceeded': {
			if (typeof data !== 'string') return null;
			return { type: 'budget_exceeded', message: data };
		}

		case 'step_expired': {
			if (data === null || typeof data !== 'object' || Array.isArray(data)) return null;
			const d = data as Record<string, unknown>;
			if (typeof d.stepId !== 'number') return null;
			if (typeof d.toolName !== 'string') return null;
			return { type: 'step_expired', stepId: d.stepId, toolName: d.toolName };
		}

		default:
			return null;
	}
}

export interface StreamHandlers {
	onEvent: (event: AgentEvent) => void;
	onOpen?: () => void;
}

export interface StreamHandle {
	close(): void;
}

/**
 * Open a per-conversation agent SSE stream.
 *
 * Returns a handle with a `close()` method. On SSR (typeof window undefined)
 * returns a no-op handle immediately.
 *
 * Reconnection is handled by the browser automatically; we do NOT close on
 * error. onOpen is called on every (re)connect so the caller can resync state.
 */
export function openAgentStream(convId: number, handlers: StreamHandlers): StreamHandle {
	if (typeof convId !== 'number' || convId <= 0) {
		throw new Error(`openAgentStream: convId must be a positive integer, got ${convId}`);
	}
	if (typeof handlers.onEvent !== 'function') {
		throw new Error('openAgentStream: handlers.onEvent must be a function');
	}

	// SSR guard — EventSource is browser-only.
	if (typeof window === 'undefined') {
		return { close() {} };
	}

	const url = `/api/agent/conversations/${convId}/stream`;
	const es = new EventSource(url, { withCredentials: true });

	es.addEventListener('open', () => {
		// Called on initial connect and every auto-reconnect — let caller resync.
		if (typeof handlers.onOpen === 'function') {
			handlers.onOpen();
		}
	});

	es.addEventListener('message', (event: MessageEvent) => {
		if (typeof event.data !== 'string' || event.data.length === 0) return;
		const agentEvent = parseAgentFrame(event.data);
		if (agentEvent !== null) {
			handlers.onEvent(agentEvent);
		}
	});

	// On error we intentionally do NOT close: the browser auto-reconnects and
	// the next "open" event triggers an onOpen resync.

	return {
		close() {
			es.close();
		}
	};
}
