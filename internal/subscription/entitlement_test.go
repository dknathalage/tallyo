package subscription

import "testing"

func TestEntitled(t *testing.T) {
	cases := map[string]bool{
		StatusActive:   true,
		StatusTrialing: true,
		StatusPastDue:  true,
		StatusNone:     false,
		StatusCanceled: false,
		"":             false,
		"bogus":        false,
	}
	for status, want := range cases {
		if got := Entitled(status); got != want {
			t.Errorf("Entitled(%q) = %v, want %v", status, got, want)
		}
	}
}
