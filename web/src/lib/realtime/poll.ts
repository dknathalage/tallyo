/**
 * Polling replacement for the removed SSE subscription. Calls refetch once
 * immediately, then on a fixed interval and on visibility/focus regain.
 * Returns a cleanup that stops everything. SSR-safe (no-op without window).
 * ponytail: fixed 30s interval + focus refetch; tune only if stale or chatty.
 */
const POLL_INTERVAL_MS = 30_000;

export function startPolling(refetch: () => void): () => void {
	if (typeof refetch !== 'function') throw new Error('startPolling: refetch must be a function');
	if (typeof window === 'undefined' || typeof document === 'undefined') return () => {};

	refetch();
	const interval = setInterval(refetch, POLL_INTERVAL_MS);
	const onVisible = (): void => {
		if (document.visibilityState === 'visible') refetch();
	};
	document.addEventListener('visibilitychange', onVisible);
	window.addEventListener('focus', refetch);

	return () => {
		clearInterval(interval);
		document.removeEventListener('visibilitychange', onVisible);
		window.removeEventListener('focus', refetch);
	};
}
