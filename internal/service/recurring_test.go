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

func newRecurringSvc(t *testing.T) (*RecurringService, *realtime.Hub, *repository.ClientsRepo) {
	t.Helper()
	conn, err := appdb.Open(filepath.Join(t.TempDir(), "recurring.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	if err := appdb.Migrate(conn); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	hub := realtime.NewHub()
	return NewRecurringService(conn, hub), hub, repository.NewClients(conn)
}

// seedRecurringInput builds a valid monthly template input for the given client,
// due in the past so GenerateDue/GenerateOne will produce an invoice.
func seedRecurringInput(clientID int64) repository.RecurringInput {
	cid := clientID
	return repository.RecurringInput{
		ClientID:  &cid,
		Name:      "Monthly",
		Frequency: "monthly",
		NextDue:   "2026-01-01",
		LineItems: []repository.RecurringLine{
			{Description: "A", Quantity: 2, Rate: 10, SortOrder: 0},
			{Description: "B", Quantity: 1, Rate: 5, SortOrder: 1},
		},
		TaxRate:  10,
		IsActive: true,
	}
}

func TestRecurringCreateBroadcasts(t *testing.T) {
	svc, hub, clients := newRecurringSvc(t)
	clientID := seedClient(t, clients)
	ch, unsub := hub.Subscribe()
	defer unsub()
	ctx := context.Background()

	tpl, err := svc.Create(ctx, seedRecurringInput(clientID))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if tpl == nil {
		t.Fatal("Create returned nil template")
	}
	select {
	case e := <-ch:
		if e.Entity != "recurring_template" || e.ID != tpl.ID || e.Action != "create" {
			t.Fatalf("event=%+v want recurring_template/%d/create", e, tpl.ID)
		}
	case <-time.After(time.Second):
		t.Fatal("no broadcast after Create")
	}
}

func TestRecurringGenerateOneBroadcasts(t *testing.T) {
	svc, hub, clients := newRecurringSvc(t)
	clientID := seedClient(t, clients)
	ctx := context.Background()

	tpl, err := svc.Create(ctx, seedRecurringInput(clientID))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	ch, unsub := hub.Subscribe()
	defer unsub()

	inv, err := svc.GenerateOne(ctx, tpl.ID)
	if err != nil {
		t.Fatalf("GenerateOne: %v", err)
	}
	if inv == nil {
		t.Fatal("GenerateOne returned nil invoice")
	}

	// Expect a recurring_template/generate followed by an invoice/create.
	gotGenerate, gotCreate := false, false
	for i := 0; i < 2; i++ { // bounded: exactly two events expected
		select {
		case e := <-ch:
			switch {
			case e.Entity == "recurring_template" && e.ID == tpl.ID && e.Action == "generate":
				gotGenerate = true
			case e.Entity == "invoice" && e.ID == inv.ID && e.Action == "create":
				gotCreate = true
			default:
				t.Fatalf("unexpected event=%+v", e)
			}
		case <-time.After(time.Second):
			t.Fatalf("missing broadcast (generate=%v create=%v)", gotGenerate, gotCreate)
		}
	}
	if !gotGenerate || !gotCreate {
		t.Fatalf("want both generate and create events, got generate=%v create=%v", gotGenerate, gotCreate)
	}
}

func TestRecurringGenerateOneMissingTemplate(t *testing.T) {
	svc, _, _ := newRecurringSvc(t)
	inv, err := svc.GenerateOne(context.Background(), 999)
	if err != nil {
		t.Fatalf("GenerateOne missing: %v", err)
	}
	if inv != nil {
		t.Fatalf("want nil invoice for missing template, got %+v", inv)
	}
}
