<script lang="ts">
	import { i18n } from '$lib/stores/i18n.svelte.js';

	type Status = 'draft' | 'sent' | 'paid' | 'overdue' | 'accepted' | 'rejected' | 'expired';

	const { status }: { status: Status } = $props();

	const statusClasses: Record<Status, string> = {
		draft: 'bg-gray-100 text-gray-700 dark:bg-gray-700 dark:text-gray-300',
		sent: 'bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-400',
		paid: 'bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400',
		overdue: 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-400',
		accepted: 'bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400',
		rejected: 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-400',
		expired: 'bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-400'
	};
</script>

<span class="inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-medium {statusClasses[status]}">
	{i18n.t(`status.${status}`)}
</span>
