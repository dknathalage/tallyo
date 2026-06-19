package billing

import "testing"

func TestClassify(t *testing.T) {
	cases := map[string]UnitClass{
		"H":    UnitTime,
		"hour": UnitTime,
		"hr":   UnitTime,
		"KM":   UnitDistance,
		"km":   UnitDistance,
		"EA":   UnitCount,
		"D":    UnitCount,
		"WK":   UnitCount,
		"":     UnitCount,
		" H ":  UnitTime, // trims + upper-cases
	}
	for in, want := range cases {
		if got := Classify(in); got != want {
			t.Errorf("Classify(%q) = %v, want %v", in, got, want)
		}
	}
}
