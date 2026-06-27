<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { session } from '$lib/stores/session.svelte';
	import { getFirebaseAuth, getAuthMethods, type AuthMethods } from '$lib/firebase';
	import {
		signInWithEmailAndPassword,
		signInWithPopup,
		GoogleAuthProvider,
		sendSignInLinkToEmail,
		isSignInWithEmailLink,
		signInWithEmailLink
	} from 'firebase/auth';
	import {
		isAccountExistsWithDifferentCredential,
		pendingLinkFromError,
		linkWithPassword,
		PASSWORD_METHOD,
		EMAIL_LINK_METHOD,
		type PendingLink
	} from '$lib/auth-link';
	import Button from '$lib/components/Button.svelte';
	import Card from '$lib/components/Card.svelte';
	import Field from '$lib/components/Field.svelte';
	import Receipt from '@lucide/svelte/icons/receipt';

	// localStorage key for the email-link flow (the email is needed to complete
	// sign-in when the user clicks the link in their inbox).
	const EMAIL_LINK_KEY = 'tallyo.emailForSignIn';

	let methods = $state<AuthMethods | null>(null);
	let email = $state('');
	let password = $state('');
	let error = $state<string | null>(null);
	let info = $state<string | null>(null);
	let submitting = $state(false);

	// Tenant-disambiguation: after a successful Firebase sign-in we load the
	// agnostic session; if the account spans multiple tenants we let the user pick.
	let tenantChoices = $state<{ id: string; tenantName: string; role: string }[]>([]);
	let selectedTenantId = $state<string>('');

	// Account-linking: a Google popup against an email that already has a
	// password/email-link account throws account-exists-with-different-credential.
	// We capture the pending Google cred and ask the user to authenticate with
	// their existing method, then link Google onto the same uid.
	let pendingLink = $state<PendingLink | null>(null);
	let linkPassword = $state('');
	let linkInfo = $state<string | null>(null);
	// The existing method to re-auth with: 'password' or 'emailLink'.
	const linkMethod = $derived.by<string>(() => {
		if (!pendingLink) return '';
		if (pendingLink.methods.includes(PASSWORD_METHOD)) return PASSWORD_METHOD;
		if (pendingLink.methods.includes(EMAIL_LINK_METHOD)) return EMAIL_LINK_METHOD;
		return pendingLink.methods[0] ?? '';
	});

	onMount(() => {
		void boot();
	});

	async function boot(): Promise<void> {
		try {
			methods = await getAuthMethods();
		} catch {
			error = 'Could not load sign-in options. Please try again.';
			return;
		}
		// Complete an email-link sign-in if we landed here from the magic link.
		if (methods.emailLink) {
			const auth = await getFirebaseAuth();
			if (isSignInWithEmailLink(auth, window.location.href)) {
				await completeEmailLink();
			}
		}
	}

	/** After Firebase sign-in succeeds, load the session and land on a tenant. */
	async function afterSignIn(): Promise<void> {
		const data = await session.loadSession();
		const tenants = data?.tenants ?? [];
		if (tenants.length === 0) {
			// Signed in but no tenant membership — send to root, which will handle it.
			await goto('/');
			return;
		}
		if (tenants.length === 1) {
			await goto('/' + tenants[0].id + '/');
			return;
		}
		// Multiple tenants: let the user choose.
		tenantChoices = tenants;
		selectedTenantId = tenants[0].id;
	}

	async function submitPassword(e: SubmitEvent): Promise<void> {
		e.preventDefault();
		error = null;
		submitting = true;
		try {
			const auth = await getFirebaseAuth();
			await signInWithEmailAndPassword(auth, email, password);
			await afterSignIn();
		} catch (err) {
			error = authError(err, 'Invalid email or password.');
		} finally {
			submitting = false;
		}
	}

	async function signInGoogle(): Promise<void> {
		error = null;
		submitting = true;
		try {
			const auth = await getFirebaseAuth();
			await signInWithPopup(auth, new GoogleAuthProvider());
			await afterSignIn();
		} catch (err) {
			if (isAccountExistsWithDifferentCredential(err)) {
				// Same email already uses another method: capture the pending Google
				// credential and switch to the linking panel.
				const auth = await getFirebaseAuth();
				const pl = await pendingLinkFromError(auth, err);
				if (pl) {
					pendingLink = pl;
					email = pl.email;
					return;
				}
			}
			error = authError(err, 'Google sign-in failed.');
		} finally {
			submitting = false;
		}
	}

	/** User entered their existing password — sign in, link Google, continue. */
	async function resolveLinkPassword(e: SubmitEvent): Promise<void> {
		e.preventDefault();
		if (!pendingLink) return;
		error = null;
		submitting = true;
		try {
			const auth = await getFirebaseAuth();
			await linkWithPassword(auth, pendingLink, linkPassword);
			pendingLink = null;
			linkPassword = '';
			await afterSignIn();
		} catch (err) {
			error = authError(err, 'Could not link your Google account.');
		} finally {
			submitting = false;
		}
	}

	/**
	 * Existing method is email-link: send the magic link. The pending Google cred
	 * cannot survive the redirect, so after the user signs in via the link we ask
	 * them to retry Google once — at which point sign-in succeeds (the email is now
	 * the current user) and Google links automatically on the provider's side.
	 */
	async function resolveLinkEmailLink(): Promise<void> {
		if (!pendingLink) return;
		error = null;
		submitting = true;
		try {
			const auth = await getFirebaseAuth();
			await sendSignInLinkToEmail(auth, pendingLink.email, {
				url: window.location.origin + '/login',
				handleCodeInApp: true
			});
			window.localStorage.setItem(EMAIL_LINK_KEY, pendingLink.email);
			linkInfo = `We emailed a sign-in link to ${pendingLink.email}. Open it, then choose "Continue with Google" again to finish linking.`;
		} catch (err) {
			error = authError(err, 'Could not send the sign-in link.');
		} finally {
			submitting = false;
		}
	}

	function cancelLink(): void {
		pendingLink = null;
		linkPassword = '';
		linkInfo = null;
		error = null;
	}

	async function sendEmailLink(e: SubmitEvent): Promise<void> {
		e.preventDefault();
		error = null;
		info = null;
		submitting = true;
		try {
			const auth = await getFirebaseAuth();
			await sendSignInLinkToEmail(auth, email, {
				url: window.location.origin + '/login',
				handleCodeInApp: true
			});
			window.localStorage.setItem(EMAIL_LINK_KEY, email);
			info = `We emailed a sign-in link to ${email}. Open it on this device to continue.`;
		} catch (err) {
			error = authError(err, 'Could not send the sign-in link.');
		} finally {
			submitting = false;
		}
	}

	async function completeEmailLink(): Promise<void> {
		submitting = true;
		error = null;
		try {
			let savedEmail = window.localStorage.getItem(EMAIL_LINK_KEY);
			if (!savedEmail) {
				// Opened on a different device: ask for the email to confirm.
				savedEmail = window.prompt('Please confirm your email to finish signing in') ?? '';
			}
			if (!savedEmail) {
				error = 'Email is required to complete sign-in.';
				return;
			}
			const auth = await getFirebaseAuth();
			await signInWithEmailLink(auth, savedEmail, window.location.href);
			window.localStorage.removeItem(EMAIL_LINK_KEY);
			await afterSignIn();
		} catch (err) {
			error = authError(err, 'This sign-in link is invalid or has expired.');
		} finally {
			submitting = false;
		}
	}

	async function continueWithTenant(e: SubmitEvent): Promise<void> {
		e.preventDefault();
		if (selectedTenantId === '') {
			error = 'Please select an organisation.';
			return;
		}
		await goto('/' + selectedTenantId + '/');
	}

	/** Map a Firebase auth error to a friendly message. */
	function authError(err: unknown, fallback: string): string {
		if (err && typeof err === 'object' && 'code' in err) {
			const code = String((err as { code: unknown }).code);
			if (code === 'auth/popup-closed-by-user' || code === 'auth/cancelled-popup-request') {
				return 'Sign-in was cancelled.';
			}
			if (
				code === 'auth/invalid-credential' ||
				code === 'auth/wrong-password' ||
				code === 'auth/user-not-found'
			) {
				return 'Invalid email or password.';
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
		<h1 class="mb-6 text-xl font-semibold tracking-tight">Sign in to Tallyo</h1>

		{#if pendingLink !== null}
			<p class="mb-4 text-sm text-gray-600">
				You already have a Tallyo account for
				<span class="font-medium">{pendingLink.email}</span>. Sign in with it to link your Google
				account.
			</p>

			{#if linkMethod === PASSWORD_METHOD}
				<form class="space-y-4" onsubmit={resolveLinkPassword}>
					<Field label="Password" id="link-password">
						<input
							id="link-password"
							type="password"
							bind:value={linkPassword}
							required
							autocomplete="current-password"
							class="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm"
						/>
					</Field>
					{#if error}
						<p class="text-sm text-red-600" role="alert">{error}</p>
					{/if}
					<Button type="submit" loading={submitting} class="w-full">Link Google account</Button>
				</form>
			{:else if linkMethod === EMAIL_LINK_METHOD}
				<Button
					type="button"
					loading={submitting}
					class="w-full"
					onclick={resolveLinkEmailLink}
				>
					Email me a sign-in link
				</Button>
				{#if linkInfo}
					<p class="mt-4 text-sm text-green-700" role="status">{linkInfo}</p>
				{/if}
				{#if error}
					<p class="mt-4 text-sm text-red-600" role="alert">{error}</p>
				{/if}
			{:else}
				<p class="text-sm text-red-600" role="alert">
					This email uses a sign-in method we can't link automatically. Please sign in with that
					method first.
				</p>
			{/if}

			<Button type="button" variant="secondary" class="mt-3 w-full" onclick={cancelLink}>
				Cancel
			</Button>
		{:else if tenantChoices.length > 0}
			<p class="mb-4 text-sm text-gray-600">
				Your account belongs to more than one organisation. Choose which one to open.
			</p>
			<form class="space-y-4" onsubmit={continueWithTenant}>
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

				<Button type="submit" class="w-full">Continue</Button>
			</form>
		{:else if methods === null}
			<p class="text-sm text-gray-500">Loading sign-in options…</p>
		{:else}
			{#if methods.emailPassword}
				<form class="space-y-4" onsubmit={submitPassword}>
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
					<Button type="submit" loading={submitting} class="w-full">Sign in</Button>
				</form>
			{/if}

			{#if methods.emailLink}
				<form class="mt-4 space-y-4" onsubmit={sendEmailLink}>
					{#if !methods.emailPassword}
						<Field label="Email" id="email-link">
							<input
								id="email-link"
								type="email"
								bind:value={email}
								required
								autocomplete="email"
								class="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm"
							/>
						</Field>
					{/if}
					<Button type="submit" variant="secondary" loading={submitting} class="w-full">
						Email me a sign-in link
					</Button>
				</form>
			{/if}

			{#if methods.google}
				<div class="mt-4">
					<Button
						type="button"
						variant="secondary"
						loading={submitting}
						class="w-full"
						onclick={signInGoogle}
					>
						Continue with Google
					</Button>
				</div>
			{/if}

			{#if info}
				<p class="mt-4 text-sm text-green-700" role="status">{info}</p>
			{/if}
			{#if error}
				<p class="mt-4 text-sm text-red-600" role="alert">{error}</p>
			{/if}

			<p class="mt-4 text-center text-sm text-gray-500">
				New to Tallyo? <a href="/signup" class="font-medium text-brand-700 hover:text-brand-800"
					>Create an account</a
				>
			</p>
		{/if}
	</Card>
</div>
