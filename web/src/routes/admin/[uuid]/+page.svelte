<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import {
		getTenant,
		setSubscription,
		suspendTenant,
		unsuspendTenant,
		deleteTenant,
		SUBSCRIPTION_STATUSES
	} from '$lib/api/admin';
	import type { AdminTenant, AuditRecord } from '$lib/api/admin';
	import { adminTenants } from '$lib/stores/adminTenants.svelte';
	import { ApiError } from '$lib/api/client';
	import Card from '$lib/components/Card.svelte';
	import Button from '$lib/components/Button.svelte';
	import Badge from '$lib/components/Badge.svelte';
	import Field from '$lib/components/Field.svelte';
	import Modal from '$lib/components/Modal.svelte';

	// ── Route param ──────────────────────────────────────────────────────────
	// page.params.uuid is typed as string | undefined; the route guarantees it is
	// present, but we default to '' to satisfy the type checker.
	const uuid = $derived(page.params.uuid ?? '');

	// ── Page state ───────────────────────────────────────────────────────────
	let tenant = $state<AdminTenant | null>(null);
	let auditTrail = $state<AuditRecord[]>([]);
	let loadError = $state<string | null>(null);
	let forbidden = $state(false);

	// Subscription override form
	let subStatus = $state('');
	let subTrialEndsAt = $state('');
	let subBusy = $state(false);
	let subError = $state<string | null>(null);
	let subSuccess = $state<string | null>(null);

	// Suspend / unsuspend
	let suspendBusy = $state(false);
	let suspendError = $state<string | null>(null);
	let suspendSuccess = $state<string | null>(null);

	// Delete confirmation modal
	let deleteOpen = $state(false);
	let deleteConfirmName = $state('');
	let deleteBusy = $state(false);
	let deleteError = $state<string | null>(null);

	// Suspend confirmation modal
	let suspendModalOpen = $state(false);
	let suspendConfirmName = $state('');

	// ── Load ─────────────────────────────────────────────────────────────────
	async function load(): Promise<void> {
		loadError = null;
		forbidden = false;
		try {
			const detail = await getTenant(uuid);
			if (detail) {
				tenant = detail.tenant;
				auditTrail = detail.audit;
				// Seed the subscription form with the current value.
				subStatus = detail.tenant.subscriptionStatus || 'none';
				subTrialEndsAt = detail.tenant.trialEnd
					? detail.tenant.trialEnd.slice(0, 10) // ISO date portion
					: '';
			}
		} catch (e) {
			if (e instanceof ApiError && e.status === 403) {
				forbidden = true;
			} else if (e instanceof ApiError && e.status === 404) {
				loadError = 'Tenant not found.';
			} else {
				loadError = e instanceof Error ? e.message : String(e);
			}
		}
	}

	onMount(() => {
		void load();
	});

	// ── Helpers ───────────────────────────────────────────────────────────────
	function subStatusTone(
		status: string
	): 'green' | 'blue' | 'amber' | 'red' | 'gray' {
		switch (status) {
			case 'active':
				return 'green';
			case 'trialing':
				return 'blue';
			case 'past_due':
				return 'amber';
			case 'canceled':
				return 'red';
			default:
				return 'gray';
		}
	}

	function tenantStatusTone(status: string): 'red' | 'green' | 'gray' {
		if (status === 'suspended') return 'red';
		if (status === 'active') return 'green';
		return 'gray';
	}

	function fmt(iso: string | undefined | null): string {
		if (!iso) return '—';
		const d = new Date(iso);
		return isNaN(d.getTime()) ? iso : d.toLocaleDateString();
	}

	// ── Subscription override ─────────────────────────────────────────────────
	async function handleSetSubscription(): Promise<void> {
		if (!subStatus) return;
		subBusy = true;
		subError = null;
		subSuccess = null;
		try {
			await setSubscription(uuid, {
				status: subStatus,
				...(subStatus === 'trialing' && subTrialEndsAt ? { trialEndsAt: subTrialEndsAt } : {})
			});
			subSuccess = `Subscription status set to "${subStatus}".`;
			// Refresh tenant data and invalidate list cache.
			adminTenants.invalidate();
			await load();
		} catch (e) {
			subError = e instanceof Error ? e.message : String(e);
		} finally {
			subBusy = false;
		}
	}

	// ── Suspend / unsuspend ───────────────────────────────────────────────────
	const isSuspended = $derived(tenant?.status === 'suspended');

	async function handleSuspend(): Promise<void> {
		suspendModalOpen = false;
		suspendConfirmName = '';
		suspendBusy = true;
		suspendError = null;
		suspendSuccess = null;
		try {
			await suspendTenant(uuid);
			suspendSuccess = 'Tenant suspended.';
			adminTenants.invalidate();
			await load();
		} catch (e) {
			suspendError = e instanceof Error ? e.message : String(e);
		} finally {
			suspendBusy = false;
		}
	}

	async function handleUnsuspend(): Promise<void> {
		suspendBusy = true;
		suspendError = null;
		suspendSuccess = null;
		try {
			await unsuspendTenant(uuid);
			suspendSuccess = 'Tenant unsuspended.';
			adminTenants.invalidate();
			await load();
		} catch (e) {
			suspendError = e instanceof Error ? e.message : String(e);
		} finally {
			suspendBusy = false;
		}
	}

	// ── Delete ────────────────────────────────────────────────────────────────
	const deleteNameMatches = $derived(
		tenant !== null && deleteConfirmName.trim() === tenant.name.trim()
	);

	async function handleDelete(): Promise<void> {
		if (!deleteNameMatches) return;
		deleteBusy = true;
		deleteError = null;
		try {
			await deleteTenant(uuid);
			deleteOpen = false;
			adminTenants.invalidate();
			// Navigate back to the admin list after deletion.
			await goto('/admin');
		} catch (e) {
			deleteError = e instanceof Error ? e.message : String(e);
			deleteBusy = false;
		}
	}

	// Reset confirm inputs when modals close.
	$effect(() => {
		if (!deleteOpen) {
			deleteConfirmName = '';
			deleteError = null;
		}
	});
	$effect(() => {
		if (!suspendModalOpen) {
			suspendConfirmName = '';
		}
	});

	// Suspend modal: match name
	const suspendNameMatches = $derived(
		tenant !== null && suspendConfirmName.trim() === tenant.name.trim()
	);
</script>

{#if forbidden}
	<div class="rounded-lg border border-red-300 bg-red-50 px-4 py-3 text-sm text-red-800" role="alert">
		Access denied — you must be a platform admin to view this page.
	</div>
{:else if loadError}
	<div class="rounded-lg border border-red-300 bg-red-50 px-4 py-3 text-sm text-red-800" role="alert">
		{loadError}
	</div>
{:else if !tenant}
	<p class="text-sm text-gray-500">Loading…</p>
{:else}
	<!-- Back link -->
	<nav class="mb-4">
		<a href="/admin" class="text-sm text-brand-700 hover:underline">← All tenants</a>
	</nav>

	<div class="space-y-6">
		<!-- Tenant info -->
		<Card>
			<div class="mb-4 flex items-start justify-between gap-4">
				<div>
					<h1 class="text-2xl font-semibold tracking-tight">{tenant.name}</h1>
					<p class="mt-1 font-mono text-xs text-gray-500">{tenant.id}</p>
				</div>
				<div class="flex shrink-0 flex-col items-end gap-1.5">
					<Badge tone={tenantStatusTone(tenant.status)}>{tenant.status}</Badge>
					<Badge tone={subStatusTone(tenant.subscriptionStatus)}>
						{tenant.subscriptionStatus || 'none'}
					</Badge>
				</div>
			</div>

			<dl class="grid gap-x-6 gap-y-3 text-sm sm:grid-cols-2 lg:grid-cols-3">
				<div>
					<dt class="text-gray-500">Created</dt>
					<dd class="font-medium">{fmt(tenant.createdAt)}</dd>
				</div>
				<div>
					<dt class="text-gray-500">Updated</dt>
					<dd class="font-medium">{fmt(tenant.updatedAt)}</dd>
				</div>
				<div>
					<dt class="text-gray-500">Trial ends</dt>
					<dd class="font-medium">{fmt(tenant.trialEnd)}</dd>
				</div>
				<div>
					<dt class="text-gray-500">Current period ends</dt>
					<dd class="font-medium">{fmt(tenant.currentPeriodEnd)}</dd>
				</div>
				{#if tenant.stripeCustomerId}
					<div>
						<dt class="text-gray-500">Stripe customer</dt>
						<dd class="font-mono text-xs">{tenant.stripeCustomerId}</dd>
					</div>
				{/if}
				{#if tenant.stripeSubscriptionId}
					<div>
						<dt class="text-gray-500">Stripe subscription</dt>
						<dd class="font-mono text-xs">{tenant.stripeSubscriptionId}</dd>
					</div>
				{/if}
			</dl>
		</Card>

		<!-- Subscription override -->
		<Card>
			<h2 class="mb-4 text-base font-semibold">Override subscription</h2>
			<form
				class="space-y-4"
				onsubmit={(e) => {
					e.preventDefault();
					void handleSetSubscription();
				}}
			>
				<Field label="Subscription status" id="sub-status" required>
					<select
						id="sub-status"
						bind:value={subStatus}
						class="w-full rounded-lg border border-gray-300 px-2.5 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-brand-500"
					>
						{#each SUBSCRIPTION_STATUSES as s (s)}
							<option value={s}>{s}</option>
						{/each}
					</select>
				</Field>

				{#if subStatus === 'trialing'}
					<Field label="Trial ends at" id="sub-trial-ends">
						<input
							id="sub-trial-ends"
							type="date"
							bind:value={subTrialEndsAt}
							class="w-full rounded-lg border border-gray-300 px-2.5 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-brand-500"
						/>
					</Field>
				{/if}

				{#if subError}
					<p class="text-sm text-red-600" role="alert">{subError}</p>
				{/if}
				{#if subSuccess}
					<p class="text-sm text-green-700" role="status">{subSuccess}</p>
				{/if}

				<Button type="submit" loading={subBusy} disabled={subBusy || !subStatus}>
					Save subscription
				</Button>
			</form>
		</Card>

		<!-- Suspend / unsuspend -->
		<Card>
			<h2 class="mb-4 text-base font-semibold">Tenant access</h2>

			{#if suspendError}
				<p class="mb-3 text-sm text-red-600" role="alert">{suspendError}</p>
			{/if}
			{#if suspendSuccess}
				<p class="mb-3 text-sm text-green-700" role="status">{suspendSuccess}</p>
			{/if}

			{#if isSuspended}
				<p class="mb-3 text-sm text-gray-600">
					This tenant is <strong>suspended</strong>. All users are blocked from logging in.
				</p>
				<Button
					variant="secondary"
					loading={suspendBusy}
					disabled={suspendBusy}
					onclick={() => void handleUnsuspend()}
				>
					Unsuspend tenant
				</Button>
			{:else}
				<p class="mb-3 text-sm text-gray-600">
					Suspending will block all users from logging in. You can unsuspend at any time.
				</p>
				<Button
					variant="danger"
					loading={suspendBusy}
					disabled={suspendBusy}
					onclick={() => (suspendModalOpen = true)}
				>
					Suspend tenant
				</Button>
			{/if}
		</Card>

		<!-- Audit trail -->
		<Card padded={false}>
			<div class="border-b border-gray-200 px-5 py-4">
				<h2 class="text-base font-semibold">Audit trail</h2>
				<p class="mt-0.5 text-xs text-gray-500">Most recent 50 events, newest first.</p>
			</div>
			{#if auditTrail.length === 0}
				<p class="px-5 py-4 text-sm text-gray-400">No audit events.</p>
			{:else}
				<ul class="divide-y divide-gray-100">
					{#each auditTrail as record (record.id)}
						<li class="px-5 py-3 text-sm">
							<div class="flex items-start justify-between gap-4">
								<div class="min-w-0">
									<span class="font-medium text-gray-900">{record.action}</span>
									{#if record.entityType}
										<span class="text-gray-500"> on {record.entityType}</span>
									{/if}
									{#if record.entityId}
										<span class="ml-1 font-mono text-xs text-gray-400">{record.entityId}</span>
									{/if}
									{#if record.userId}
										<p class="mt-0.5 text-xs text-gray-400">by {record.userId}</p>
									{/if}
									{#if record.changes}
										<pre class="mt-1 overflow-x-auto rounded bg-gray-50 px-2 py-1 text-xs text-gray-600">{record.changes}</pre>
									{/if}
								</div>
								<time
									datetime={record.createdAt}
									class="shrink-0 text-xs text-gray-400"
									title={record.createdAt}
								>
									{fmt(record.createdAt)}
								</time>
							</div>
						</li>
					{/each}
				</ul>
			{/if}
		</Card>

		<!-- Danger zone: delete -->
		<Card>
			<h2 class="mb-1 text-base font-semibold text-red-700">Danger zone</h2>
			<p class="mb-4 text-sm text-gray-600">
				Permanently delete this tenant and all its data. This action is irreversible.
			</p>
			<Button variant="danger" onclick={() => (deleteOpen = true)}>Delete tenant</Button>
		</Card>
	</div>
{/if}

<!-- ── Suspend confirmation modal ───────────────────────────────────────── -->
<Modal bind:open={suspendModalOpen} title="Suspend tenant">
	<p class="mb-4 text-sm text-gray-700">
		Type <strong>{tenant?.name}</strong> to confirm you want to suspend this tenant.
		All users will be blocked from logging in.
	</p>
	<Field label="Confirm tenant name" id="suspend-confirm-name">
		<input
			id="suspend-confirm-name"
			type="text"
			placeholder={tenant?.name ?? ''}
			bind:value={suspendConfirmName}
			autocomplete="off"
			class="w-full rounded-lg border border-gray-300 px-2.5 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-brand-500"
		/>
	</Field>

	{#snippet footer()}
		<Button variant="secondary" onclick={() => (suspendModalOpen = false)}>Cancel</Button>
		<Button
			variant="danger"
			disabled={!suspendNameMatches || suspendBusy}
			loading={suspendBusy}
			onclick={() => void handleSuspend()}
		>
			Suspend
		</Button>
	{/snippet}
</Modal>

<!-- ── Delete confirmation modal ────────────────────────────────────────── -->
<Modal bind:open={deleteOpen} title="Delete tenant">
	<p class="mb-4 text-sm text-gray-700">
		This will permanently delete <strong>{tenant?.name}</strong> and all its data.
		This action <strong>cannot be undone</strong>. Type the tenant name to confirm.
	</p>
	<Field label="Confirm tenant name" id="delete-confirm-name">
		<input
			id="delete-confirm-name"
			type="text"
			placeholder={tenant?.name ?? ''}
			bind:value={deleteConfirmName}
			autocomplete="off"
			class="w-full rounded-lg border border-gray-300 px-2.5 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-brand-500"
		/>
	</Field>

	{#if deleteError}
		<p class="mt-3 text-sm text-red-600" role="alert">{deleteError}</p>
	{/if}

	{#snippet footer()}
		<Button variant="secondary" onclick={() => (deleteOpen = false)} disabled={deleteBusy}>
			Cancel
		</Button>
		<Button
			variant="danger"
			disabled={!deleteNameMatches || deleteBusy}
			loading={deleteBusy}
			onclick={() => void handleDelete()}
		>
			Delete permanently
		</Button>
	{/snippet}
</Modal>
