<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { apiGet, apiPost } from '$lib/api/client';
	import { session } from '$lib/stores/session.svelte';
	import { getFirebaseAuth, getAuthMethods, type AuthMethods } from '$lib/firebase';
	import {
		createUserWithEmailAndPassword,
		signInWithEmailAndPassword,
		updateProfile,
		signInWithPopup,
		GoogleAuthProvider,
		signOut
	} from 'firebase/auth';
	import type { InviteInfo, User } from '$lib/api/types';
	import Button from '$lib/components/Button.svelte';
	import Card from '$lib/components/Card.svelte';
	import Field from '$lib/components/Field.svelte';
	import Receipt from '@lucide/svelte/icons/receipt';

	let token = $state('');
	let invite = $state<InviteInfo | null>(null);
	let methods = $state<AuthMethods | null>(null);
	let name = $state('');
	let password = $state('');
	// New users set a password; returning users (account already exists) sign in.
	let hasAccount = $state(false);
	let error = $state<string | null>(null);
	let loading = $state(true);
	let submitting = $state(false);

	onMount(() => {
		token = page.url.searchParams.get('token') ?? '';
		void boot();
	});

	async function boot(): Promise<void> {
		loading = true;
		error = null;
		try {
			methods = await getAuthMethods();
		} catch {
			// Non-fatal: invite info still loads; sign-in just won't render options.
		}
		if (token === '') {
			error = 'Missing invite token.';
			loading = false;
			return;
		}
		try {
			invite = await apiGet<InviteInfo>(`/api/invites/${encodeURIComponent(token)}`);
			if (invite === null) {
				error = 'This invite is invalid or has expired.';
			}
		} catch {
			error = 'This invite is invalid or has expired.';
		} finally {
			loading = false;
		}
	}

	/** Link the now-signed-in Firebase user to the tenant, then go to it. */
	async function accept(): Promise<void> {
		const user = await apiPost<User>(`/api/invites/${encodeURIComponent(token)}/accept`, { name });
		session.set(user);
		const data = await session.loadSession();
		const first = data?.tenants[0];
		await goto(first ? '/' + first.id + '/' : '/login');
	}

	async function submitEmail(e: SubmitEvent): Promise<void> {
		e.preventDefault();
		if (!invite) return;
		error = null;
		submitting = true;
		try {
			const auth = await getFirebaseAuth();
			if (hasAccount) {
				await signInWithEmailAndPassword(auth, invite.email, password);
			} else {
				const cred = await createUserWithEmailAndPassword(auth, invite.email, password);
				if (name) await updateProfile(cred.user, { displayName: name });
			}
			try {
				await accept();
			} catch (err) {
				await signOut(auth).catch(() => {});
				throw err;
			}
		} catch (err) {
			error = authError(err, 'Failed to accept invite.');
		} finally {
			submitting = false;
		}
	}

	async function acceptGoogle(): Promise<void> {
		error = null;
		submitting = true;
		try {
			const auth = await getFirebaseAuth();
			await signInWithPopup(auth, new GoogleAuthProvider());
			try {
				await accept();
			} catch (err) {
				await signOut(auth).catch(() => {});
				throw err;
			}
		} catch (err) {
			error = authError(err, 'Google sign-in failed.');
		} finally {
			submitting = false;
		}
	}

	function authError(err: unknown, fallback: string): string {
		if (err && typeof err === 'object' && 'code' in err) {
			const code = String((err as { code: unknown }).code);
			if (code === 'auth/email-already-in-use') {
				return 'You already have an account — switch to "I already have an account" to sign in.';
			}
			if (
				code === 'auth/invalid-credential' ||
				code === 'auth/wrong-password' ||
				code === 'auth/user-not-found'
			) {
				return 'Incorrect password for this account.';
			}
			if (code === 'auth/popup-closed-by-user' || code === 'auth/cancelled-popup-request') {
				return 'Sign-in was cancelled.';
			}
		}
		return err instanceof Error ? err.message : fallback;
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
		<h1 class="mb-6 text-xl font-semibold tracking-tight">Accept your invitation</h1>

		{#if loading}
			<p class="text-sm text-gray-500">Loading invite…</p>
		{:else if invite !== null}
			<p class="mb-4 text-sm text-gray-600">
				Joining as <span class="font-medium">{invite.email}</span>
				({invite.role}).
			</p>

			{#if methods?.emailPassword !== false}
				<form class="space-y-4" onsubmit={submitEmail}>
					{#if !hasAccount}
						<Field label="Your name" id="name">
							<input
								id="name"
								type="text"
								bind:value={name}
								required
								autocomplete="name"
								class="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm"
							/>
						</Field>
					{/if}
					<Field
						label={hasAccount ? 'Password' : 'Choose a password'}
						id="password"
						hint={hasAccount ? undefined : 'At least 6 characters.'}
					>
						<input
							id="password"
							type="password"
							bind:value={password}
							required
							minlength="6"
							autocomplete={hasAccount ? 'current-password' : 'new-password'}
							class="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm"
						/>
					</Field>

					{#if error}
						<p class="text-sm text-red-600" role="alert">{error}</p>
					{/if}

					<Button type="submit" loading={submitting} class="w-full">Accept invite</Button>
				</form>

				<button
					type="button"
					class="mt-3 text-sm text-brand-700 hover:text-brand-800"
					onclick={() => {
						hasAccount = !hasAccount;
						error = null;
					}}
				>
					{hasAccount ? 'I need to create an account' : 'I already have an account'}
				</button>
			{/if}

			{#if methods?.google}
				<div class="mt-4">
					<Button
						type="button"
						variant="secondary"
						loading={submitting}
						class="w-full"
						onclick={acceptGoogle}
					>
						Continue with Google
					</Button>
				</div>
				{#if methods?.emailPassword === false && error}
					<p class="mt-4 text-sm text-red-600" role="alert">{error}</p>
				{/if}
			{/if}
		{:else}
			<p class="text-sm text-red-600" role="alert">{error ?? 'Invite not found.'}</p>
		{/if}
	</Card>
</div>
