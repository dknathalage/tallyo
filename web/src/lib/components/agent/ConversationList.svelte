<script lang="ts">
	import { onMount } from 'svelte';
	import { agentChat } from '$lib/stores/agentChat.svelte';

	function formatDate(iso: string): string {
		const d = new Date(iso);
		return d.toLocaleDateString(undefined, { month: 'short', day: 'numeric' });
	}

	onMount(() => {
		void agentChat.loadConversations();
	});
</script>

<div class="flex h-full flex-col">
	<!-- Header -->
	<div class="flex items-center justify-between border-b border-gray-200 px-3 py-3">
		<span class="text-sm font-semibold text-gray-700">Conversations</span>
		<button
			type="button"
			onclick={() => agentChat.newConversation()}
			class="rounded border border-gray-300 px-2 py-1 text-xs hover:bg-gray-50"
			aria-label="New chat"
		>
			New chat
		</button>
	</div>

	<!-- Conversation list -->
	<ul class="flex-1 overflow-y-auto py-1" role="list" aria-label="Conversation list">
		{#each agentChat.conversations as conv (conv.id)}
			<li>
				<button
					type="button"
					onclick={() => agentChat.selectConversation(conv.id)}
					class="flex w-full flex-col gap-0.5 rounded px-3 py-2 text-left transition-colors
						{agentChat.conversationId === conv.id
						? 'bg-gray-100 font-medium text-gray-900'
						: 'text-gray-700 hover:bg-gray-50'}"
					aria-current={agentChat.conversationId === conv.id ? 'page' : undefined}
				>
					<span class="truncate text-sm">{conv.title}</span>
					<span class="text-xs text-gray-400">{formatDate(conv.createdAt)}</span>
				</button>
			</li>
		{:else}
			<li class="px-3 py-4 text-sm text-gray-400">No conversations yet</li>
		{/each}
	</ul>
</div>
