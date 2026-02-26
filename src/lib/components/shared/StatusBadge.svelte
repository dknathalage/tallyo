<script lang="ts">
	type Status = 'draft' | 'sent' | 'paid' | 'overdue';

	let { status }: { status: Status } = $props();

	const statusClasses: Record<Status, string> = {
		draft: 'bg-gray-100 text-gray-700 dark:bg-gray-700 dark:text-gray-300',
		sent: 'bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-400',
		paid: 'bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400',
		overdue: 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-400'
	};
</script>

<span class="inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-medium {statusClasses[status]}">
	{status.charAt(0).toUpperCase() + status.slice(1)}
</span>
