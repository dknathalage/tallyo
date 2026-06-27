<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { apiPost } from '$lib/api/client';
	import { session } from '$lib/stores/session.svelte';
	import { getFirebaseAuth, getAuthMethods, type AuthMethods } from '$lib/firebase';
	import {
		createUserWithEmailAndPassword,
		updateProfile,
		signInWithPopup,
		signInWithEmailAndPassword,
		GoogleAuthProvider,
		linkWithCredential,
		signOut
	} from 'firebase/auth';
	import {
		errorCode,
		signInMethodsForEmail,
		emailPasswordCredential,
		PASSWORD_METHOD,
		GOOGLE_METHOD
	} from '$lib/auth-link';
	import type { User } from '$lib/api/types';
	import Button from '$lib/components/Button.svelte';
	import Card from '$lib/components/Card.svelte';
	import Field from '$lib/components/Field.svelte';
	import Receipt from '@lucide/svelte/icons/receipt';

	let methods = $state<AuthMethods | null>(null);
	let businessName = $state('');
	let name = $state('');
	let email = $state('');
	let password = $state('');
	let error = $state<string | null>(null);
	let submitting = $state(false);

	// Account-linking: the email already exists under another method. We capture
	// the existing methods and ask the user to authenticate with one, then link
	// the new credential (so one email = one uid) before provisioning the tenant.
	let existingMethods = $state<string[] | null>(null);
	const existingHasGoogle = $derived((existingMethods ?? []).includes(GOOGLE_METHOD));
	const existingHasPassword = $derived((existingMethods ?? []).includes(PASSWORD_METHOD));

	onMount(() => {
		void boot();
	});

	async function boot(): Promise<void> {
		try {
			methods = await getAuthMethods();
		} catch {
			error = 'Could not load sign-up options. Please try again.';
		}
	}

	/** Create the tenant for the freshly-signed-in Firebase user, then land in it. */
	async function provision(): Promise<void> {
		const user = await apiPost<User>('/api/signup', { businessName, name });
		session.set(user);
		const data = await session.loadSession();
		const first = data?.tenants[0];
		await goto(first ? '/' + first.id + '/' : '/');
	}

	async function submitEmail(e: SubmitEvent): Promise<void> {
		e.preventDefault();
		error = null;
		submitting = true;
		try {
			const auth = await getFirebaseAuth();
			const cred = await createUserWithEmailAndPassword(auth, email, password);
			// Carry the display name into the token's claims for the backend.
			if (name) await updateProfile(cred.user, { displayName: name });
			await provisionGuarded();
		} catch (err) {
			if (errorCode(err) === 'auth/email-already-in-use') {
				// The email already has an account (likely Google). Switch to the
				// linking flow: authenticate with the existing method, link this
				// password credential, then provision.
				const auth = await getFirebaseAuth();
				try {
					existingMethods = await signInMethodsForEmail(auth, email);
				} catch {
					existingMethods = [];
				}
				error =
					'An account with this email already exists. Sign in below to add this business to it.';
				return;
			}
			error = authError(err, 'Sign up failed.');
		} finally {
			submitting = false;
		}
	}

	/** Provision the tenant; on backend failure sign back out to avoid a half state. */
	async function provisionGuarded(): Promise<void> {
		const auth = await getFirebaseAuth();
		try {
			await provision();
		} catch (err) {
			await signOut(auth).catch(() => {});
			throw err;
		}
	}

	/** Existing account uses a password: sign in, link nothing new, provision. */
	async function linkViaPassword(e: SubmitEvent): Promise<void> {
		e.preventDefault();
		error = null;
		submitting = true;
		try {
			const auth = await getFirebaseAuth();
			await signInWithEmailAndPassword(auth, email, password);
			await provisionGuarded();
		} catch (err) {
			error = authError(err, 'Could not sign in to the existing account.');
		} finally {
			submitting = false;
		}
	}

	/** Existing account uses Google: sign in via popup, link the typed password, provision. */
	async function linkViaGoogle(): Promise<void> {
		error = null;
		submitting = true;
		try {
			const auth = await getFirebaseAuth();
			await signInWithPopup(auth, new GoogleAuthProvider());
			// If the user supplied a password, attach it as an additional sign-in
			// method on the same uid (best-effort — ignore if already linked).
			if (password && auth.currentUser) {
				try {
					await linkWithCredential(auth.currentUser, emailPasswordCredential(email, password));
				} catch {
					// Already linked or password unusable — proceed with Google auth.
				}
			}
			await provisionGuarded();
		} catch (err) {
			error = authError(err, 'Could not link your Google account.');
		} finally {
			submitting = false;
		}
	}

	function cancelLink(): void {
		existingMethods = null;
		error = null;
	}

	async function signUpGoogle(): Promise<void> {
		error = null;
		if (businessName.trim() === '') {
			error = 'Please enter a business name first.';
			return;
		}
		submitting = true;
		try {
			const auth = await getFirebaseAuth();
			await signInWithPopup(auth, new GoogleAuthProvider());
			await provisionGuarded();
		} catch (err) {
			error = authError(err, 'Google sign-up failed.');
		} finally {
			submitting = false;
		}
	}

	function authError(err: unknown, fallback: string): string {
		if (err && typeof err === 'object' && 'code' in err) {
			const code = String((err as { code: unknown }).code);
			if (code === 'auth/email-already-in-use') {
				return 'An account with this email already exists. Try signing in instead.';
			}
			if (code === 'auth/weak-password') {
				return 'Password is too weak — use at least 6 characters.';
			}
			if (code === 'auth/popup-closed-by-user' || code === 'auth/cancelled-popup-request') {
				return 'Sign-up was cancelled.';
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
		<h1 class="mb-1 text-xl font-semibold tracking-tight">Create your Tallyo account</h1>
		<p class="mb-6 text-sm text-gray-500">Set up your business in one step.</p>

		{#if methods === null}
			<p class="text-sm text-gray-500">Loading…</p>
		{:else if existingMethods !== null}
			<p class="mb-4 text-sm text-gray-600">
				<span class="font-medium">{email}</span> already has a Tallyo account. Sign in to add
				<span class="font-medium">{businessName}</span> to it.
			</p>

			{#if existingHasPassword}
				<form class="space-y-4" onsubmit={linkViaPassword}>
					<Field label="Password" id="link-password">
						<input
							id="link-password"
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
					<Button type="submit" loading={submitting} class="w-full">Sign in and continue</Button>
				</form>
			{/if}

			{#if existingHasGoogle}
				<div class={existingHasPassword ? 'mt-4' : ''}>
					<Button
						type="button"
						variant="secondary"
						loading={submitting}
						class="w-full"
						onclick={linkViaGoogle}
					>
						Continue with Google
					</Button>
				</div>
				{#if !existingHasPassword && error}
					<p class="mt-4 text-sm text-red-600" role="alert">{error}</p>
				{/if}
			{/if}

			{#if !existingHasPassword && !existingHasGoogle}
				<p class="text-sm text-red-600" role="alert">
					This email uses a sign-in method we can't link automatically. Please sign in on the
					<a href="/login" class="font-medium text-brand-700 hover:text-brand-800">sign-in page</a>.
				</p>
			{/if}

			<Button type="button" variant="secondary" class="mt-3 w-full" onclick={cancelLink}>
				Use a different email
			</Button>
		{:else}
			{#if methods.emailPassword}
				<form class="space-y-4" onsubmit={submitEmail}>
					<Field label="Business name" id="businessName">
						<input
							id="businessName"
							type="text"
							bind:value={businessName}
							required
							class="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm"
						/>
					</Field>
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
					<Field label="Password" id="password" hint="At least 6 characters.">
						<input
							id="password"
							type="password"
							bind:value={password}
							required
							minlength="6"
							autocomplete="new-password"
							class="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm"
						/>
					</Field>
					{#if error}
						<p class="text-sm text-red-600" role="alert">{error}</p>
					{/if}
					<Button type="submit" loading={submitting} class="w-full">Create account</Button>
				</form>
			{/if}

			{#if methods.google}
				{#if !methods.emailPassword}
					<Field label="Business name" id="businessName-g">
						<input
							id="businessName-g"
							type="text"
							bind:value={businessName}
							required
							class="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm"
						/>
					</Field>
				{/if}
				<div class="mt-4">
					<Button
						type="button"
						variant="secondary"
						loading={submitting}
						class="w-full"
						onclick={signUpGoogle}
					>
						Sign up with Google
					</Button>
				</div>
				{#if !methods.emailPassword && error}
					<p class="mt-4 text-sm text-red-600" role="alert">{error}</p>
				{/if}
			{/if}

			<p class="mt-4 text-center text-sm text-gray-500">
				Already have an account? <a
					href="/login"
					class="font-medium text-brand-700 hover:text-brand-800">Sign in</a
				>
			</p>
		{/if}
	</Card>
</div>
