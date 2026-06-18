package billing

import "math"

// Totals holds the server-computed money fields derived from line items.
type Totals struct {
	Subtotal float64
	Tax      float64
	Total    float64
}

// ComputeTotals sums line totals into the subtotal and applies the (already
// computed) tax amount. Each boundary is rounded to the cent (spec §6).
// The tax argument is an absolute amount, not a rate.
func ComputeTotals(items []LineItemInput, tax float64) Totals {
	var subtotal float64
	for i := range items { // bounded by len(items)
		subtotal += Round2(items[i].Quantity * items[i].UnitPrice)
	}
	subtotal = Round2(subtotal)
	tax = Round2(tax)
	return Totals{Subtotal: subtotal, Tax: tax, Total: Round2(subtotal + tax)}
}

// Round2 rounds to two decimal places (cents). A small epsilon is added before
// rounding to handle IEEE 754 cases such as 1.005, which cannot be represented
// exactly and would otherwise round down without it.
func Round2(x float64) float64 {
	return math.Round((x+1e-9)*100) / 100
}
