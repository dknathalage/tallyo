// Display-only marketing pricing. The backend has exactly ONE Stripe price
// (monthly, AUD). The annual figure is the per-month equivalent of the $190/yr
// plan. The monthly/annual toggle changes shown numbers only — it does not
// affect checkout. The CTA goes to /signup (no plan param).

// AUD. Monthly is the real charge; annual shown as the per-month equivalent.
export const monthlyPrice = '$19';
export const annualPerMonth = '$15.83';
export const annualTotal = '$190';

/** Pure selection: price + period string for the chosen billing cadence. */
export function planFor(annual: boolean): { price: string; period: string } {
	return annual
		? { price: annualPerMonth, period: '/mo, billed annually' }
		: { price: monthlyPrice, period: '/month' };
}
