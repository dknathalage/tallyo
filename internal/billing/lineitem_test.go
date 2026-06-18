package billing

import "testing"

func TestLineItemTypes(t *testing.T) {
	var in LineItemInput
	if in.Quantity != 0 || in.GstFree {
		t.Fatalf("unexpected zero value: %+v", in)
	}
	li := LineItem{Code: "01_011", Quantity: 2, UnitPrice: 10}
	if li.Code != "01_011" {
		t.Fatalf("LineItem field mismatch")
	}
}
