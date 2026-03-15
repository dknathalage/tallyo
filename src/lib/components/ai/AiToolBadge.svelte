<script lang="ts">
  interface Props {
    name: string;
    result?: string;
    is_error?: boolean;
  }
  let { name, result, is_error = false }: Props = $props();
  let expanded = $state(false);
  const done = $derived(result !== undefined);
</script>

<div class="inline-flex items-center gap-1 text-xs rounded-full px-2 py-0.5 border my-0.5
  {done ? (is_error ? 'border-red-300 bg-red-50 dark:border-red-700 dark:bg-red-950' : 'border-green-300 bg-green-50 dark:border-green-700 dark:bg-green-950') : 'border-blue-300 bg-blue-50 dark:border-blue-700 dark:bg-blue-950'}">
  {#if !done}
    <span class="inline-block w-3 h-3 border-2 border-blue-400 border-t-transparent rounded-full animate-spin"></span>
  {:else if is_error}
    <span class="text-red-500">✗</span>
  {:else}
    <span class="text-green-500">✓</span>
  {/if}
  <button onclick={() => expanded = !expanded} class="font-mono hover:underline">
    {name}
  </button>
  {#if expanded && result}
    <pre class="mt-1 text-xs overflow-auto max-h-32 p-1 rounded bg-white/50 dark:bg-black/20 w-full">{result}</pre>
  {/if}
</div>
