package service

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	appdb "github.com/dknathalage/tallyo/internal/db"
	"github.com/dknathalage/tallyo/internal/realtime"
	"github.com/dknathalage/tallyo/internal/repository"
)

func newPayerSvc(t *testing.T) (*PayerService, *realtime.Hub) {
	t.Helper()
	conn, err := appdb.Open(filepath.Join(t.TempDir(), "payer.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	if err := appdb.Migrate(conn); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	hub := realtime.NewHub()
	return NewPayerService(conn, hub), hub
}

func TestPayerCreateBroadcasts(t *testing.T) {
	svc, hub := newPayerSvc(t)
	ch, unsub := hub.Subscribe()
	defer unsub()
	ctx := context.Background()

	payer, err := svc.Create(ctx, repository.PayerInput{Name: "Acme"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if payer == nil {
		t.Fatal("Create returned nil payer")
	}

	select {
	case e := <-ch:
		if e.Entity != "payer" || e.ID != payer.ID || e.Action != "create" {
			t.Fatalf("event=%+v want payer/%d/create", e, payer.ID)
		}
	case <-time.After(time.Second):
		t.Fatal("no broadcast after Create")
	}
}

func TestPayerCreateEmptyNameNoEvent(t *testing.T) {
	svc, hub := newPayerSvc(t)
	ch, unsub := hub.Subscribe()
	defer unsub()

	if _, err := svc.Create(context.Background(), repository.PayerInput{Name: ""}); err == nil {
		t.Fatal("empty name must error")
	}
	select {
	case e := <-ch:
		t.Fatalf("no event expected on failed create, got %+v", e)
	case <-time.After(100 * time.Millisecond):
		// ok
	}
}
