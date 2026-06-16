<script lang="ts">
	import ChatPane from '$lib/components/agent/ChatPane.svelte';
	import ConversationList from '$lib/components/agent/ConversationList.svelte';
</script>

<div class="flex h-[calc(100vh-4rem)] gap-4 overflow-hidden">
	<aside class="w-56 shrink-0 overflow-hidden rounded-lg border border-gray-200 bg-white">
		<ConversationList />
	</aside>
	<section class="flex-1 overflow-hidden rounded-lg border border-gray-200 bg-white">
		<ChatPane />
	</section>
</div>
