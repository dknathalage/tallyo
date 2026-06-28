<script lang="ts">
	import { onMount } from 'svelte';
	import { billing } from '$lib/stores/billing.svelte';
	import { session } from '$lib/stores/session.svelte';
	import { startCheckout, openPortal, trialDaysLeft } from '$lib/api/billing';
	import { planFor } from '$lib/pricing';
	import Button from '$lib/components/Button.svelte';
	import Card from '$lib/components/Card.svelte';

	const isOwner = $derived(session.isOwner);
	let busy = $state(false);
	let error = $state<string | null>(null);
	// Billing cadence sent to Checkout. Pre-seeded from the landing-page toggle
	// (sessionStorage 'tallyo_plan'), but the user can change it here — this page
	// is the source of truth for what checkout receives.
	let annual = $state(false);

	onMount(() => {
		void billing.load();
		annual = sessionStorage.getItem('tallyo_plan') === 'annual';
	});

	const status = $derived(billing.status);
	const label = $derived.by(() => {
		switch (status?.status) {
			case 'active':
				return 'Active';
			case 'trialing':
				return `Trial — ${trialDaysLeft(status.trialEnd)} day(s) left`;
			case 'past_due':
				return 'Payment overdue';
			case 'canceled':
				return 'Canceled';
			default:
				return 'No subscription';
		}
	});
	// Show "Subscribe" until there is a Stripe customer to manage (active/past_due
	// /canceled went through Checkout once → use the Portal instead).
	const canManage = $derived(status?.status === 'active' || status?.status === 'past_due');

	async function subscribe(): Promise<void> {
		busy = true;
		error = null;
		try {
			const url = await startCheckout(annual ? 'annual' : 'monthly');
			if (url) window.location.assign(url);
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not start checkout';
		} finally {
			busy = false;
		}
	}

	async function manage(): Promise<void> {
		busy = true;
		error = null;
		try {
			const url = await openPortal();
			if (url) window.location.assign(url);
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not open billing portal';
		} finally {
			busy = false;
		}
	}
</script>

<section>
	<h1 class="mb-1 text-2xl font-semibold tracking-tight">Billing</h1>
	<p class="mb-6 text-sm text-gray-500">Manage your Tallyo subscription.</p>

	<Card class="max-w-lg space-y-4">
		<div>
			<p class="text-sm text-gray-500">Status</p>
			<p class="text-lg font-medium">{label}</p>
		</div>

		{#if error}
			<p class="text-sm text-red-600">{error}</p>
		{/if}

		{#if !isOwner}
			<p class="text-sm text-gray-500">Only the account owner can manage billing.</p>
		{:else if canManage}
			<Button onclick={manage} loading={busy} disabled={busy}>Manage billing</Button>
		{:else}
			<div class="inline-flex items-center gap-1 rounded-lg border border-gray-200 p-1 text-sm">
				<button
					type="button"
					onclick={() => (annual = false)}
					class="rounded-md px-3 py-1 font-medium transition-colors
						{!annual ? 'bg-brand-700 text-onbrand' : 'text-gray-600 hover:text-gray-900'}"
					aria-pressed={!annual}>Monthly</button
				>
				<button
					type="button"
					onclick={() => (annual = true)}
					class="rounded-md px-3 py-1 font-medium transition-colors
						{annual ? 'bg-brand-700 text-onbrand' : 'text-gray-600 hover:text-gray-900'}"
					aria-pressed={annual}>Annual</button
				>
			</div>
			<p class="text-sm text-gray-500">
				{planFor(annual).price}{planFor(annual).period}
			</p>
			<Button onclick={subscribe} loading={busy} disabled={busy}>Subscribe</Button>
		{/if}
	</Card>
</section>
