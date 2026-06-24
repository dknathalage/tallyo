package ids

import "testing"

func TestNewIsUUIDv7AndOrdered(t *testing.T) {
	a := New()
	b := New()
	if len(a) != 36 {
		t.Fatalf("want 36-char uuid, got %q", a)
	}
	if a[14] != '7' { // version nibble
		t.Fatalf("want version 7, got %q in %q", a[14], a)
	}
	if !(a < b) { // v7 is time-ordered
		t.Fatalf("v7 ids must be lexically time-ordered: %q !< %q", a, b)
	}
}
