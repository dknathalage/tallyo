import { initializeApp, type FirebaseApp } from 'firebase/app';
import { getAuth, connectAuthEmulator, type Auth } from 'firebase/auth';

/**
 * The auth methods the backend has enabled, mirrored from GET /api/auth/config.
 * The login page renders only the enabled methods.
 */
export interface AuthMethods {
	emailPassword: boolean;
	google: boolean;
	emailLink: boolean;
}

/** Response shape of the public GET /api/auth/config endpoint (see contract). */
interface AuthConfig {
	firebase: { apiKey: string; authDomain: string; projectId: string };
	methods: AuthMethods;
	/** Optional flag: backend may signal it runs against the emulator. */
	emulator?: boolean;
	emulatorHost?: string;
}

/**
 * Lazy, idempotent Firebase bootstrap. The Firebase config (browser API key,
 * auth domain, project id) is NOT baked into the build — it is fetched at runtime
 * from GET /api/auth/config so the same SPA bundle works across environments.
 *
 * initFirebase() is safe to call repeatedly: the first call performs the fetch +
 * initializeApp + getAuth (+ emulator wiring) and caches the promise; subsequent
 * calls await the same promise. Callers that need the auth instance should
 * `await getFirebaseAuth()`.
 */
interface Initialized {
	app: FirebaseApp;
	auth: Auth;
	methods: AuthMethods;
}

let initPromise: Promise<Initialized> | null = null;

// In dev (vite proxy → :8080) we honour the emulator if the backend tells us to.
function shouldUseEmulator(cfg: AuthConfig): boolean {
	return cfg.emulator === true || typeof cfg.emulatorHost === 'string';
}

function emulatorUrl(cfg: AuthConfig): string {
	// Default to the standard auth-emulator port if a bare flag was sent.
	const host = cfg.emulatorHost && cfg.emulatorHost.length > 0 ? cfg.emulatorHost : 'localhost:9099';
	return host.startsWith('http') ? host : `http://${host}`;
}

async function doInit(): Promise<Initialized> {
	const res = await fetch('/api/auth/config');
	if (!res.ok) {
		throw new Error(`auth config fetch failed (${res.status})`);
	}
	const cfg = (await res.json()) as AuthConfig;

	const app = initializeApp({
		apiKey: cfg.firebase.apiKey,
		authDomain: cfg.firebase.authDomain,
		projectId: cfg.firebase.projectId
	});
	const auth = getAuth(app);

	if (shouldUseEmulator(cfg)) {
		// disableWarnings keeps the dev console clean; harmless in prod (never hit).
		connectAuthEmulator(auth, emulatorUrl(cfg), { disableWarnings: true });
	}

	return { app, auth, methods: cfg.methods };
}

/** Run (or reuse) the one-time Firebase bootstrap. */
export function initFirebase(): Promise<Initialized> {
	if (initPromise === null) {
		initPromise = doInit();
	}
	return initPromise;
}

/** Await the initialised Firebase Auth instance. */
export async function getFirebaseAuth(): Promise<Auth> {
	const { auth } = await initFirebase();
	return auth;
}

/** Await the enabled auth-method flags from the backend config. */
export async function getAuthMethods(): Promise<AuthMethods> {
	const { methods } = await initFirebase();
	return methods;
}
