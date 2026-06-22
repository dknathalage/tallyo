package recurring

import (
	"testing"
	"time"
)

func TestRecurringCreateBroadcasts(t *testing.T) {
	svc, hub, tenantID, participantUUID := newRecurringSvc(t)
	ch, unsub := hub.Subscribe(tenantID)
	defer unsub()
	ctx := tctx(tenantID)

	tpl, err := svc.Create(ctx, seedRecurringInput(participantUUID))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if tpl == nil {
		t.Fatal("Create returned nil template")
	}
	select {
	case e := <-ch:
		if e.Entity != "recurring_template" || e.UUID != tpl.UUID || e.Action != "create" {
			t.Fatalf("event=%+v want recurring_template/%d/create", e, tpl.ID)
		}
	case <-time.After(time.Second):
		t.Fatal("no broadcast after Create")
	}
}

func TestRecurringGenerateOneBroadcasts(t *testing.T) {
	svc, hub, tenantID, participantUUID := newRecurringSvc(t)
	ctx := tctx(tenantID)

	tpl, err := svc.Create(ctx, seedRecurringInput(participantUUID))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	ch, unsub := hub.Subscribe(tenantID)
	defer unsub()

	inv, err := svc.GenerateOne(ctx, tpl.UUID)
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
			case e.Entity == "recurring_template" && e.UUID == tpl.UUID && e.Action == "generate":
				gotGenerate = true
			case e.Entity == "invoice" && e.UUID == inv.UUID && e.Action == "create":
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
	inv, err := svc.GenerateOne(tctx(tenantID), "3f1b8e2a-6c4d-4f7a-9b0c-1d2e3f4a5b6c")
	if err != nil {
		t.Fatalf("GenerateOne missing: %v", err)
	}
	if inv != nil {
		t.Fatalf("want nil invoice for missing template, got %+v", inv)
	}
}
