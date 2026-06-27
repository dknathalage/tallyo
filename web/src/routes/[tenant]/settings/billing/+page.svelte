<script lang="ts">
	import { onMount } from 'svelte';
	import { billing } from '$lib/stores/billing.svelte';
	import { session } from '$lib/stores/session.svelte';
	import { startCheckout, openPortal, trialDaysLeft } from '$lib/api/billing';
	import Button from '$lib/components/Button.svelte';
	import Card from '$lib/components/Card.svelte';

	const isOwner = $derived(session.isOwner);
	let busy = $state(false);
	let error = $state<string | null>(null);

	onMount(() => {
		void billing.load();
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
			const url = await startCheckout();
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
			<Button onclick={subscribe} loading={busy} disabled={busy}>Subscribe</Button>
		{/if}
	</Card>
</section>
