import { apiGet, apiPost, tenantPath } from './client';

/** Subscription status for the current tenant (mirrors GET /api/t/{id}/billing). */
export interface BillingStatus {
	status: string; // none | trialing | active | past_due | canceled
	trialEnd: string; // RFC3339, "" when unknown
	currentPeriodEnd: string;
	entitled: boolean;
}

/** Fetch the current tenant's subscription status. */
export async function getBillingStatus(): Promise<BillingStatus | null> {
	return apiGet<BillingStatus>(tenantPath('billing'));
}

/**
 * Start a Stripe Checkout session and return its redirect URL (owner-only).
 * `plan` selects the cadence; "annual" uses the annual price, anything else
 * (default) uses monthly. Passed as a query param on the POST.
 */
export async function startCheckout(plan: 'monthly' | 'annual' = 'monthly'): Promise<string | null> {
	const res = await apiPost<{ url: string }>(tenantPath(`billing/checkout?plan=${plan}`));
	return res?.url ?? null;
}

/** Open the Stripe Customer Portal and return its redirect URL (owner-only). */
export async function openPortal(): Promise<string | null> {
	const res = await apiGet<{ url: string }>(tenantPath('billing/portal'));
	return res?.url ?? null;
}

/** Days remaining until trialEnd (0 when unknown/past). */
export function trialDaysLeft(trialEnd: string): number {
	if (!trialEnd) return 0;
	const end = new Date(trialEnd).getTime();
	if (Number.isNaN(end)) return 0;
	const ms = end - Date.now();
	return ms <= 0 ? 0 : Math.ceil(ms / 86_400_000);
}
