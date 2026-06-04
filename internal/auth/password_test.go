package auth

import "testing"

func TestHashAndVerify(t *testing.T) {
	h, err := HashPassword("s3cret-pw")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if h == "s3cret-pw" || h == "" {
		t.Fatalf("hash not produced: %q", h)
	}
	if !VerifyPassword(h, "s3cret-pw") {
		t.Fatal("verify should succeed")
	}
	if VerifyPassword(h, "wrong") {
		t.Fatal("verify should fail for wrong pw")
	}
}

func TestHashRejectsEmpty(t *testing.T) {
	if _, err := HashPassword(""); err == nil {
		t.Fatal("empty password must error")
	}
}
