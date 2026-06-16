<script lang="ts">
	import type { AgentMessageDTO, AgentBlock } from '$lib/api/agent';

	interface Props {
		message: AgentMessageDTO;
	}

	let { message }: Props = $props();

	// Extract all text blocks and join into a single display string.
	const textContent = $derived(
		message.content
			.filter((b: AgentBlock) => b.type === 'text')
			.map((b: AgentBlock) => b.text ?? '')
			.join('')
	);

	// Tool-use blocks shown for assistant messages as compact muted annotations.
	const toolUseBlocks = $derived(
		message.role === 'assistant'
			? message.content.filter((b: AgentBlock) => b.type === 'tool_use')
			: ([] as AgentBlock[])
	);
</script>

<div
	class="flex w-full {message.role === 'user' ? 'justify-end' : 'justify-start'}"
	aria-label="{message.role} message"
>
	<div
		class="max-w-[80%] rounded-lg px-3 py-2 text-sm
			{message.role === 'user'
			? 'bg-gray-800 text-white'
			: 'border border-gray-200 bg-white text-gray-800'}"
	>
		{#if textContent.length > 0}
			<!-- Text blocks: pre-wrap preserves newlines, no @html -->
			<p class="whitespace-pre-wrap">{textContent}</p>
		{/if}

		{#each toolUseBlocks as block (block.toolUseId ?? block.toolName)}
			<p class="mt-1 text-xs text-gray-400 italic">
				used {block.toolName ?? 'tool'}
			</p>
		{/each}
	</div>
</div>
