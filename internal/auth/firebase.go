package auth

import (
	"context"
	"errors"
	"fmt"
	"os"

	firebase "firebase.google.com/go/v4"
	firebaseauth "firebase.google.com/go/v4/auth"
	"google.golang.org/api/option"
)

// Token carries the verified identity claims extracted from a Firebase ID
// token. It is the stateless replacement for the old scs session: every /api
// request presents a bearer token, RequireAuth verifies it, and the resulting
// Token (uid + email + name) is attached to the request context.
type Token struct {
	UID   string
	Email string
	Name  string
}

// TokenVerifier verifies a raw Firebase ID token (the bearer credential) and
// returns its identity claims. It is an interface so handlers and middleware can
// be wired with a real Firebase verifier in production and a stub in tests (the
// emulator is honoured by the real verifier when FIREBASE_AUTH_EMULATOR_HOST is
// set, but the interface keeps unit tests from needing any GCP at all).
type TokenVerifier interface {
	VerifyIDToken(ctx context.Context, idToken string) (Token, error)
}

// ErrInvalidToken is returned by a verifier when the presented bearer token is
// missing, malformed, expired, or otherwise not a valid Firebase ID token.
var ErrInvalidToken = errors.New("invalid id token")

// FirebaseVerifier is the production TokenVerifier backed by the Firebase Admin
// SDK. It verifies tokens against the project's keys (or the local emulator when
// FIREBASE_AUTH_EMULATOR_HOST is set).
type FirebaseVerifier struct {
	client *firebaseauth.Client
}

// NewFirebaseVerifier builds the Admin SDK auth client. The project id is taken
// from FIREBASE_PROJECT_ID (required); credentials come from ADC on Cloud Run.
// When FIREBASE_AUTH_EMULATOR_HOST is set (local/dev/tests) the SDK talks to the
// emulator and does not require real ADC — we pass an empty credentials option
// so initialization does not fail looking for a service account.
func NewFirebaseVerifier(ctx context.Context) (*FirebaseVerifier, error) {
	projectID := os.Getenv("FIREBASE_PROJECT_ID")
	if projectID == "" {
		return nil, fmt.Errorf("firebase: FIREBASE_PROJECT_ID is required")
	}
	cfg := &firebase.Config{ProjectID: projectID}

	var opts []option.ClientOption
	if os.Getenv("FIREBASE_AUTH_EMULATOR_HOST") != "" {
		// The emulator needs no real credentials; without this the SDK searches
		// for ADC and fails in a bare local/test environment.
		opts = append(opts, option.WithoutAuthentication())
	}

	appFB, err := firebase.NewApp(ctx, cfg, opts...)
	if err != nil {
		return nil, fmt.Errorf("firebase: new app: %w", err)
	}
	client, err := appFB.Auth(ctx)
	if err != nil {
		return nil, fmt.Errorf("firebase: auth client: %w", err)
	}
	return &FirebaseVerifier{client: client}, nil
}

// VerifyIDToken verifies the bearer token and extracts uid, email and name. A
// verification failure is mapped to ErrInvalidToken so the HTTP layer can answer
// 401 without leaking the underlying reason.
func (v *FirebaseVerifier) VerifyIDToken(ctx context.Context, idToken string) (Token, error) {
	if idToken == "" {
		return Token{}, ErrInvalidToken
	}
	tok, err := v.client.VerifyIDToken(ctx, idToken)
	if err != nil {
		return Token{}, fmt.Errorf("%w: %v", ErrInvalidToken, err)
	}
	out := Token{UID: tok.UID}
	if v, ok := tok.Claims["email"].(string); ok {
		out.Email = v
	}
	if v, ok := tok.Claims["name"].(string); ok {
		out.Name = v
	}
	return out, nil
}
