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
	if Round2(1.005) != 1.01 {
		t.Fatalf("Round2(1.005) = %v, want 1.01", Round2(1.005))
	}
}
