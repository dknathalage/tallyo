<script lang="ts">
	import { agentChat } from '$lib/stores/agentChat.svelte';

	let text = $state('');
	// Textarea element ref for auto-grow
	let textareaEl = $state<HTMLTextAreaElement | null>(null);

	function send(): void {
		const trimmed = text.trim();
		// No-op on empty input
		if (trimmed.length === 0) return;
		agentChat.send(trimmed);
		text = '';
		// Reset textarea height after clearing
		if (textareaEl !== null) {
			textareaEl.style.height = 'auto';
		}
	}

	function onKeydown(e: KeyboardEvent): void {
		// Shift+Enter → insert newline (default textarea behaviour; we just don't intercept it)
		// Enter (no Shift) → send; ⌘/Ctrl+Enter → also send
		if (e.key === 'Enter') {
			if (e.shiftKey) {
				// Allow the browser to insert a newline
				return;
			}
			// Plain Enter or ⌘/Ctrl+Enter both send
			e.preventDefault();
			send();
		}
	}

	function onInput(): void {
		// Auto-grow: reset to auto first so shrinking works, then expand to scrollHeight
		if (textareaEl !== null) {
			textareaEl.style.height = 'auto';
			textareaEl.style.height = `${textareaEl.scrollHeight}px`;
		}
	}

	const disabled = $derived(agentChat.status === 'running');
</script>

<div class="flex items-end gap-2 border-t border-gray-200 px-3 py-3">
	<textarea
		bind:this={textareaEl}
		bind:value={text}
		onkeydown={onKeydown}
		oninput={onInput}
		rows={1}
		{disabled}
		placeholder="Message the assistant…"
		aria-label="Message input"
		class="max-h-48 flex-1 resize-none overflow-y-auto rounded border border-gray-300 px-3 py-2 text-sm
			placeholder:text-gray-400
			focus:border-gray-400 focus:outline-none
			disabled:bg-gray-50 disabled:text-gray-400"
	></textarea>
	<button
		type="button"
		onclick={send}
		{disabled}
		aria-label="Send message"
		class="shrink-0 rounded border border-gray-300 px-3 py-2 text-sm
			hover:bg-gray-50
			disabled:cursor-not-allowed disabled:opacity-50"
	>
		Send
	</button>
</div>
