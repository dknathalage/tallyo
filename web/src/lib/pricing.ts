// Display-only marketing pricing. Tallyo has ONE paid plan, billed monthly or
// annually (two Stripe prices). The landing monthly/annual toggle records the
// choice in sessionStorage (key PLAN_STORAGE_KEY) so the in-app billing page can
// default its cadence at first checkout — see settings/billing/+page.svelte.
// Prices are in AUD.

export type Plan = 'monthly' | 'annual';

/** sessionStorage key the landing toggle writes and the billing page reads. */
export const PLAN_STORAGE_KEY = 'tallyo_plan';

/** Monthly plan price (shown when the toggle is on "Monthly"). */
export const monthlyPrice = '$19';

/** Annual plan shown per month ($190/yr ÷ 12 ≈ $15.83). */
export const annualPerMonth = '$15.83';

/** Annual plan billed total. */
export const annualTotal = '$190';

/** Currency label shown alongside prices. */
export const currency = 'AUD';

/** Pure selection: the per-period price string to display for the cadence. */
export function priceFor(annual: boolean): string {
	return annual ? annualPerMonth : monthlyPrice;
}

/** The period suffix shown next to the price. */
export function periodFor(annual: boolean): string {
	return annual ? '/mo, billed annually' : '/month';
}
