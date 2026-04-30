<script lang="ts">
  import { onMount } from 'svelte';
  import AiMessage from '$lib/components/ai/AiMessage.svelte';
  import { aiChat } from '$lib/stores/ai-chat.svelte.js';

  let { data } = $props();
  let inputText = $state('');
  let messagesEl = $state<HTMLElement | null>(null);
  let inputEl = $state<HTMLTextAreaElement | null>(null);

  const EXAMPLE_PROMPTS = [
    'Show me my outstanding invoices',
    'What\'s my total revenue this month?',
    'Create an invoice for $500 of consulting work',
    'Which clients have overdue payments?'
  ];

  onMount(async () => {
    await aiChat.loadSessions();
    const firstSession = aiChat.sessions[0];
    if (firstSession) {
      await aiChat.selectSession(firstSession.id);
    } else {
      await aiChat.createSession();
    }
  });

  $effect(() => {
    // Auto-scroll when messages or streaming text changes
    if (aiChat.messages.length || aiChat.streaming.text) {
      setTimeout(() => messagesEl?.scrollTo({ top: messagesEl.scrollHeight, behavior: 'smooth' }), 50);
    }
  });

  async function handleSubmit() {
    const text = inputText.trim();
    if (!text || aiChat.isStreaming) return;
    inputText = '';
    await aiChat.sendMessage(text);
  }

  function handleKeydown(e: KeyboardEvent) {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      handleSubmit();
    }
  }
</script>

<div class="flex h-full overflow-hidden">
  <!-- Session sidebar -->
  <aside class="w-64 border-r border-gray-200 dark:border-gray-700 flex flex-col shrink-0">
    <div class="p-3 border-b border-gray-200 dark:border-gray-700">
      <button
        onclick={() => aiChat.createSession()}
        class="w-full flex items-center gap-2 px-3 py-2 rounded-lg bg-blue-600 hover:bg-blue-700 text-white text-sm font-medium transition-colors"
      >
        <svg xmlns="http://www.w3.org/2000/svg" class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 4v16m8-8H4" />
        </svg>
        New Chat
      </button>
    </div>
    <div class="flex-1 overflow-y-auto p-2 space-y-1">
      {#each aiChat.sessions as session (session.id)}
        <div class="group relative">
          <button
            onclick={() => aiChat.selectSession(session.id)}
            class="w-full text-left px-3 py-2 rounded-lg text-sm truncate transition-colors
              {aiChat.activeSessionId === session.id
                ? 'bg-blue-50 dark:bg-blue-900/30 text-blue-700 dark:text-blue-300'
                : 'hover:bg-gray-100 dark:hover:bg-gray-800 text-gray-700 dark:text-gray-300'}"
          >
            {session.title}
          </button>
          <button
            onclick={(e) => { e.stopPropagation(); aiChat.deleteSession(session.id); }}
            class="absolute right-1 top-1/2 -translate-y-1/2 opacity-0 group-hover:opacity-100 p-1 rounded hover:bg-red-100 dark:hover:bg-red-900/40 text-red-500 transition-opacity"
            aria-label="Delete session"
          >
            <svg xmlns="http://www.w3.org/2000/svg" class="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
            </svg>
          </button>
        </div>
      {/each}
    </div>
  </aside>

  <!-- Main chat area -->
  <div class="flex-1 flex flex-col min-w-0">
    <!-- No API key banner -->
    {#if !data.apiKeyConfigured}
      <div class="bg-amber-50 dark:bg-amber-900/20 border-b border-amber-200 dark:border-amber-800 px-4 py-2 text-sm text-amber-800 dark:text-amber-300 flex items-center gap-2">
        <svg xmlns="http://www.w3.org/2000/svg" class="w-4 h-4 shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
        </svg>
        <span>No API key configured. <a href="/console/settings" class="underline font-medium">Go to Settings</a> to add your Anthropic API key.</span>
      </div>
    {/if}

    <!-- Messages -->
    <div bind:this={messagesEl} class="flex-1 overflow-y-auto p-4">
      {#if aiChat.messages.length === 0 && !aiChat.isStreaming}
        <div class="h-full flex flex-col items-center justify-center text-center gap-6">
          <div>
            <div class="w-12 h-12 rounded-full bg-blue-100 dark:bg-blue-900/40 flex items-center justify-center mx-auto mb-3">
              <svg xmlns="http://www.w3.org/2000/svg" class="w-6 h-6 text-blue-600 dark:text-blue-400" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9.813 15.904L9 18.75l-.813-2.846a4.5 4.5 0 00-3.09-3.09L2.25 12l2.846-.813a4.5 4.5 0 003.09-3.09L9 5.25l.813 2.846a4.5 4.5 0 003.09 3.09L15.75 12l-2.846.813a4.5 4.5 0 00-3.09 3.09z" />
              </svg>
            </div>
            <h2 class="text-lg font-semibold text-gray-900 dark:text-gray-100">AI Assistant</h2>
            <p class="text-sm text-gray-500 dark:text-gray-400 mt-1">Ask me anything about your invoices, clients, or finances.</p>
          </div>
          <div class="grid grid-cols-1 sm:grid-cols-2 gap-2 w-full max-w-md">
            {#each EXAMPLE_PROMPTS as prompt}
              <button
                onclick={() => { inputText = prompt; inputEl?.focus(); }}
                class="text-left text-sm px-3 py-2 rounded-lg border border-gray-200 dark:border-gray-700 hover:bg-gray-50 dark:hover:bg-gray-800 text-gray-700 dark:text-gray-300 transition-colors"
              >{prompt}</button>
            {/each}
          </div>
        </div>
      {:else}
        {#each aiChat.messages as msg (msg.id)}
          <AiMessage role={msg.role} content={msg.content} toolCalls={msg.tool_calls} />
        {/each}
        {#if aiChat.isStreaming}
          <AiMessage
            role="assistant"
            content=""
            isStreaming={true}
            streamText={aiChat.streaming.text}
            streamToolCalls={aiChat.streaming.toolCalls}
          />
        {/if}
      {/if}
    </div>

    <!-- Input -->
    <div class="border-t border-gray-200 dark:border-gray-700 p-3">
      <div class="flex gap-2 items-end max-w-4xl mx-auto">
        <textarea
          bind:this={inputEl}
          bind:value={inputText}
          onkeydown={handleKeydown}
          placeholder="Ask about your invoices, clients, finances..."
          rows="1"
          disabled={aiChat.isStreaming}
          class="flex-1 resize-none rounded-xl border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-800 text-gray-900 dark:text-gray-100 px-4 py-2.5 text-sm focus:outline-none focus:ring-2 focus:ring-blue-500 disabled:opacity-50 min-h-[42px] max-h-32"
          style="overflow-y: auto; field-sizing: content;"
        ></textarea>
        {#if aiChat.isStreaming}
          <button
            onclick={() => aiChat.stopStreaming()}
            class="shrink-0 p-2.5 rounded-xl bg-red-100 dark:bg-red-900/40 text-red-600 dark:text-red-400 hover:bg-red-200 dark:hover:bg-red-900/60 transition-colors"
            aria-label="Stop"
          >
            <svg xmlns="http://www.w3.org/2000/svg" class="w-5 h-5" fill="currentColor" viewBox="0 0 24 24">
              <rect x="6" y="6" width="12" height="12" rx="1" />
            </svg>
          </button>
        {:else}
          <button
            onclick={handleSubmit}
            disabled={!inputText.trim()}
            class="shrink-0 p-2.5 rounded-xl bg-blue-600 text-white hover:bg-blue-700 disabled:opacity-40 disabled:cursor-not-allowed transition-colors"
            aria-label="Send"
          >
            <svg xmlns="http://www.w3.org/2000/svg" class="w-5 h-5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 19l9 2-9-18-9 18 9-2zm0 0v-8" />
            </svg>
          </button>
        {/if}
      </div>
      <p class="text-xs text-center text-gray-400 dark:text-gray-500 mt-1.5">Enter to send · Shift+Enter for newline</p>
    </div>
  </div>
</div>
