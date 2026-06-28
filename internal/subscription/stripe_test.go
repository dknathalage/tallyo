package subscription

import "testing"

// priceFor is the money path for the annual/monthly checkout split. Unknown or
// empty plans must fall back to monthly; annual falls back to monthly when no
// annual price is configured.
func TestPriceFor(t *testing.T) {
	c := &Client{priceID: "price_monthly", priceIDAnnual: "price_annual"}
	cases := map[string]string{
		"annual":  "price_annual",
		"monthly": "price_monthly",
		"":        "price_monthly",
		"bogus":   "price_monthly",
	}
	for plan, want := range cases {
		if got := c.priceFor(plan); got != want {
			t.Errorf("priceFor(%q) = %q, want %q", plan, got, want)
		}
	}

	// No annual price configured → annual falls back to monthly.
	noAnnual := &Client{priceID: "price_monthly"}
	if got := noAnnual.priceFor("annual"); got != "price_monthly" {
		t.Errorf("priceFor(annual) with no annual price = %q, want price_monthly", got)
	}
}
