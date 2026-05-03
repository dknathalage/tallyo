<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import {
		getChat,
		setOpen,
		setSession,
		setMessages,
		appendMessage,
		updateLastAssistant,
		setPending,
		removePending,
		setStreaming,
		clearLocal,
		setSkills,
		setLoadedSkillIds
	} from '$lib/stores/ai-chat.svelte.js';
	import type { ChatMessage, ChatToolEvent, ChatSubAgentRun, SkillSummary } from '$lib/stores/ai-chat.svelte.js';
	import {
		startActivity,
		updateActivity,
		endActivity
	} from '$lib/stores/activity.svelte.js';

	const chat = $derived(getChat());

	let input = $state('');
	let inputEl: HTMLTextAreaElement | null = $state(null);
	let scrollEl: HTMLDivElement | null = $state(null);
	let bootError = $state<string | null>(null);
	let helpOpen = $state(false);
	let skillsPopoverOpen = $state(false);
	let agentsPopoverOpen = $state(false);
	let auditOpen = $state(false);
	let auditCalls = $state<AuditCall[]>([]);
	let editingId = $state<string | null>(null);
	let editText = $state('');
	let abortController: AbortController | null = $state(null);
	let stageLabel = $state<string | null>(null);
	let expandedEvents = $state<Set<string>>(new Set());

	interface AuditCall {
		uuid: string;
		tool_name: string;
		args_json: string;
		status: string;
		result_json: string;
		error_message: string;
		agent_id: string | null;
		created_at: string;
		updated_at: string;
	}

	function toggleEventOpen(uuid: string) {
		const next = new Set(expandedEvents);
		if (next.has(uuid)) next.delete(uuid);
		else next.add(uuid);
		expandedEvents = next;
	}

	const COMMANDS = [
		{ name: '/help', desc: 'List commands' },
		{ name: '/audit', desc: 'Show full history of every tool call performed in this chat' },
		{ name: '/skills', desc: 'View and toggle loaded skills' },
		{ name: '/agents', desc: 'List specialized sub-agents you can invoke' },
		{ name: '/tools', desc: 'List currently loaded tools' },
		{ name: '/clear', desc: 'Clear current chat' },
		{ name: '/compact', desc: 'Summarize chat history into one message' },
		{ name: '/here', desc: 'Show current page context' },
		{ name: '/new', desc: 'Start a new chat session' },
		{ name: '/rename <title>', desc: 'Rename this chat' }
	];

	const AGENTS_LIST = [
		{
			id: 'invoice',
			title: 'InvoiceAgent',
			desc: 'Drafts an invoice end-to-end from notes',
			template: 'Use the InvoiceAgent to draft an invoice based on: '
		},
		{
			id: 'collection',
			title: 'CollectionAgent',
			desc: 'Reviews overdue and aging invoices',
			template: 'Use the CollectionAgent to follow up on overdue invoices'
		},
		{
			id: 'catalog-bulk',
			title: 'CatalogBulkAgent',
			desc: 'Performs bulk catalog edits',
			template: 'Use the CatalogBulkAgent to '
		}
	];

	onMount(() => {
		void ensureSession();
		const handler = (e: KeyboardEvent) => {
			if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'k') {
				e.preventDefault();
				setOpen(!chat.open);
				if (chat.open) setTimeout(() => inputEl?.focus(), 50);
			}
		};
		window.addEventListener('keydown', handler);
		return () => window.removeEventListener('keydown', handler);
	});

	$effect(() => {
		if (chat.messages.length || chat.streaming) {
			queueMicrotask(() => {
				if (scrollEl) scrollEl.scrollTop = scrollEl.scrollHeight;
			});
		}
	});

	async function ensureSession(): Promise<void> {
		try {
			const stored = localStorage.getItem('tallyo:ai_session_id');
			if (stored) {
				const id = Number(stored);
				if (Number.isFinite(id) && id > 0) {
					const ok = await loadSession(id);
					if (ok) return;
				}
			}
			await createNewSession();
		} catch (e) {
			bootError = e instanceof Error ? e.message : 'Failed to init chat';
		}
	}

	async function loadSession(id: number): Promise<boolean> {
		const res = await fetch(`/api/ai/chat/sessions/${id}`);
		if (!res.ok) return false;
		const data = (await res.json()) as {
			session: { id: number; uuid: string };
			messages: { id: number; role: string; content: string }[];
			pending: { uuid: string; tool_name: string; args_json: string }[];
		};
		setSession(data.session.id, data.session.uuid);
		const msgs: ChatMessage[] = data.messages.map((m) => ({
			id: String(m.id),
			role: (m.role === 'user' || m.role === 'assistant' ? m.role : 'system') as ChatMessage['role'],
			content: m.content,
			tool_events: [],
			pending: false
		}));
		setMessages(msgs);
		const pending: ChatToolEvent[] = data.pending.map((p) => ({
			uuid: p.uuid,
			tool_name: p.tool_name,
			args: JSON.parse(p.args_json) as Record<string, unknown>,
			status: 'pending'
		}));
		setPending(pending);
		localStorage.setItem('tallyo:ai_session_id', String(data.session.id));
		await fetchSkills(data.session.id);
		return true;
	}

	async function createNewSession(): Promise<void> {
		const res = await fetch('/api/ai/chat/sessions', {
			method: 'POST',
			headers: { 'Content-Type': 'application/json' },
			body: JSON.stringify({ title: 'Chat' })
		});
		if (!res.ok) throw new Error('Failed to create session');
		const data = (await res.json()) as { session: { id: number; uuid: string } };
		setSession(data.session.id, data.session.uuid);
		setMessages([]);
		setPending([]);
		localStorage.setItem('tallyo:ai_session_id', String(data.session.id));
		await fetchSkills(data.session.id);
	}

	async function fetchSkills(sessionId: number): Promise<void> {
		try {
			const res = await fetch(`/api/ai/chat/sessions/${sessionId}/skills`);
			if (!res.ok) return;
			const data = (await res.json()) as { loaded: string[]; available: SkillSummary[] };
			setSkills(data.loaded, data.available);
		} catch {
			// non-fatal
		}
	}

	async function openAudit(): Promise<void> {
		if (chat.sessionId == null) return;
		auditOpen = true;
		try {
			const res = await fetch(`/api/ai/chat/sessions/${chat.sessionId}/tool-calls`);
			if (!res.ok) return;
			const data = (await res.json()) as { calls: AuditCall[] };
			auditCalls = data.calls;
		} catch {
			// non-fatal
		}
	}

	function fmtTime(iso: string): string {
		if (!iso) return '';
		try {
			return new Date(iso).toLocaleString();
		} catch {
			return iso;
		}
	}

	function statusColor(status: string): string {
		if (status === 'succeeded') return 'text-green-600 dark:text-green-400';
		if (status === 'failed') return 'text-red-600 dark:text-red-400';
		if (status === 'rejected') return 'text-gray-500 dark:text-gray-400';
		if (status === 'pending') return 'text-amber-600 dark:text-amber-400';
		return 'text-gray-500 dark:text-gray-400';
	}

	async function toggleSkill(id: string, makeLoaded: boolean): Promise<void> {
		if (chat.sessionId == null) return;
		const body = makeLoaded ? { add: [id] } : { remove: [id] };
		const res = await fetch(`/api/ai/chat/sessions/${chat.sessionId}/skills`, {
			method: 'POST',
			headers: { 'Content-Type': 'application/json' },
			body: JSON.stringify(body)
		});
		if (!res.ok) return;
		const data = (await res.json()) as { loaded: string[] };
		setLoadedSkillIds(data.loaded);
	}

	async function send() {
		const text = input.trim();
		if (!text || chat.streaming || chat.sessionId == null) return;
		input = '';
		if (text.startsWith('/')) {
			await runCommand(text);
			return;
		}
		await runStream({ message: text, continuation: false, showUser: true });
	}

	async function runStream(opts: {
		message: string;
		continuation: boolean;
		showUser: boolean;
		editFromMessageId?: number;
	}) {
		if (chat.sessionId == null) return;
		if (opts.showUser) {
			appendMessage({
				id: `local-u-${Date.now()}`,
				role: 'user',
				content: opts.message,
				tool_events: [],
				pending: false
			});
		}
		appendMessage({
			id: `local-a-${Date.now() + 1}`,
			role: 'assistant',
			content: '',
			tool_events: [],
			pending: true
		});
		setStreaming(true);
		stageLabel = 'thinking';
		abortController = new AbortController();
		const activityId = startActivity('AI chat');
		try {
			const res = await fetch(`/api/ai/chat/sessions/${chat.sessionId}/messages`, {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({
					message: opts.message,
					route: page.url.pathname,
					continuation: opts.continuation,
					editFromMessageId: opts.editFromMessageId
				}),
				signal: abortController.signal
			});
			if (!res.ok || !res.body) throw new Error(`HTTP ${res.status}`);
			const reader = res.body.getReader();
			const dec = new TextDecoder();
			let buf = '';
			let done = false;
			while (!done) {
				const c = await reader.read();
				done = c.done;
				if (c.value) buf += dec.decode(c.value, { stream: true });
				const lines = buf.split('\n');
				buf = lines.pop() ?? '';
				for (const line of lines) {
					if (!line.trim()) continue;
					handleEvent(JSON.parse(line) as Record<string, unknown>, activityId);
				}
			}
			updateLastAssistant({ pending: false });
			if (chat.sessionId != null) await loadSession(chat.sessionId);
		} catch (e) {
			const msg = e instanceof Error ? e.message : String(e);
			const aborted = e instanceof DOMException && e.name === 'AbortError';
			updateLastAssistant({
				content: aborted ? '⏹ Stopped by user' : `Error: ${msg}`,
				pending: false
			});
		} finally {
			endActivity(activityId);
			setStreaming(false);
			stageLabel = null;
			abortController = null;
		}
	}

	function abort() {
		if (abortController) abortController.abort();
	}

	function startEdit(m: ChatMessage) {
		const idNum = Number(m.id);
		if (!Number.isFinite(idNum)) return;
		editingId = m.id;
		editText = m.content;
	}

	function cancelEdit() {
		editingId = null;
		editText = '';
	}

	async function saveEdit(m: ChatMessage) {
		const idNum = Number(m.id);
		if (!Number.isFinite(idNum)) return;
		const text = editText.trim();
		if (!text) return;
		editingId = null;
		editText = '';
		await runStream({
			message: text,
			continuation: false,
			showUser: false,
			editFromMessageId: idNum
		});
	}

	function handleEvent(ev: Record<string, unknown>, activityId: string): void {
		const type = ev['type'];
		switch (type) {
			case 'download_start':
				stageLabel = 'downloading model';
				updateActivity(activityId, {
					stage: 'downloading model',
					progress: { bytes: 0, total: Number(ev['total']) || 0 }
				});
				break;
			case 'download_progress': {
				const bytes = Number(ev['bytes']) || 0;
				const total = Number(ev['total']) || 0;
				const pct = total > 0 ? Math.round((bytes / total) * 100) : 0;
				stageLabel = `downloading model · ${pct}%`;
				updateActivity(activityId, {
					stage: 'downloading model',
					progress: { bytes, total }
				});
				break;
			}
			case 'model_loading':
				stageLabel = 'loading model';
				updateActivity(activityId, { stage: 'loading model', progress: null });
				break;
			case 'model_ready':
				stageLabel = 'thinking';
				updateActivity(activityId, { stage: 'ready', progress: null });
				break;
			case 'token': {
				const t = String(ev['text'] ?? '');
				const last = chat.messages[chat.messages.length - 1];
				if (!last) break;
				stageLabel = 'writing';
				updateLastAssistant({ content: last.content + t });
				break;
			}
			case 'tool_call_started': {
				const last = chat.messages[chat.messages.length - 1];
				if (!last) break;
				const evt: ChatToolEvent = {
					uuid: String(ev['uuid']),
					tool_name: String(ev['tool_name']),
					args: (ev['args'] as Record<string, unknown>) ?? {},
					status: 'running'
				};
				updateLastAssistant({ tool_events: [...last.tool_events, evt] });
				stageLabel = `running ${evt.tool_name}`;
				updateActivity(activityId, { stage: `tool: ${evt.tool_name}` });
				break;
			}
			case 'tool_call_succeeded':
			case 'tool_call_failed': {
				const last = chat.messages[chat.messages.length - 1];
				if (!last) break;
				const isOk = type === 'tool_call_succeeded';
				const next: ChatToolEvent[] = last.tool_events.map((e) => {
					if (e.uuid !== ev['uuid']) return e;
					const updated: ChatToolEvent = {
						...e,
						status: (isOk ? 'succeeded' : 'failed') as ChatToolEvent['status'],
						result: ev['result']
					};
					if (typeof ev['error'] === 'string') updated.error = ev['error'];
					return updated;
				});
				updateLastAssistant({ tool_events: next });
				break;
			}
			case 'quick_reply': {
				const question = String(ev['question'] ?? '');
				const rawOpts = Array.isArray(ev['options']) ? (ev['options'] as unknown[]) : [];
				const options = rawOpts.map((o) => String(o)).filter((o) => o.length > 0);
				updateLastAssistant({ quick_reply: { question, options } });
				break;
			}
			case 'tool_pending': {
				const evt: ChatToolEvent = {
					uuid: String(ev['uuid']),
					tool_name: String(ev['tool_name']),
					args: (ev['args'] as Record<string, unknown>) ?? {},
					status: 'pending'
				};
				setPending([...chat.pendingToolCalls, evt]);
				break;
			}
			case 'skills_resolved': {
				const ids = Array.isArray(ev['loaded']) ? (ev['loaded'] as unknown[]).map((s) => String(s)) : [];
				setLoadedSkillIds(ids);
				stageLabel = `loaded: ${ids.join(', ')}`;
				break;
			}
			case 'skill_loaded': {
				const id = String(ev['skill_id'] ?? '');
				if (!id) break;
				const current = chat.loadedSkills;
				if (!current.includes(id)) setLoadedSkillIds([...current, id]);
				stageLabel = `skill loaded: ${id}`;
				break;
			}
			case 'subagent_started': {
				const last = chat.messages[chat.messages.length - 1];
				if (!last) break;
				const run: ChatSubAgentRun = {
					agent_id: String(ev['agent_id'] ?? 'unknown'),
					title: String(ev['title'] ?? ev['agent_id'] ?? 'agent'),
					tool_events: [],
					tokens: '',
					summary: '',
					queued: [],
					steps: 0,
					open: true
				};
				const runs = [...(last.subagent_runs ?? []), run];
				updateLastAssistant({ subagent_runs: runs });
				stageLabel = `agent: ${run.title}`;
				break;
			}
			case 'subagent_token': {
				const last = chat.messages[chat.messages.length - 1];
				if (!last || !last.subagent_runs) break;
				const aid = String(ev['agent_id']);
				const text = String(ev['text'] ?? '');
				const runs = last.subagent_runs.map((r) => (r.agent_id === aid ? { ...r, tokens: r.tokens + text } : r));
				updateLastAssistant({ subagent_runs: runs });
				break;
			}
			case 'subagent_tool_started': {
				const last = chat.messages[chat.messages.length - 1];
				if (!last || !last.subagent_runs) break;
				const aid = String(ev['agent_id']);
				const evt: ChatToolEvent = {
					uuid: String(ev['uuid']),
					tool_name: String(ev['tool_name']),
					args: (ev['args'] as Record<string, unknown>) ?? {},
					status: 'running'
				};
				const runs = last.subagent_runs.map((r) =>
					r.agent_id === aid ? { ...r, tool_events: [...r.tool_events, evt] } : r
				);
				updateLastAssistant({ subagent_runs: runs });
				break;
			}
			case 'subagent_tool_succeeded':
			case 'subagent_tool_failed': {
				const last = chat.messages[chat.messages.length - 1];
				if (!last || !last.subagent_runs) break;
				const aid = String(ev['agent_id']);
				const isOk = type === 'subagent_tool_succeeded';
				const runs = last.subagent_runs.map((r) => {
					if (r.agent_id !== aid) return r;
					const next = r.tool_events.map((e): ChatToolEvent => {
						if (e.uuid !== ev['uuid']) return e;
						const updated: ChatToolEvent = {
							...e,
							status: (isOk ? 'succeeded' : 'failed') as ChatToolEvent['status'],
							result: ev['result']
						};
						if (typeof ev['error'] === 'string') updated.error = ev['error'];
						return updated;
					});
					return { ...r, tool_events: next };
				});
				updateLastAssistant({ subagent_runs: runs });
				break;
			}
			case 'subagent_done': {
				const last = chat.messages[chat.messages.length - 1];
				if (!last || !last.subagent_runs) break;
				const aid = String(ev['agent_id']);
				const summary = String(ev['summary'] ?? '');
				const queued = Array.isArray(ev['queued']) ? (ev['queued'] as unknown[]).map((s) => String(s)) : [];
				const steps = Number(ev['steps']) || 0;
				const stoppedReason = typeof ev['stopped_reason'] === 'string' ? ev['stopped_reason'] : undefined;
				const errorMsg = typeof ev['error'] === 'string' ? ev['error'] : undefined;
				const runs = last.subagent_runs.map((r) => {
					if (r.agent_id !== aid) return r;
					const updated: ChatSubAgentRun = { ...r, summary, queued, steps, open: false };
					if (stoppedReason) updated.stopped_reason = stoppedReason;
					if (errorMsg) updated.error = errorMsg;
					return updated;
				});
				updateLastAssistant({ subagent_runs: runs });
				break;
			}
			case 'error': {
				updateLastAssistant({ content: `Error: ${String(ev['message'] ?? 'unknown')}`, pending: false });
				break;
			}
		}
	}

	function toggleSubAgentOpen(messageId: string, agentId: string) {
		const idx = chat.messages.findIndex((m) => m.id === messageId);
		if (idx === -1) return;
		const m = chat.messages[idx];
		if (!m || !m.subagent_runs) return;
		const runs = m.subagent_runs.map((r) => (r.agent_id === agentId ? { ...r, open: !r.open } : r));
		const updated = { ...m, subagent_runs: runs };
		const before = chat.messages.slice(0, idx);
		const after = chat.messages.slice(idx + 1);
		setMessages([...before, updated, ...after]);
	}

	async function pickQuickReply(option: string) {
		if (chat.streaming) return;
		const last = chat.messages[chat.messages.length - 1];
		if (last && last.role === 'assistant') {
			updateLastAssistant({ quick_reply: undefined });
		}
		await runStream({ message: option, continuation: false, showUser: true });
	}

	async function approve(uuid: string) {
		const res = await fetch(`/api/ai/chat/tool-calls/${uuid}`, {
			method: 'POST',
			headers: { 'Content-Type': 'application/json' },
			body: JSON.stringify({ action: 'approve' })
		});
		if (!res.ok) {
			let msg = `HTTP ${res.status}`;
			try {
				const body = (await res.json()) as { message?: string };
				if (body?.message) msg = body.message;
			} catch {
				msg = (await res.text()) || msg;
			}
			appendMessage(systemMsg(`Approve failed: ${msg}`));
			return;
		}
		removePending(uuid);
		appendMessage(systemMsg(`✓ Tool executed`));
		await runStream({ message: '', continuation: true, showUser: false });
	}

	async function reject(uuid: string) {
		await fetch(`/api/ai/chat/tool-calls/${uuid}`, {
			method: 'POST',
			headers: { 'Content-Type': 'application/json' },
			body: JSON.stringify({ action: 'reject' })
		});
		removePending(uuid);
		appendMessage(systemMsg(`✗ Tool rejected`));
		await runStream({ message: '', continuation: true, showUser: false });
	}

	async function runCommand(cmd: string): Promise<void> {
		const [head, ...rest] = cmd.slice(1).split(/\s+/);
		const arg = rest.join(' ');
		if (chat.sessionId == null) return;
		switch (head) {
			case 'help':
				helpOpen = true;
				appendMessage(systemMsg(COMMANDS.map((c) => `${c.name} — ${c.desc}`).join('\n')));
				return;
			case 'skills':
				skillsPopoverOpen = true;
				if (chat.sessionId != null) await fetchSkills(chat.sessionId);
				return;
			case 'agents':
				agentsPopoverOpen = true;
				return;
			case 'audit':
				await openAudit();
				return;
			case 'tools': {
				const loaded = chat.availableSkills.filter((s) => s.loaded);
				const lines = loaded.length === 0
					? ['(no skills loaded yet)']
					: loaded.map((s) => `• ${s.title} (${s.id}) — ${s.tool_count} tools`);
				appendMessage(systemMsg('Loaded skills + tool counts:\n' + lines.join('\n')));
				return;
			}
			case 'here':
				appendMessage(systemMsg(`Current route: ${page.url.pathname}`));
				return;
			case 'clear':
				await fetch(`/api/ai/chat/sessions/${chat.sessionId}`, {
					method: 'POST',
					headers: { 'Content-Type': 'application/json' },
					body: JSON.stringify({ action: 'clear' })
				});
				clearLocal();
				return;
			case 'compact': {
				appendMessage(systemMsg('Compacting…'));
				const res = await fetch(`/api/ai/chat/sessions/${chat.sessionId}`, {
					method: 'POST',
					headers: { 'Content-Type': 'application/json' },
					body: JSON.stringify({ action: 'compact' })
				});
				if (res.ok && chat.sessionId != null) await loadSession(chat.sessionId);
				else appendMessage(systemMsg('Compact failed'));
				return;
			}
			case 'new':
				await createNewSession();
				return;
			case 'rename': {
				if (!arg.trim()) {
					appendMessage(systemMsg('usage: /rename <title>'));
					return;
				}
				await fetch(`/api/ai/chat/sessions/${chat.sessionId}`, {
					method: 'POST',
					headers: { 'Content-Type': 'application/json' },
					body: JSON.stringify({ action: 'rename', title: arg })
				});
				appendMessage(systemMsg(`Renamed to "${arg}"`));
				return;
			}
			default:
				appendMessage(systemMsg(`Unknown command: /${head}. Try /help.`));
		}
	}

	function systemMsg(content: string): ChatMessage {
		return {
			id: `sys-${Date.now()}-${Math.random().toString(36).slice(2, 6)}`,
			role: 'system',
			content,
			tool_events: [],
			pending: false
		};
	}

	function onKeyDown(e: KeyboardEvent) {
		if (e.key === 'Enter' && !e.shiftKey) {
			e.preventDefault();
			void send();
		}
	}

	function fmtArgs(args: Record<string, unknown>): string {
		try {
			return JSON.stringify(args, null, 2);
		} catch {
			return String(args);
		}
	}
</script>

{#if chat.open}
	<button
		type="button"
		class="fixed inset-0 z-30 bg-black/40 lg:hidden"
		onclick={() => setOpen(false)}
		aria-label="Close chat"
	></button>
{/if}

<aside
	class="fixed right-0 top-0 z-40 flex h-screen w-full max-w-md flex-col border-l border-gray-200 bg-white text-gray-900 shadow-xl transition-transform duration-200 dark:border-gray-700 dark:bg-gray-900 dark:text-gray-100 lg:w-96 lg:shadow-none {chat.open ? 'translate-x-0' : 'translate-x-full'}"
	aria-label="AI chat"
>
	<header class="flex items-center justify-between border-b border-gray-200 px-4 py-3 dark:border-gray-700">
		<div class="flex items-center gap-2">
			<span class="inline-block h-2 w-2 rounded-full bg-primary-500"></span>
			<h2 class="text-sm font-semibold">Tallyo AI</h2>
		</div>
		<div class="flex items-center gap-1">
			<button
				type="button"
				class="rounded px-2 py-1 text-xs text-gray-500 hover:bg-gray-100 dark:hover:bg-gray-800"
				onclick={() => void runCommand('/new')}
				title="New chat"
			>+ New</button>
			<button
				type="button"
				class="rounded px-2 py-1 text-xs text-gray-500 hover:bg-gray-100 dark:hover:bg-gray-800"
				onclick={() => (helpOpen = !helpOpen)}
				title="Help"
			>?</button>
			<button
				type="button"
				class="rounded px-2 py-1 text-xs text-gray-500 hover:bg-gray-100 dark:hover:bg-gray-800"
				onclick={() => setOpen(false)}
				aria-label="Close"
			>✕</button>
		</div>
	</header>

	{#if helpOpen}
		<div class="border-b border-gray-200 bg-gray-50 px-4 py-2 text-xs dark:border-gray-700 dark:bg-gray-800">
			<div class="mb-1 font-semibold">Commands</div>
			<ul class="space-y-0.5">
				{#each COMMANDS as c}
					<li><code class="text-primary-600 dark:text-primary-400">{c.name}</code> — {c.desc}</li>
				{/each}
			</ul>
		</div>
	{/if}

	{#if bootError}
		<div class="border-b border-red-200 bg-red-50 px-4 py-2 text-xs text-red-800 dark:border-red-800 dark:bg-red-950 dark:text-red-200">
			{bootError}
		</div>
	{/if}

	<div bind:this={scrollEl} class="flex-1 space-y-3 overflow-y-auto px-4 py-4 text-sm">
		{#if chat.messages.length === 0}
			<div class="text-center text-xs text-gray-400 dark:text-gray-500">
				Ask me anything. Try /help for commands.
			</div>
		{/if}
		{#each chat.messages as m (m.id)}
			<div class="space-y-1">
				{#if m.role === 'user'}
					{#if editingId === m.id}
						<div class="ml-auto w-[90%] space-y-2">
							<textarea
								bind:value={editText}
								rows="3"
								class="w-full resize-none rounded border border-primary-300 bg-white px-3 py-2 text-sm text-gray-900 focus:border-primary-500 focus:outline-none dark:border-primary-700 dark:bg-gray-800 dark:text-gray-100"
							></textarea>
							<div class="flex justify-end gap-2 text-xs">
								<button type="button" class="rounded px-2 py-1 text-gray-500 hover:bg-gray-100 dark:hover:bg-gray-800" onclick={cancelEdit}>Cancel</button>
								<button type="button" class="rounded bg-primary-600 px-3 py-1 font-medium text-white hover:bg-primary-700" onclick={() => void saveEdit(m)}>Save & rerun</button>
							</div>
						</div>
					{:else}
						<div class="ml-auto flex max-w-[85%] flex-col items-end gap-1">
							<div class="rounded-lg bg-primary-600 px-3 py-2 text-white">{m.content}</div>
							{#if Number.isFinite(Number(m.id)) && !chat.streaming}
								<button
									type="button"
									class="text-[10px] text-gray-400 hover:text-gray-600 dark:hover:text-gray-300"
									onclick={() => startEdit(m)}
								>edit & rerun</button>
							{/if}
						</div>
					{/if}
				{:else if m.role === 'system'}
					<div class="mx-auto max-w-[90%] whitespace-pre-wrap rounded border border-gray-200 bg-gray-50 px-3 py-1.5 text-xs text-gray-600 dark:border-gray-700 dark:bg-gray-800 dark:text-gray-300">
						{m.content}
					</div>
				{:else}
					<div class="max-w-[90%] space-y-2">
						{#each m.tool_events as t (t.uuid)}
							<div class="overflow-hidden rounded border border-gray-200 text-xs dark:border-gray-700">
								<button
									type="button"
									class="flex w-full items-center justify-between gap-2 bg-gray-50 px-2 py-1 text-left hover:bg-gray-100 dark:bg-gray-800 dark:hover:bg-gray-700"
									onclick={() => toggleEventOpen(t.uuid)}
								>
									<span class="flex items-center gap-2">
										<span class="text-gray-400">{expandedEvents.has(t.uuid) ? '▾' : '▸'}</span>
										<span class="font-mono">{t.tool_name}</span>
									</span>
									<span class="capitalize {t.status === 'failed' ? 'text-red-500' : 'text-gray-500'}">{t.status}</span>
								</button>
								{#if expandedEvents.has(t.uuid)}
									<div class="space-y-1 border-t border-gray-200 bg-white px-2 py-1 dark:border-gray-700 dark:bg-gray-900">
										<div class="text-[10px] uppercase tracking-wide text-gray-400">args</div>
										<pre class="max-h-32 overflow-auto whitespace-pre-wrap text-gray-700 dark:text-gray-200">{fmtArgs(t.args)}</pre>
										{#if t.result !== undefined}
											<div class="text-[10px] uppercase tracking-wide text-gray-400">result</div>
											<pre class="max-h-32 overflow-auto whitespace-pre-wrap text-gray-700 dark:text-gray-200">{fmtArgs((t.result ?? null) as Record<string, unknown>)}</pre>
										{/if}
										{#if t.error}
											<div class="text-[10px] uppercase tracking-wide text-red-500">error</div>
											<div class="text-red-500">{t.error}</div>
										{/if}
									</div>
								{/if}
							</div>
						{/each}
						{#if m.subagent_runs}
							{#each m.subagent_runs as r (r.agent_id)}
								<div class="rounded border border-primary-200 bg-primary-50 text-xs dark:border-primary-800 dark:bg-primary-950">
									<button
										type="button"
										class="flex w-full items-center justify-between px-2 py-1 text-left"
										onclick={() => toggleSubAgentOpen(m.id, r.agent_id)}
									>
										<span class="flex items-center gap-2">
											<span class="text-primary-700 dark:text-primary-300">{r.open ? '▾' : '▸'}</span>
											<span class="font-semibold text-primary-800 dark:text-primary-200">{r.title}</span>
											<span class="text-primary-600 dark:text-primary-400">{r.tool_events.length} tools{r.queued.length ? ` · ${r.queued.length} pending` : ''}{r.steps ? ` · ${r.steps} steps` : ''}{r.stopped_reason ? ` · ${r.stopped_reason}` : ''}</span>
										</span>
									</button>
									{#if r.open}
										<div class="space-y-1 border-t border-primary-200 px-2 py-1 dark:border-primary-800">
											{#each r.tool_events as t (t.uuid)}
												<div class="overflow-hidden rounded border border-primary-200 dark:border-primary-800">
													<button
														type="button"
														class="flex w-full items-center justify-between gap-2 px-1 py-0.5 text-left hover:bg-white/50 dark:hover:bg-black/20"
														onclick={() => toggleEventOpen(t.uuid)}
													>
														<span class="flex items-center gap-2">
															<span class="text-primary-400">{expandedEvents.has(t.uuid) ? '▾' : '▸'}</span>
															<span class="font-mono">{t.tool_name}</span>
														</span>
														<span class="capitalize {t.status === 'failed' ? 'text-red-500' : 'text-primary-600 dark:text-primary-300'}">{t.status}</span>
													</button>
													{#if expandedEvents.has(t.uuid)}
														<div class="space-y-1 border-t border-primary-200 bg-white px-1 py-1 dark:border-primary-800 dark:bg-gray-900">
															<div class="text-[10px] uppercase tracking-wide text-gray-400">args</div>
															<pre class="max-h-32 overflow-auto whitespace-pre-wrap text-gray-700 dark:text-gray-200">{fmtArgs(t.args)}</pre>
															{#if t.result !== undefined}
																<div class="text-[10px] uppercase tracking-wide text-gray-400">result</div>
																<pre class="max-h-32 overflow-auto whitespace-pre-wrap text-gray-700 dark:text-gray-200">{fmtArgs((t.result ?? null) as Record<string, unknown>)}</pre>
															{/if}
															{#if t.error}
																<div class="text-[10px] uppercase tracking-wide text-red-500">error</div>
																<div class="text-red-500">{t.error}</div>
															{/if}
														</div>
													{/if}
												</div>
											{/each}
											{#if r.tokens}
												<div class="whitespace-pre-wrap text-primary-700 dark:text-primary-300">{r.tokens}</div>
											{/if}
											{#if r.error}
												<div class="text-red-500">Error: {r.error}</div>
											{/if}
										</div>
									{/if}
								</div>
							{/each}
						{/if}
						{#if m.content}
							<div class="whitespace-pre-wrap rounded-lg bg-gray-100 px-3 py-2 text-gray-900 dark:bg-gray-800 dark:text-gray-100">
								{m.content}{#if m.pending}<span class="ml-1 inline-block h-2 w-2 animate-pulse rounded-full bg-primary-500"></span>{/if}
							</div>
						{/if}
						{#if m.quick_reply}
							<div class="space-y-2 rounded-lg border border-primary-200 bg-primary-50 p-3 dark:border-primary-800 dark:bg-primary-950">
								<div class="text-xs font-medium text-primary-900 dark:text-primary-100">{m.quick_reply.question}</div>
								<div class="flex flex-wrap gap-2">
									{#each m.quick_reply.options as opt}
										<button
											type="button"
											disabled={chat.streaming}
											onclick={() => void pickQuickReply(opt)}
											class="rounded border border-primary-300 bg-white px-3 py-1 text-xs font-medium text-primary-700 hover:bg-primary-100 disabled:opacity-50 dark:border-primary-700 dark:bg-gray-900 dark:text-primary-200 dark:hover:bg-primary-900"
										>{opt}</button>
									{/each}
								</div>
							</div>
						{/if}
					</div>
				{/if}
			</div>
		{/each}

		{#each chat.pendingToolCalls as p (p.uuid)}
			<div class="rounded-lg border border-amber-300 bg-amber-50 p-3 dark:border-amber-700 dark:bg-amber-950">
				<div class="mb-1 flex items-center justify-between">
					<span class="text-xs font-semibold text-amber-800 dark:text-amber-200">Approve action</span>
					<span class="font-mono text-xs text-amber-700 dark:text-amber-300">{p.tool_name}</span>
				</div>
				<pre class="mb-2 max-h-40 overflow-auto rounded bg-white p-2 text-xs text-gray-700 dark:bg-gray-900 dark:text-gray-200">{fmtArgs(p.args)}</pre>
				<div class="flex gap-2">
					<button
						type="button"
						class="flex-1 rounded bg-green-600 px-3 py-1 text-xs font-medium text-white hover:bg-green-700"
						onclick={() => void approve(p.uuid)}
					>Approve</button>
					<button
						type="button"
						class="flex-1 rounded bg-gray-200 px-3 py-1 text-xs font-medium text-gray-800 hover:bg-gray-300 dark:bg-gray-700 dark:text-gray-200 dark:hover:bg-gray-600"
						onclick={() => void reject(p.uuid)}
					>Reject</button>
				</div>
			</div>
		{/each}
	</div>

	{#if chat.streaming}
		<div class="flex items-center justify-between gap-2 border-t border-gray-200 bg-primary-50 px-4 py-2 text-xs text-primary-900 dark:border-gray-700 dark:bg-primary-950 dark:text-primary-200">
			<div class="flex items-center gap-2">
				<span class="inline-block h-2 w-2 animate-pulse rounded-full bg-primary-500"></span>
				<span>{stageLabel ?? 'thinking'}…</span>
			</div>
			<button
				type="button"
				onclick={abort}
				class="rounded border border-primary-300 px-2 py-0.5 text-primary-700 hover:bg-primary-100 dark:border-primary-700 dark:text-primary-200 dark:hover:bg-primary-900"
			>Stop</button>
		</div>
	{/if}

	{#if skillsPopoverOpen}
		<div class="max-h-64 overflow-y-auto border-t border-gray-200 bg-gray-50 px-3 py-2 text-xs dark:border-gray-700 dark:bg-gray-800">
			<div class="mb-1 flex items-center justify-between">
				<span class="font-semibold">Skills</span>
				<button type="button" class="text-gray-500 hover:text-gray-700 dark:hover:text-gray-300" onclick={() => (skillsPopoverOpen = false)}>✕</button>
			</div>
			<ul class="space-y-1">
				{#each chat.availableSkills as s (s.id)}
					<li class="flex items-start justify-between gap-2 rounded p-1 hover:bg-white dark:hover:bg-gray-900">
						<div class="flex-1">
							<div class="font-mono text-[11px]">{s.id} <span class="text-gray-400">({s.tool_count} tools)</span></div>
							<div class="text-gray-600 dark:text-gray-400">{s.description}</div>
						</div>
						<button
							type="button"
							disabled={s.alwaysLoaded}
							onclick={() => void toggleSkill(s.id, !s.loaded)}
							class="shrink-0 rounded px-2 py-0.5 text-[10px] font-medium {s.loaded
								? 'bg-primary-600 text-white'
								: 'border border-gray-300 text-gray-700 hover:bg-gray-100 dark:border-gray-600 dark:text-gray-300 dark:hover:bg-gray-700'} disabled:opacity-50"
						>{s.alwaysLoaded ? 'always' : s.loaded ? 'loaded' : 'load'}</button>
					</li>
				{/each}
			</ul>
		</div>
	{/if}

	{#if agentsPopoverOpen}
		<div class="max-h-64 overflow-y-auto border-t border-gray-200 bg-gray-50 px-3 py-2 text-xs dark:border-gray-700 dark:bg-gray-800">
			<div class="mb-1 flex items-center justify-between">
				<span class="font-semibold">Agents</span>
				<button type="button" class="text-gray-500 hover:text-gray-700 dark:hover:text-gray-300" onclick={() => (agentsPopoverOpen = false)}>✕</button>
			</div>
			<ul class="space-y-1">
				{#each AGENTS_LIST as a (a.id)}
					<li class="flex items-start justify-between gap-2 rounded p-1 hover:bg-white dark:hover:bg-gray-900">
						<div class="flex-1">
							<div class="font-semibold">{a.title}</div>
							<div class="text-gray-600 dark:text-gray-400">{a.desc}</div>
						</div>
						<button
							type="button"
							onclick={() => { input = a.template; agentsPopoverOpen = false; setTimeout(() => inputEl?.focus(), 50); }}
							class="shrink-0 rounded border border-gray-300 px-2 py-0.5 text-[10px] font-medium text-gray-700 hover:bg-gray-100 dark:border-gray-600 dark:text-gray-300 dark:hover:bg-gray-700"
						>insert</button>
					</li>
				{/each}
			</ul>
		</div>
	{/if}

	<div class="flex flex-wrap items-center gap-1 border-t border-gray-200 px-3 py-1 text-[10px] dark:border-gray-700">
		<span class="text-gray-400">Skills:</span>
		{#each chat.availableSkills.filter((s) => s.loaded) as s (s.id)}
			<span class="rounded bg-primary-100 px-1.5 py-0.5 font-mono text-primary-700 dark:bg-primary-900 dark:text-primary-200">{s.id}</span>
		{/each}
		<button
			type="button"
			class="rounded border border-gray-300 px-1.5 py-0.5 text-gray-500 hover:bg-gray-100 dark:border-gray-700 dark:hover:bg-gray-800"
			onclick={() => { skillsPopoverOpen = !skillsPopoverOpen; if (skillsPopoverOpen && chat.sessionId != null) void fetchSkills(chat.sessionId); }}
		>+ Skills</button>
		<button
			type="button"
			class="rounded border border-gray-300 px-1.5 py-0.5 text-gray-500 hover:bg-gray-100 dark:border-gray-700 dark:hover:bg-gray-800"
			onclick={() => (agentsPopoverOpen = !agentsPopoverOpen)}
		>Agents</button>
		<button
			type="button"
			class="rounded border border-gray-300 px-1.5 py-0.5 text-gray-500 hover:bg-gray-100 dark:border-gray-700 dark:hover:bg-gray-800"
			onclick={() => void openAudit()}
		>Audit</button>
	</div>

	{#if auditOpen}
		<div class="fixed inset-0 z-50 flex items-center justify-center bg-black/40 p-4">
			<div class="flex max-h-[80vh] w-full max-w-2xl flex-col rounded-lg border border-gray-200 bg-white shadow-xl dark:border-gray-700 dark:bg-gray-900">
				<div class="flex items-center justify-between border-b border-gray-200 px-4 py-2 text-sm dark:border-gray-700">
					<span class="font-semibold">Action audit · {auditCalls.length} call{auditCalls.length === 1 ? '' : 's'}</span>
					<button type="button" class="rounded px-2 py-1 text-gray-500 hover:bg-gray-100 dark:hover:bg-gray-800" onclick={() => (auditOpen = false)}>✕</button>
				</div>
				<div class="flex-1 overflow-y-auto px-4 py-3 text-xs">
					{#if auditCalls.length === 0}
						<div class="text-center text-gray-400">No tool calls yet in this session.</div>
					{/if}
					<ul class="space-y-2">
						{#each auditCalls as c (c.uuid)}
							<li class="rounded border border-gray-200 dark:border-gray-700">
								<div class="flex items-center justify-between gap-2 border-b border-gray-200 bg-gray-50 px-2 py-1 dark:border-gray-700 dark:bg-gray-800">
									<span class="flex items-center gap-2">
										<span class="font-mono">{c.tool_name}</span>
										{#if c.agent_id}
											<span class="rounded bg-primary-100 px-1 text-primary-700 dark:bg-primary-900 dark:text-primary-200">{c.agent_id}</span>
										{/if}
									</span>
									<span class="flex items-center gap-2">
										<span class="capitalize {statusColor(c.status)}">{c.status}</span>
										<span class="text-gray-400">{fmtTime(c.updated_at || c.created_at)}</span>
									</span>
								</div>
								<details class="px-2 py-1">
									<summary class="cursor-pointer text-gray-500">args / result</summary>
									<div class="mt-1 space-y-1">
										<div class="text-[10px] uppercase tracking-wide text-gray-400">args</div>
										<pre class="max-h-40 overflow-auto whitespace-pre-wrap text-gray-700 dark:text-gray-200">{c.args_json}</pre>
										{#if c.status === 'succeeded'}
											<div class="text-[10px] uppercase tracking-wide text-gray-400">result</div>
											<pre class="max-h-40 overflow-auto whitespace-pre-wrap text-gray-700 dark:text-gray-200">{c.result_json}</pre>
										{/if}
										{#if c.error_message}
											<div class="text-[10px] uppercase tracking-wide text-red-500">error</div>
											<div class="text-red-500">{c.error_message}</div>
										{/if}
									</div>
								</details>
							</li>
						{/each}
					</ul>
				</div>
			</div>
		</div>
	{/if}

	<div class="border-t border-gray-200 p-3 dark:border-gray-700">
		<textarea
			bind:this={inputEl}
			bind:value={input}
			onkeydown={onKeyDown}
			rows="2"
			maxlength="8000"
			placeholder="Ask anything · / for commands · Enter to send"
			disabled={chat.streaming}
			class="w-full resize-none rounded border border-gray-300 bg-white px-3 py-2 text-sm text-gray-900 focus:border-primary-500 focus:outline-none disabled:opacity-50 dark:border-gray-700 dark:bg-gray-800 dark:text-gray-100"
		></textarea>
		<div class="mt-1 flex items-center justify-between text-xs text-gray-400">
			<span>{page.url.pathname}</span>
			<span>{chat.streaming ? 'Working…' : '⌘K to focus'}</span>
		</div>
	</div>
</aside>
