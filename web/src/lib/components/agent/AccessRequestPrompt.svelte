<script lang="ts">
	import { agentChat } from '$lib/stores/agentChat.svelte';
	import type { AccessRequestInfo } from '$lib/stores/agentChatReducer';

	interface Props {
		access: AccessRequestInfo;
	}

	let { access }: Props = $props();

	// Guard: disable both buttons after the first click so only one action fires.
	let decided = $state(false);

	const prettyInput = $derived(JSON.stringify(access.input, null, 2));

	// Countdown — recomputed every second via a derived + interval.
	let now = $state(Date.now());

	// Update `now` every second.
	$effect(() => {
		const id = setInterval(() => {
			now = Date.now();
		}, 1000);
		return () => clearInterval(id);
	});

	const expiresMs = $derived(Date.parse(access.expiresAt));
	const remainingSeconds = $derived(Math.max(0, Math.floor((expiresMs - now) / 1000)));
	const expired = $derived(now >= expiresMs);

	const countdownLabel = $derived(
		expired
			? 'expired'
			: remainingSeconds >= 60
				? `${Math.floor(remainingSeconds / 60)}m ${remainingSeconds % 60}s`
				: `${remainingSeconds}s`
	);

	function allow(): void {
		if (decided) return;
		decided = true;
		agentChat.decide(access.stepId, true);
	}

	function deny(): void {
		if (decided) return;
		decided = true;
		agentChat.decide(access.stepId, false);
	}
</script>

<div
	class="rounded-lg border-2 border-amber-400 bg-amber-50 px-4 py-3 shadow-sm"
	role="alertdialog"
	aria-label="Access request: {access.toolName}"
>
	<!-- Header -->
	<div class="mb-2 flex items-center justify-between gap-2">
		<p class="text-sm font-semibold text-amber-900">{access.summary}</p>
		<span
			class="shrink-0 rounded px-2 py-0.5 text-xs font-medium
				{expired ? 'bg-red-100 text-red-700' : 'bg-amber-100 text-amber-700'}"
			aria-live="polite"
		>
			{countdownLabel}
		</span>
	</div>

	<!-- Tool name -->
	<p class="mb-2 text-xs text-amber-700">
		Tool: <span class="font-mono font-semibold">{access.toolName}</span>
	</p>

	<!-- Input preview -->
	<pre
		class="mb-3 max-h-40 overflow-auto rounded border border-amber-200 bg-white px-3 py-2 text-xs text-gray-800 whitespace-pre-wrap break-all"
	>{prettyInput}</pre>

	<!-- Action buttons -->
	<div class="flex gap-2">
		<button
			type="button"
			onclick={allow}
			disabled={decided || expired}
			class="rounded border border-green-300 bg-green-100 px-3 py-1.5 text-sm font-medium text-green-800
				hover:bg-green-200 disabled:cursor-not-allowed disabled:opacity-50"
		>
			Allow
		</button>
		<button
			type="button"
			onclick={deny}
			disabled={decided || expired}
			class="rounded border border-red-300 bg-red-100 px-3 py-1.5 text-sm font-medium text-red-800
				hover:bg-red-200 disabled:cursor-not-allowed disabled:opacity-50"
		>
			Deny
		</button>
	</div>
</div>
