package subscription

import "testing"

func TestPriceFor(t *testing.T) {
	c := &Client{priceID: "price_monthly", priceIDAnnual: "price_annual"}
	cases := map[string]string{
		"annual":  "price_annual",
		"monthly": "price_monthly",
		"":        "price_monthly",
		"weekly":  "price_monthly", // unknown → monthly
	}
	for plan, want := range cases {
		if got := c.priceFor(plan); got != want {
			t.Errorf("priceFor(%q) = %q, want %q", plan, got, want)
		}
	}
}

func TestNewClientAnnualFallsBackToMonthly(t *testing.T) {
	c, err := NewClient(Config{SecretKey: "sk_test", PriceID: "price_monthly"})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if got := c.priceFor("annual"); got != "price_monthly" {
		t.Errorf("annual price with no STRIPE_PRICE_ID_ANNUAL = %q, want fallback price_monthly", got)
	}
}
