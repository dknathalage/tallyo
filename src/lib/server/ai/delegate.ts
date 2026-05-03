import { LlamaChatSession } from 'node-llama-cpp';
import type { LlamaContext } from 'node-llama-cpp';
import { defineChatSessionFunction } from 'node-llama-cpp';
import { listTools } from './tools.js';
import type { ToolSpec } from './tools.js';
import { getSkill } from './skills.js';
import type { SubAgent } from './agents.js';
import { wrapTool } from './agent.js';
import type { AgentEventFn, ToolCallRecord } from './agent.js';
import { log } from '../logger.js';

const l = log('ai:delegate');

const DEFAULT_MAX_STEPS = 6;

export interface RunSubAgentOpts {
	agent: SubAgent;
	input: Record<string, unknown>;
	sessionId: number;
	parentContext: LlamaContext;
	pendingWrites: ToolCallRecord[];
	emit: AgentEventFn;
}

export interface SubAgentResult {
	agent_id: string;
	summary: string;
	queued: string[];
	steps: number;
	stopped_reason: 'completed' | 'max_steps' | 'pending_write' | 'no_progress';
}

export async function runSubAgent(opts: RunSubAgentOpts): Promise<SubAgentResult> {
	const { agent, input, sessionId, parentContext, pendingWrites, emit } = opts;
	const maxSteps = agent.maxSteps ?? DEFAULT_MAX_STEPS;
	const initialPendingCount = pendingWrites.length;

	l.info('subagent start', { agent: agent.id, sessionId, input });
	emit({ type: 'subagent_started', agent_id: agent.id, title: agent.title, input });

	const sequence = parentContext.getSequence();
	const subSession = new LlamaChatSession({
		contextSequence: sequence,
		systemPrompt: agent.systemPrompt
	});

	const allowedToolNames = new Set<string>();
	for (const skillId of agent.skills) {
		const skill = getSkill(skillId);
		if (!skill) continue;
		for (const t of skill.tools) {
			if (t.startsWith('delegateTo')) continue;
			if (t === 'loadSkill' || t === 'listAvailableSkills') continue;
			allowedToolNames.add(t);
		}
	}
	const specByName = new Map<string, ToolSpec>(listTools().map((t) => [t.name, t]));

	let summary = '';
	let stopReason: SubAgentResult['stopped_reason'] = 'completed';
	let stepsTaken = 0;

	for (let step = 0; step < maxSteps; step++) {
		stepsTaken = step + 1;
		const stopController = new AbortController();
		const readToolsCalled = new Set<string>();
		const functions: Record<string, ReturnType<typeof defineChatSessionFunction>> = {};
		for (const name of allowedToolNames) {
			const spec = specByName.get(name);
			if (!spec) continue;
			functions[name] = wrapTool({
				spec,
				sessionId,
				pendingWrites,
				readToolsCalled,
				emit,
				stopController,
				agentId: agent.id
			});
		}

		const turnText = step === 0 ? `Task: ${JSON.stringify(input)}` : 'Continue. If the task is complete, summarize and stop.';

		let stepText = '';
		try {
			await subSession.prompt(turnText, {
				functions,
				maxTokens: 768,
				signal: stopController.signal,
				stopOnAbortSignal: true,
				onTextChunk: (chunk: string) => {
					stepText += chunk;
					emit({ type: 'subagent_token', agent_id: agent.id, text: chunk });
				}
			});
		} catch (e) {
			const message = e instanceof Error ? e.message : 'sub-agent failed';
			summary = stepText.trim() || summary;
			const writesQueued = pendingWrites.length > initialPendingCount;
			const aborted =
				stopController.signal.aborted ||
				(e instanceof Error && e.name === 'AbortError');
			if (aborted && writesQueued) {
				stopReason = 'pending_write';
				break;
			}
			if (aborted) {
				stopReason = 'completed';
				break;
			}
			l.error('subagent step failed', { agent: agent.id, step, error: message });
			emit({
				type: 'subagent_done',
				agent_id: agent.id,
				error: message
			});
			stopReason = 'no_progress';
			break;
		}
		summary = stepText.trim() || summary;

		const newWritesQueued = pendingWrites.length > initialPendingCount;
		if (newWritesQueued) {
			stopReason = 'pending_write';
			break;
		}
		if (!stepText.trim()) {
			stopReason = 'no_progress';
			break;
		}
		// If the model produced text but didn't call any tool, treat as completion.
		if (readToolsCalled.size === 0) {
			stopReason = 'completed';
			break;
		}
		if (step === maxSteps - 1) stopReason = 'max_steps';
	}

	const queuedFromAgent = pendingWrites
		.slice(initialPendingCount)
		.filter((p) => p.agent_id === agent.id)
		.map((p) => p.uuid);

	l.info('subagent done', { agent: agent.id, steps: stepsTaken, stopped_reason: stopReason, queued: queuedFromAgent.length });
	emit({
		type: 'subagent_done',
		agent_id: agent.id,
		summary,
		queued: queuedFromAgent,
		steps: stepsTaken,
		stopped_reason: stopReason
	});

	return {
		agent_id: agent.id,
		summary: summary || `Sub-agent ${agent.id} produced no output (${stopReason}).`,
		queued: queuedFromAgent,
		steps: stepsTaken,
		stopped_reason: stopReason
	};
}
