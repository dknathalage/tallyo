<script lang="ts">
	import { agentChat } from '$lib/stores/agentChat.svelte';
	import type { AgentMessageDTO } from '$lib/api/agent';
	import MessageBubble from './MessageBubble.svelte';
	import Composer from './Composer.svelte';
	import PlanCard from './PlanCard.svelte';
	import ToolResultView from './ToolResultView.svelte';
	import AccessRequestPrompt from './AccessRequestPrompt.svelte';
	import RevertControl from './RevertControl.svelte';

	// Ref for the scrollable message region; guarded before use.
	let scrollEl = $state<HTMLDivElement | null>(null);

	const hasMessages = $derived(agentChat.messages.length > 0);
	const hasTurn = $derived(
		agentChat.turn.plan !== undefined ||
			agentChat.turn.toolResults.length > 0 ||
			agentChat.turn.pendingAccess != null ||
			agentChat.turn.finalText !== undefined ||
			agentChat.status === 'running' ||
			agentChat.status === 'error'
	);

	// Auto-scroll to the bottom whenever messages or the active turn change.
	$effect(() => {
		// Access reactive dependencies so the effect re-runs on change.
		void agentChat.messages.length;
		void agentChat.turn;
		if (scrollEl !== null) {
			scrollEl.scrollTop = scrollEl.scrollHeight;
		}
	});
</script>

<div class="flex h-full flex-col overflow-hidden">
	{#if !agentChat.enabled}
		<!-- Disabled banner — no composer -->
		<div class="flex flex-1 items-center justify-center p-8">
			<div
				class="max-w-sm rounded-lg border border-gray-200 bg-gray-50 px-6 py-5 text-center"
				role="alert"
			>
				<p class="text-sm font-medium text-gray-700">AI assistant is not configured</p>
				<p class="mt-1 text-xs text-gray-500">
					Contact your administrator to enable the AI features.
				</p>
			</div>
		</div>
	{:else}
		<!-- Scrollable message + turn region -->
		<div bind:this={scrollEl} class="flex-1 overflow-y-auto px-4 py-4">
			{#if !hasMessages && !hasTurn}
				<!-- Empty state -->
				<div class="flex h-full min-h-32 items-center justify-center">
					<p class="text-sm text-gray-400">
						Ask me to do something — e.g. &ldquo;list overdue invoices&rdquo;.
					</p>
				</div>
			{:else}
				<!-- Persisted message history -->
				<div class="space-y-3">
					{#each agentChat.messages as message (message.id)}
						<div>
							<MessageBubble {message} />
							{#if message.role === 'assistant' && message.checkpointId != null}
								<div class="mt-1 flex justify-start pl-1">
									<RevertControl
										checkpointId={message.checkpointId}
										status={message.checkpointStatus}
									/>
								</div>
							{/if}
						</div>
					{/each}
				</div>

				<!-- Live turn region -->
				{#if hasTurn}
					<div class="mt-3 space-y-3">
						{#if agentChat.turn.plan !== undefined}
							<PlanCard steps={agentChat.turn.plan} />
						{/if}

						{#each agentChat.turn.toolResults as result (result.toolUseId)}
							<ToolResultView {result} />
						{/each}

						{#if agentChat.turn.pendingAccess != null}
							<AccessRequestPrompt access={agentChat.turn.pendingAccess} />
						{/if}

						{#if agentChat.status === 'running'}
							<p class="text-xs text-gray-400 italic" aria-live="polite">
								{agentChat.turn.toolResults.length > 0 ? 'Working…' : 'Thinking…'}
							</p>
						{/if}

						{#if agentChat.turn.finalText !== undefined}
							<div
								class="rounded-lg border border-gray-200 bg-white px-3 py-2 text-sm text-gray-800"
							>
								<p class="whitespace-pre-wrap">{agentChat.turn.finalText}</p>
							</div>
						{/if}

						{#if agentChat.status === 'error'}
							<div
								class="rounded-lg border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700"
								role="alert"
							>
								{agentChat.errorText.length > 0 ? agentChat.errorText : 'An unexpected error occurred.'}
							</div>
						{/if}
					</div>
				{/if}
			{/if}
		</div>

		<!-- Composer pinned at bottom -->
		<Composer />
	{/if}
</div>
