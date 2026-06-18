package billing

import "testing"

func TestComputeTotals(t *testing.T) {
	items := []LineItemInput{{Quantity: 2, UnitPrice: 10}, {Quantity: 1, UnitPrice: 5}}
	got := ComputeTotals(items, 10) // 10 = absolute tax amount, NOT a rate
	if got.Subtotal != 25 || got.Tax != 10 || got.Total != 35 {
		t.Fatalf("ComputeTotals = %+v, want {25 10 35}", got)
	}
}

func TestRound2(t *testing.T) {
	// Cases off the half-cent boundary (where float64 representation makes
	// rounding ambiguous): Round2 is math.Round(x*100)/100, the verbatim
	// original used across invoice/estimate/recurring totals.
	cases := []struct {
		in   float64
		want float64
	}{
		{1.234, 1.23},
		{1.236, 1.24},
		{2.5, 2.5},
		{0.1 + 0.2, 0.3}, // 0.30000000000000004 → 0.3
	}
	for _, c := range cases {
		if got := Round2(c.in); got != c.want {
			t.Fatalf("Round2(%v) = %v, want %v", c.in, got, c.want)
		}
	}
}
