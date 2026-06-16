package service

import (
	"testing"
	"time"

	"github.com/dknathalage/tallyo/internal/realtime"
	"github.com/dknathalage/tallyo/internal/repository"
)

func newRecurringSvc(t *testing.T) (*RecurringService, *realtime.Hub, int64, int64) {
	t.Helper()
	conn := newTestDB(t)
	tenantID := seedTenant(t, conn)
	participantID := seedParticipant(t, conn, tenantID)
	hub := realtime.NewHub()
	return NewRecurringService(conn, hub), hub, tenantID, participantID
}

// seedRecurringInput builds a valid monthly template input for the given
// participant, due in the past so GenerateOne will produce an invoice.
func seedRecurringInput(participantID int64) repository.RecurringInput {
	pid := participantID
	return repository.RecurringInput{
		ParticipantID: &pid,
		Name:          "Monthly",
		Frequency:     "monthly",
		NextDue:       "2026-01-01",
		LineItems: []repository.RecurringLine{
			{Description: "A", Quantity: 2, UnitPrice: 10, SortOrder: 0},
			{Description: "B", Quantity: 1, UnitPrice: 5, SortOrder: 1},
		},
		TaxRate:  10,
		IsActive: true,
	}
}

func TestRecurringCreateBroadcasts(t *testing.T) {
	svc, hub, tenantID, participantID := newRecurringSvc(t)
	ch, unsub := hub.Subscribe(tenantID)
	defer unsub()
	ctx := tctx(tenantID)

	tpl, err := svc.Create(ctx, seedRecurringInput(participantID))
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
	svc, hub, tenantID, participantID := newRecurringSvc(t)
	ctx := tctx(tenantID)

	tpl, err := svc.Create(ctx, seedRecurringInput(participantID))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	ch, unsub := hub.Subscribe(tenantID)
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
	svc, _, tenantID, _ := newRecurringSvc(t)
	inv, err := svc.GenerateOne(tctx(tenantID), 999)
	if err != nil {
		t.Fatalf("GenerateOne missing: %v", err)
	}
	if inv != nil {
		t.Fatalf("want nil invoice for missing template, got %+v", inv)
	}
}
