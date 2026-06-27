import { getBillingStatus, type BillingStatus } from '$lib/api/billing';

/**
 * Singleton billing store. Holds the current tenant's subscription status for the
 * layout banner and the billing settings page. `blocked` is flipped on by the API
 * client when a write returns 402 (subscription required), so the UI can prompt to
 * subscribe even before the next status refresh.
 */
function createBillingStore() {
	let status = $state<BillingStatus | null>(null);
	let blocked = $state(false);

	async function load(): Promise<void> {
		const s = await getBillingStatus();
		if (s !== null) {
			status = s;
			blocked = !s.entitled;
		}
	}

	return {
		get status(): BillingStatus | null {
			return status;
		},
		get entitled(): boolean {
			// Unknown status (not yet loaded) is treated as entitled so the UI doesn't
			// flash a paywall before the first load; the server is the real gate.
			return status === null ? true : status.entitled;
		},
		get blocked(): boolean {
			return blocked;
		},
		markBlocked(): void {
			blocked = true;
		},
		load
	};
}

export const billing = createBillingStore();
