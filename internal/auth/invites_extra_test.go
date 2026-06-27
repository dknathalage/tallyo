package auth

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestMarkAcceptedUnknownTokenFails(t *testing.T) {
	conn, _, _ := mustInviteDB(t)
	defer conn.Close()
	err := NewInvites(conn).MarkAccepted(context.Background(), "no-such-token")
	if !errors.Is(err, ErrInviteInvalid) {
		t.Fatalf("want ErrInviteInvalid, got %v", err)
	}
}

func TestInviteCreateRejectsZeroTenant(t *testing.T) {
	conn, _, owner := mustInviteDB(t)
	defer conn.Close()
	if _, err := NewInvites(conn).Create(context.Background(), "", "a@x.com", "member", owner, time.Hour); err == nil {
		t.Fatal("zero tenant id must error")
	}
}

func TestAcceptRejectsEmptyTokenOrUID(t *testing.T) {
	conn, _, _ := mustInviteDB(t)
	defer conn.Close()
	repo := NewInvites(conn)
	ctx := context.Background()
	if _, err := repo.Accept(ctx, "", "Name", "uid"); err == nil {
		t.Fatal("empty token must error")
	}
	if _, err := repo.Accept(ctx, "tok", "Name", ""); err == nil {
		t.Fatal("empty firebase uid must error")
	}
}

func TestAcceptUnknownTokenFails(t *testing.T) {
	conn, _, _ := mustInviteDB(t)
	defer conn.Close()
	_, err := NewInvites(conn).Accept(context.Background(), "no-such-token", "Name", "uid")
	if !errors.Is(err, ErrInviteInvalid) {
		t.Fatalf("want ErrInviteInvalid, got %v", err)
	}
}

func TestAcceptExpiredInviteFails(t *testing.T) {
	conn, tid, owner := mustInviteDB(t)
	defer conn.Close()
	repo := NewInvites(conn)
	ctx := context.Background()

	inv, err := repo.Create(ctx, tid, "late@x.com", "member", owner, -1*time.Minute)
	if err != nil {
		t.Fatalf("Create expired: %v", err)
	}
	if _, err := repo.Accept(ctx, inv.Token, "Late", "uid-late"); !errors.Is(err, ErrInviteInvalid) {
		t.Fatalf("want ErrInviteInvalid for expired accept, got %v", err)
	}
	// the user must NOT have been created (rollback / pre-check).
	got, err := NewUsers(conn).GetByEmail(ctx, tid, "late@x.com")
	if err != nil {
		t.Fatalf("GetByEmail: %v", err)
	}
	if got != nil {
		t.Fatalf("expired accept created a user: %+v", got)
	}
}

func TestNewInvitesNilDBPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("NewInvites(nil) must panic")
		}
	}()
	NewInvites(nil)
}
