import {
	EmailAuthProvider,
	GoogleAuthProvider,
	fetchSignInMethodsForEmail,
	linkWithCredential,
	signInWithEmailAndPassword,
	signInWithPopup,
	type Auth,
	type AuthCredential,
	type UserCredential
} from 'firebase/auth';

/**
 * Account-linking helpers for the "one email = one uid across email/password,
 * email-link, and Google" requirement (GCIP allow_duplicate_emails=false).
 *
 * GCIP does NOT auto-merge a second method onto an existing email. The second
 * attempt throws and leaves a *pending* credential that must be linked AFTER the
 * user proves ownership by signing in with their existing method. These helpers
 * carry out that catch+reauth+link dance. Linking is never silent — the caller
 * always re-authenticates the user first.
 */

/** The Firebase error codes that signal a same-email-different-method clash. */
export const ACCOUNT_EXISTS_CODE = 'auth/account-exists-with-different-credential';
export const EMAIL_IN_USE_CODE = 'auth/email-already-in-use';

/** Read a Firebase error's `code` (best-effort, never throws). */
export function errorCode(err: unknown): string {
	if (err && typeof err === 'object' && 'code' in err) {
		return String((err as { code: unknown }).code);
	}
	return '';
}

/** True when `err` is the Google-popup-vs-existing-email clash. */
export function isAccountExistsWithDifferentCredential(err: unknown): boolean {
	return errorCode(err) === ACCOUNT_EXISTS_CODE;
}

/**
 * Describes a clash caught from a failed Google popup: the pending Google
 * credential to link once the user has re-authenticated, the email it belongs
 * to, and the sign-in methods already registered for that email.
 */
export interface PendingLink {
	email: string;
	pendingCred: AuthCredential;
	methods: string[];
}

/** Method-id constants Firebase returns from fetchSignInMethodsForEmail. */
export const PASSWORD_METHOD = EmailAuthProvider.PROVIDER_ID; // 'password'
export const EMAIL_LINK_METHOD = 'emailLink';
export const GOOGLE_METHOD = GoogleAuthProvider.PROVIDER_ID; // 'google.com'

/**
 * Build a PendingLink from a caught `account-exists-with-different-credential`
 * error. Returns null when the error has no recoverable Google credential.
 */
export async function pendingLinkFromError(
	auth: Auth,
	err: unknown
): Promise<PendingLink | null> {
	const cred = GoogleAuthProvider.credentialFromError(
		err as Parameters<typeof GoogleAuthProvider.credentialFromError>[0]
	);
	const email =
		err && typeof err === 'object' && 'customData' in err
			? ((err as { customData?: { email?: string } }).customData?.email ?? '')
			: '';
	if (!cred || !email) return null;
	const methods = await fetchSignInMethodsForEmail(auth, email);
	return { email, pendingCred: cred, methods };
}

/** Existing sign-in methods registered for an email (empty when none). */
export function signInMethodsForEmail(auth: Auth, email: string): Promise<string[]> {
	return fetchSignInMethodsForEmail(auth, email);
}

/**
 * Re-authenticate with the existing PASSWORD method, then link the pending
 * Google credential onto that uid. Returns the resulting UserCredential.
 */
export async function linkWithPassword(
	auth: Auth,
	pending: PendingLink,
	password: string
): Promise<UserCredential> {
	await signInWithEmailAndPassword(auth, pending.email, password);
	if (!auth.currentUser) throw new Error('sign-in did not establish a user');
	return linkWithCredential(auth.currentUser, pending.pendingCred);
}

/**
 * Re-authenticate by re-running the Google popup (used when the EXISTING method
 * is Google and the pending credential is something else). The caller passes the
 * credential to link after the popup succeeds.
 */
export async function reauthGoogleAndLink(
	auth: Auth,
	credToLink: AuthCredential
): Promise<UserCredential> {
	await signInWithPopup(auth, new GoogleAuthProvider());
	if (!auth.currentUser) throw new Error('sign-in did not establish a user');
	return linkWithCredential(auth.currentUser, credToLink);
}

/** Build an email/password credential for linking after a reauth. */
export function emailPasswordCredential(email: string, password: string): AuthCredential {
	return EmailAuthProvider.credential(email, password);
}
