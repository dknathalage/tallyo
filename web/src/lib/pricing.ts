// Display-only marketing pricing. The backend has exactly ONE Stripe price;
// these tiers are copy. Every tier CTA goes to /signup (no plan param). The
// monthly/annual toggle changes shown numbers only — it does not affect checkout.

export type Tier = 'starter' | 'professional' | 'business';

export type TierPrices = Record<Tier, string>;

// Monthly prices (shown when the toggle is on "Monthly").
export const monthlyPrices: TierPrices = {
	starter: '$0',
	professional: '$29',
	business: '$79'
};

// Annual prices (shown when the toggle is on "Annual" — approx 2 months free,
// displayed per month).
export const annualPrices: TierPrices = {
	starter: '$0',
	professional: '$24',
	business: '$66'
};

/** Pure selection: the price set to display for the chosen billing cadence. */
export function pricesFor(annual: boolean): TierPrices {
	return annual ? annualPrices : monthlyPrices;
}
