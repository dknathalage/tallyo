<script lang="ts">
	import { goto } from '$app/navigation';
	import { apiPost, ApiError } from '$lib/api/client';
	import { session } from '$lib/stores/session.svelte';
	import type { User, EmailTenant } from '$lib/api/types';
	import Button from '$lib/components/Button.svelte';
	import Card from '$lib/components/Card.svelte';
	import Field from '$lib/components/Field.svelte';
	import Receipt from '@lucide/svelte/icons/receipt';

	let email = $state('');
	let password = $state('');
	let error = $state<string | null>(null);
	let submitting = $state(false);

	// Tenant-disambiguation state: when the email spans multiple tenants the API
	// answers 409 with the candidate tenants; we render a picker and re-POST with
	// the chosen tenant's uuid (the 409 body's `id`).
	let tenantChoices = $state<EmailTenant[]>([]);
	let selectedTenantId = $state<string>('');

	async function attempt(tenantId?: string): Promise<void> {
		error = null;
		submitting = true;
		try {
			const body: { email: string; password: string; tenantId?: string } = { email, password };
			if (tenantId !== undefined) body.tenantId = tenantId;
			const user = await apiPost<User>('/api/auth/login', body);
			if (user === null) {
				error = 'Invalid email or password.';
				return;
			}
			session.set(user);
			tenantChoices = [];
			// The login response is the per-tenant User; fetch the agnostic session
			// and land on the first member tenant to avoid a root-redirect flash.
			const info = await session.loadSession();
			const first = info?.tenants[0];
			await goto(first ? '/' + first.id + '/' : '/');
		} catch (err) {
			if (err instanceof ApiError && err.tenantRequired) {
				// Multiple tenants share this email: prompt the user to choose one.
				tenantChoices = err.tenants;
				selectedTenantId = err.tenants.length > 0 ? err.tenants[0].id : '';
				return;
			}
			if (err instanceof ApiError && err.status === 403) {
				error = 'This account is suspended. Please contact support.';
				return;
			}
			error = err instanceof Error ? err.message : 'Invalid email or password.';
		} finally {
			submitting = false;
		}
	}

	async function submit(e: SubmitEvent): Promise<void> {
		e.preventDefault();
		await attempt();
	}

	async function submitWithTenant(e: SubmitEvent): Promise<void> {
		e.preventDefault();
		if (selectedTenantId === '') {
			error = 'Please select an organisation.';
			return;
		}
		await attempt(selectedTenantId);
	}
</script>

<div class="mx-auto flex min-h-screen max-w-sm flex-col justify-center px-4 py-12">
	<a href="/login" class="mb-6 flex items-center justify-center gap-2">
		<span class="flex size-8 items-center justify-center rounded-lg bg-brand-700 text-onbrand">
			<Receipt class="size-5" aria-hidden="true" />
		</span>
		<span class="text-xl font-semibold tracking-tight text-brand-700">Tallyo</span>
	</a>

	<Card>
		<h1 class="mb-6 text-xl font-semibold tracking-tight">Sign in to Tallyo</h1>

		{#if tenantChoices.length > 0}
			<p class="mb-4 text-sm text-gray-600">
				Your email belongs to more than one organisation. Choose which one to sign in to.
			</p>
			<form class="space-y-4" onsubmit={submitWithTenant}>
				<Field label="Organisation" id="tenant">
					<select
						id="tenant"
						bind:value={selectedTenantId}
						class="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm"
					>
						{#each tenantChoices as t (t.id)}
							<option value={t.id}>{t.tenantName}</option>
						{/each}
					</select>
				</Field>

				{#if error}
					<p class="text-sm text-red-600" role="alert">{error}</p>
				{/if}

				<Button type="submit" loading={submitting} class="w-full">Continue</Button>
				<Button
					type="button"
					variant="secondary"
					class="w-full"
					onclick={() => {
						tenantChoices = [];
						error = null;
					}}
				>
					Back
				</Button>
			</form>
		{:else}
			<form class="space-y-4" onsubmit={submit}>
				<Field label="Email" id="email">
					<input
						id="email"
						type="email"
						bind:value={email}
						required
						autocomplete="email"
						class="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm"
					/>
				</Field>
				<Field label="Password" id="password">
					<input
						id="password"
						type="password"
						bind:value={password}
						required
						autocomplete="current-password"
						class="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm"
					/>
				</Field>

				{#if error}
					<p class="text-sm text-red-600" role="alert">{error}</p>
				{/if}

				<Button type="submit" loading={submitting} class="w-full">Sign in</Button>
			</form>

			<p class="mt-4 text-center text-sm text-gray-500">
				New to Tallyo? <a href="/signup" class="font-medium text-brand-700 hover:text-brand-800"
					>Create an account</a
				>
			</p>
		{/if}
	</Card>
</div>
