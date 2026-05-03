import { createSession, defineChatSessionFunction } from './llm.js';
import type { AiProgressFn } from './llm.js';
import { listTools, getTool } from './tools.js';
import type { ToolSpec } from './tools.js';
import { listSkills, resolveSkillsForContext, getSkill } from './skills.js';
import type { Skill } from './skills.js';
import { makeMetaTools } from './meta-tools.js';
import { listAgents, agentToolName } from './agents.js';
import type { SubAgent } from './agents.js';
import { runSubAgent } from './delegate.js';
import { repositories } from '$lib/repositories/index.js';
import type { AiChatMessage } from '$lib/db/queries/ai-chat.js';
import type { ChatHistoryItem, ChatModelResponse, LlamaContext } from 'node-llama-cpp';
import { log } from '../logger.js';

const l = log('ai:agent');

const BASE_SYSTEM_PROMPT = `You are Tallyo's AI assistant for a self-hosted invoice manager.

Rules:
- Always read before writing. Never invent ids, names, or numbers — call a search/get tool first.
- Do ONE thing at a time. Call at most one write tool per turn, then stop and wait for the user to approve or reject.
- If a tool call was approved and succeeded earlier in this conversation, the action is COMPLETE. Never call the same tool with the same arguments again. Tell the user it is done.
- For yes/no or multiple-choice, call askUserChoice.
- For complex multi-step domain tasks, prefer delegating to a specialized agent.
- If you need a capability not currently available, call loadSkill({id}) and stop — you will be re-prompted with the new tools.

Keep replies concise.`;

export interface AgentEvent {
	type:
		| 'token'
		| 'tool_call_started'
		| 'tool_call_succeeded'
		| 'tool_call_failed'
		| 'tool_pending'
		| 'quick_reply'
		| 'message_complete'
		| 'skill_loaded'
		| 'skills_resolved'
		| 'subagent_started'
		| 'subagent_token'
		| 'subagent_tool_started'
		| 'subagent_tool_succeeded'
		| 'subagent_tool_failed'
		| 'subagent_done'
		| 'error'
		| 'done';
	[key: string]: unknown;
}

export type AgentEventFn = (e: AgentEvent) => void;

export interface ToolCallRecord {
	uuid: string;
	tool_name: string;
	args: Record<string, unknown>;
	agent_id?: string;
}

const REQUIRED_READS: Record<string, string[]> = {
	createClient: ['searchClients'],
	updateClient: ['getClient', 'searchClients'],
	deleteClient: ['getClient', 'searchClients'],
	bulkDeleteClients: ['searchClients', 'getClient'],
	createInvoice: ['searchClients', 'getClient', 'listInvoices', 'getInvoice'],
	updateInvoiceStatus: ['getInvoice', 'listInvoices'],
	deleteInvoice: ['getInvoice', 'listInvoices'],
	duplicateInvoice: ['getInvoice', 'listInvoices'],
	bulkDeleteInvoices: ['listInvoices', 'getInvoice'],
	bulkUpdateInvoiceStatus: ['listInvoices', 'getInvoice'],
	createEstimate: ['searchClients', 'getClient', 'listEstimates', 'getEstimate'],
	updateEstimateStatus: ['getEstimate', 'listEstimates'],
	deleteEstimate: ['getEstimate', 'listEstimates'],
	convertEstimateToInvoice: ['getEstimate', 'listEstimates'],
	recordPayment: ['getInvoice', 'getInvoicePayments', 'getInvoiceTotalPaid'],
	deletePayment: ['getInvoicePayments'],
	createCatalogItem: ['searchCatalog'],
	updateCatalogItem: ['getCatalogItem', 'searchCatalog'],
	deleteCatalogItem: ['getCatalogItem', 'searchCatalog'],
	createPayer: ['listPayers'],
	updatePayer: ['getPayer', 'listPayers'],
	deletePayer: ['getPayer', 'listPayers'],
	createTaxRate: ['listTaxRates'],
	updateTaxRate: ['listTaxRates'],
	deleteTaxRate: ['listTaxRates'],
	createRateTier: ['listRateTiers'],
	deleteRateTier: ['listRateTiers'],
	deleteRecurringTemplate: ['listRecurringTemplates', 'getRecurringTemplate'],
	runRecurringTemplate: ['listRecurringTemplates', 'getRecurringTemplate'],
	saveBusinessProfile: ['getBusinessProfile']
};

function buildHistoryFromMessages(messages: AiChatMessage[], systemPrompt: string): ChatHistoryItem[] {
	const items: ChatHistoryItem[] = [{ type: 'system', text: systemPrompt }];
	for (const m of messages) {
		if (m.role === 'user') {
			items.push({ type: 'user', text: m.content });
			continue;
		}
		if (m.role === 'assistant') {
			const response: ChatModelResponse['response'] = [];
			if (m.content) response.push(m.content);
			items.push({ type: 'model', response });
			continue;
		}
		if (m.role === 'system' && m.content) {
			items.push({ type: 'system', text: m.content });
		}
	}
	return items;
}

function makeDelegationToolSpecs(
	parentContext: LlamaContext,
	sessionId: number,
	pendingWrites: ToolCallRecord[],
	emit: AgentEventFn
): ToolSpec[] {
	return listAgents().map((agent: SubAgent) => ({
		name: agentToolName(agent.id),
		description: agent.description,
		kind: 'read' as const,
		paramSchema: agent.inputSchema,
		execute: async (args) =>
			runSubAgent({
				agent,
				input: args,
				sessionId,
				parentContext,
				pendingWrites,
				emit
			})
	}));
}

function buildSystemPrompt(
	loadedSkills: Skill[],
	allSkills: Skill[],
	routeContext?: string
): string {
	const sections: string[] = [BASE_SYSTEM_PROMPT];
	if (routeContext) sections.push(`Current page: ${routeContext}`);

	const loadedDescriptions = loadedSkills.map((s) => {
		const addendum = s.promptAddendum ? ` ${s.promptAddendum}` : '';
		return `- ${s.id} (${s.title}): ${s.description}${addendum}`;
	});
	sections.push(`## Loaded skills\n${loadedDescriptions.join('\n')}`);

	const loadedIds = new Set(loadedSkills.map((s) => s.id));
	const otherIds = allSkills.filter((s) => !loadedIds.has(s.id)).map((s) => s.id);
	if (otherIds.length > 0) {
		sections.push(
			`## Other capabilities (call loadSkill({id}) to enable)\n${otherIds.join(', ')}`
		);
	}
	return sections.join('\n\n');
}

export interface RunAgentOpts {
	sessionId: number;
	userMessage: string;
	routeContext?: string;
	emit: AgentEventFn;
	onProgress?: AiProgressFn;
	continuation?: boolean;
}

export async function runAgentTurn(opts: RunAgentOpts): Promise<void> {
	const { sessionId, userMessage, routeContext, emit, onProgress, continuation } = opts;
	l.info('turn start', {
		sessionId,
		continuation: continuation === true,
		route: routeContext,
		userMessageLen: userMessage.length
	});
	if (!continuation) {
		if (!userMessage.trim()) throw new Error('user message empty');
		if (userMessage.length > 8000) throw new Error('user message too long');
	}

	const repo = repositories.aiChat;
	if (!continuation) {
		await repo.appendMessage({ session_id: sessionId, role: 'user', content: userMessage });
	}

	const explicitlyLoaded = await repo.getLoadedSkills(sessionId);
	const resolveOpts: Parameters<typeof resolveSkillsForContext>[0] = {
		userMessage: continuation ? '' : userMessage,
		explicitlyLoaded
	};
	if (routeContext) resolveOpts.route = routeContext;
	const loadedSkills = resolveSkillsForContext(resolveOpts);
	const loadedSkillSet = new Set(loadedSkills.map((s) => s.id));
	l.info('skills resolved', { loaded: Array.from(loadedSkillSet) });
	emit({ type: 'skills_resolved', loaded: loadedSkills.map((s) => s.id) });

	const dynamicSystemPrompt = buildSystemPrompt(loadedSkills, listSkills(), routeContext);
	const history = buildHistoryFromMessages(await repo.listMessages(sessionId), dynamicSystemPrompt);

	const { session, context } = await createSession(dynamicSystemPrompt, onProgress);
	try {
		await session.setChatHistory(history.slice(0, -1));

		const pendingWrites: ToolCallRecord[] = [];
		const readToolsCalled = new Set<string>();
		const stopController = new AbortController();

		const explicitlyLoadedSet = new Set(explicitlyLoaded);
		const metaCtx = {
			sessionId,
			loadedSkills: explicitlyLoadedSet,
			persistLoadedSkills: async (ids: string[]) => repo.setLoadedSkills(sessionId, ids),
			onSkillLoaded: (skillId: string) => emit({ type: 'skill_loaded', skill_id: skillId }),
			stopController
		};

		const allowedToolNames = new Set(loadedSkills.flatMap((s) => s.tools));
		const allSpecs: ToolSpec[] = [
			...listTools(),
			...makeMetaTools(metaCtx),
			...makeDelegationToolSpecs(context, sessionId, pendingWrites, emit)
		];
		const specByName = new Map(allSpecs.map((s) => [s.name, s]));

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
				stopController
			});
		}

		const lastUser = history[history.length - 1];
		const userText = continuation
			? 'Continue based on the latest tool result above. If the task is complete, summarize briefly and stop.'
			: lastUser?.type === 'user'
				? lastUser.text
				: userMessage;

		let assistantText = '';
		await session.prompt(userText, {
			functions,
			maxTokens: 1024,
			signal: stopController.signal,
			stopOnAbortSignal: true,
			onTextChunk: (chunk: string) => {
				assistantText += chunk;
				emit({ type: 'token', text: chunk });
			}
		});

		await repo.appendMessage({ session_id: sessionId, role: 'assistant', content: assistantText });
		l.info('turn complete', {
			sessionId,
			assistantLen: assistantText.length,
			pendingWrites: pendingWrites.length,
			readToolsCalled: Array.from(readToolsCalled)
		});
		emit({ type: 'message_complete', content: assistantText });

		for (const pw of pendingWrites) {
			emit({
				type: 'tool_pending',
				uuid: pw.uuid,
				tool_name: pw.tool_name,
				args: pw.args,
				agent_id: pw.agent_id
			});
		}
		emit({ type: 'done', loaded_skills: Array.from(loadedSkillSet) });
	} catch (e) {
		l.error('turn failed', {
			sessionId,
			error: e instanceof Error ? e.message : String(e),
			stack: e instanceof Error ? e.stack : undefined
		});
		throw e;
	} finally {
		await context.dispose();
	}
}

export interface WrapToolOpts {
	spec: ToolSpec;
	sessionId: number;
	pendingWrites: ToolCallRecord[];
	readToolsCalled: Set<string>;
	emit: AgentEventFn;
	stopController: AbortController;
	agentId?: string;
	parentToolCallUuid?: string;
}

export function wrapTool(opts: WrapToolOpts): ReturnType<typeof defineChatSessionFunction> {
	const { spec, sessionId, pendingWrites, readToolsCalled, emit, stopController, agentId, parentToolCallUuid } = opts;
	const startedKey = agentId ? 'subagent_tool_started' : 'tool_call_started';
	const succeededKey = agentId ? 'subagent_tool_succeeded' : 'tool_call_succeeded';
	const failedKey = agentId ? 'subagent_tool_failed' : 'tool_call_failed';
	const handler = async (args: Record<string, unknown>): Promise<unknown> => {
		const repo = repositories.aiChat;
		if (spec.name === 'askUserChoice') {
			try {
				const result = (await spec.execute(args ?? {})) as { question: string; options: string[] };
				emit({ type: 'quick_reply', question: result.question, options: result.options });
				queueMicrotask(() => stopController.abort());
				return result;
			} catch (e) {
				const message = e instanceof Error ? e.message : 'tool failed';
				emit({ type: failedKey as AgentEvent['type'], tool_name: spec.name, error: message, agent_id: agentId });
				return { error: message };
			}
		}
		if (spec.kind === 'write') {
			if (pendingWrites.length > 0) {
				return {
					skipped: true,
					message: 'Only one write action allowed per turn. Wait for user approval first.'
				};
			}
			const argsJson = JSON.stringify(args ?? {});
			const recent = await repo.findRecentSucceededToolCall(sessionId, spec.name, argsJson, 10 * 60 * 1000);
			if (recent) {
				l.info('write tool deduplicated', { tool: spec.name, prior_uuid: recent.uuid });
				return {
					duplicate: true,
					message: `${spec.name} with these exact arguments was already approved and executed in this session (uuid ${recent.uuid}). Do not retry. The action is complete; tell the user it is done.`,
					prior_result: recent.result_json ? JSON.parse(recent.result_json) : null
				};
			}
			const required = REQUIRED_READS[spec.name];
			if (required && !required.some((r) => readToolsCalled.has(r))) {
				return {
					blocked: true,
					message: `You must call one of [${required.join(', ')}] before ${spec.name}, to confirm whether the entity already exists.`
				};
			}
			const row = await repo.createToolCall({
				session_id: sessionId,
				message_id: null,
				tool_name: spec.name,
				args_json: JSON.stringify(args ?? {}),
				status: 'pending',
				agent_id: agentId ?? null,
				parent_tool_call_uuid: parentToolCallUuid ?? null
			});
			pendingWrites.push({
				uuid: row.uuid,
				tool_name: spec.name,
				args: args ?? {},
				...(agentId !== undefined ? { agent_id: agentId } : {})
			});
			l.info('write tool queued', { tool: spec.name, agent_id: agentId, uuid: row.uuid });
			queueMicrotask(() => stopController.abort());
			return { queued: true, message: 'Action queued for user approval. Generation stopping.' };
		}
		readToolsCalled.add(spec.name);
		const callRow = await repo.createToolCall({
			session_id: sessionId,
			message_id: null,
			tool_name: spec.name,
			args_json: JSON.stringify(args ?? {}),
			status: 'running',
			agent_id: agentId ?? null,
			parent_tool_call_uuid: parentToolCallUuid ?? null
		});
		l.debug('read tool start', { tool: spec.name, agent_id: agentId, args });
		emit({ type: startedKey as AgentEvent['type'], uuid: callRow.uuid, tool_name: spec.name, args, agent_id: agentId });
		try {
			const result = await spec.execute(args ?? {});
			await repo.updateToolCall(callRow.uuid, {
				status: 'succeeded',
				result_json: JSON.stringify(result ?? null)
			});
			l.debug('read tool ok', { tool: spec.name });
			emit({ type: succeededKey as AgentEvent['type'], uuid: callRow.uuid, result, agent_id: agentId });
			return result;
		} catch (e) {
			const message = e instanceof Error ? e.message : 'tool failed';
			await repo.updateToolCall(callRow.uuid, { status: 'failed', error_message: message });
			l.warn('read tool failed', { tool: spec.name, error: message });
			emit({ type: failedKey as AgentEvent['type'], uuid: callRow.uuid, error: message, agent_id: agentId });
			return { error: message };
		}
	};
	return defineChatSessionFunction({
		description: spec.description,
		params: spec.paramSchema as never,
		handler: handler as never
	});
}

export async function approveToolCall(uuid: string): Promise<{ result?: unknown; error?: string }> {
	const repo = repositories.aiChat;
	const call = await repo.getToolCallByUuid(uuid);
	if (!call) {
		l.warn('approve: tool call not found', { uuid });
		return { error: 'tool call not found' };
	}
	if (call.status !== 'pending') return { error: `tool call already ${call.status}` };
	l.info('approve start', { uuid, tool: call.tool_name });
	const spec = getTool(call.tool_name);
	if (!spec) {
		await repo.updateToolCall(uuid, { status: 'failed', error_message: 'unknown tool' });
		return { error: 'unknown tool' };
	}
	const args = JSON.parse(call.args_json) as Record<string, unknown>;
	try {
		const result = await spec.execute(args);
		await repo.updateToolCall(uuid, {
			status: 'succeeded',
			result_json: JSON.stringify(result ?? null)
		});
		await repo.appendMessage({
			session_id: call.session_id,
			role: 'system',
			content: `Tool ${call.tool_name} approved and executed. Result: ${JSON.stringify(result)}`
		});
		l.info('approve ok', { uuid, tool: call.tool_name });
		return { result };
	} catch (e) {
		const message = e instanceof Error ? e.message : 'execution failed';
		await repo.updateToolCall(uuid, { error_message: message });
		await repo.appendMessage({
			session_id: call.session_id,
			role: 'system',
			content: `Tool ${call.tool_name} failed: ${message}`
		});
		l.error('approve failed', { uuid, tool: call.tool_name, error: message });
		return { error: message };
	}
}

export async function rejectToolCall(uuid: string): Promise<void> {
	const repo = repositories.aiChat;
	const call = await repo.getToolCallByUuid(uuid);
	if (!call || call.status !== 'pending') return;
	await repo.updateToolCall(uuid, { status: 'rejected' });
	await repo.appendMessage({
		session_id: call.session_id,
		role: 'system',
		content: `User rejected tool ${call.tool_name}.`
	});
	l.info('rejected', { uuid, tool: call.tool_name });
}

export async function compactSession(sessionId: number): Promise<void> {
	const repo = repositories.aiChat;
	const messages = await repo.listMessages(sessionId);
	if (messages.length < 4) return;
	const transcript = messages
		.map((m) => `[${m.role}] ${m.content}`)
		.join('\n')
		.slice(0, 12_000);
	const { session, context } = await createSession(
		'You compress chat transcripts. Output a concise factual summary, no editorial.'
	);
	try {
		const summary = await session.prompt(`Summarize:\n${transcript}`, { maxTokens: 512 });
		await repo.clearMessages(sessionId);
		await repo.appendMessage({
			session_id: sessionId,
			role: 'system',
			content: `Prior conversation summary: ${summary}`
		});
	} finally {
		await context.dispose();
	}
}

export { getSkill };
