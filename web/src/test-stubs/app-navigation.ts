// Stub for $app/navigation used in tests (vitest environment without SvelteKit)
export function goto(_url: string | URL, _opts?: unknown): Promise<void> {
	return Promise.resolve();
}
