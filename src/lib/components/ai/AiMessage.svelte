<script lang="ts">
  import AiToolBadge from './AiToolBadge.svelte';
  import type { AiStreamingState } from '$lib/stores/ai-chat.svelte.js';

  interface Props {
    role: 'user' | 'assistant';
    content: string;
    toolCalls?: string | null;
    isStreaming?: boolean;
    streamText?: string;
    streamToolCalls?: AiStreamingState['toolCalls'];
  }
  let { role, content, toolCalls, isStreaming = false, streamText = '', streamToolCalls = [] }: Props = $props();

  const displayText = $derived(isStreaming ? streamText : content);

  function parseResults(toolResultsStr: string | null): Record<string, { content: string; is_error: boolean }> {
    if (!toolResultsStr) return {};
    try {
      const arr: Array<{ tool_use_id: string; content: string; is_error?: boolean }> = JSON.parse(toolResultsStr);
      return Object.fromEntries(arr.map(r => [r.tool_use_id, { content: r.content, is_error: r.is_error ?? false }]));
    } catch { return {}; }
  }

  function formatContent(text: string): string {
    return text
      .replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
      .replace(/```[\w]*\n([\s\S]*?)```/g, '<pre class="bg-gray-100 dark:bg-gray-800 rounded p-2 text-xs overflow-auto my-1">$1</pre>')
      .replace(/`([^`]+)`/g, '<code class="bg-gray-100 dark:bg-gray-800 rounded px-1 text-xs">$1</code>')
      .replace(/\*\*(.*?)\*\*/g, '<strong>$1</strong>')
      .replace(/\n/g, '<br>');
  }
</script>

<div class="flex {role === 'user' ? 'justify-end' : 'justify-start'} mb-3">
  <div class="max-w-[80%] {role === 'user'
    ? 'bg-blue-600 text-white rounded-2xl rounded-br-sm px-4 py-2'
    : 'bg-gray-100 dark:bg-gray-800 text-gray-900 dark:text-gray-100 rounded-2xl rounded-bl-sm px-4 py-2'}">

    {#if role === 'assistant'}
      <!-- Tool calls from persisted message -->
      {#if toolCalls}
        {@const tcs = (() => { try { return JSON.parse(toolCalls) as Array<{id: string; name: string}>; } catch { return []; } })()}
        {@const trs = parseResults(null)}
        <div class="flex flex-wrap gap-1 mb-2">
          {#each tcs as tc (tc.id)}
            <AiToolBadge name={tc.name} result={trs[tc.id]?.content} is_error={trs[tc.id]?.is_error} />
          {/each}
        </div>
      {/if}
      <!-- Streaming tool calls -->
      {#if isStreaming && streamToolCalls.length > 0}
        <div class="flex flex-wrap gap-1 mb-2">
          {#each streamToolCalls as tc (tc.id)}
            <AiToolBadge name={tc.name} result={tc.result as string | undefined} is_error={tc.is_error} />
          {/each}
        </div>
      {/if}
    {/if}

    {#if displayText}
      <!-- svelte-ignore html_escape_unsafe -->
      <div class="text-sm leading-relaxed prose-sm">{@html formatContent(displayText)}{#if isStreaming}<span class="inline-block w-0.5 h-4 bg-current animate-pulse ml-0.5 align-middle"></span>{/if}</div>
    {/if}
  </div>
</div>
