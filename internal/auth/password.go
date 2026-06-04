package auth

import (
	"fmt"

	"golang.org/x/crypto/bcrypt"
)

// HashPassword returns a bcrypt hash. Rejects empty input.
func HashPassword(pw string) (string, error) {
	if pw == "" {
		return "", fmt.Errorf("password: empty")
	}
	b, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("hash password: %w", err)
	}
	return string(b), nil
}

// VerifyPassword reports whether pw matches the stored bcrypt hash.
func VerifyPassword(hash, pw string) bool {
	if hash == "" || pw == "" {
		return false
	}
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(pw)) == nil
}
