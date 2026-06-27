package session

import (
	"testing"

	"github.com/dknathalage/tallyo/internal/invoice"
	"github.com/dknathalage/tallyo/internal/reqctx"
)

func newSessionSvc(t *testing.T) (*Service, string, string) {
	t.Helper()
	conn := newTestDB(t)
	tenantID := seedTenant(t, conn, "Acme")
	clientID := seedClient(t, conn, tenantID, "Jane Client")
	return NewService(conn, invoice.NewInvoices(conn)), tenantID, clientID
}

func sessionInput(pid string, date string) SessionInput {
	return SessionInput{
		ClientID: pid, ServiceDate: date,
		Note: "supported community access",
		Tags: []string{"t1"},
	}
}

func TestSessionCreate(t *testing.T) {
	svc, tenantID, clientID := newSessionSvc(t)
	ctx := tctx(tenantID)

	sh, err := svc.Create(ctx, sessionInput(clientID, "2026-01-15"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if sh == nil {
		t.Fatal("Create returned nil session")
	}
}

func TestSessionCreateAttributesAuthor(t *testing.T) {
	conn := newTestDB(t)
	tenantID := seedTenant(t, conn, "Acme")
	clientID := seedClient(t, conn, tenantID, "Jane Client")
	uid := seedUser(t, conn, tenantID)
	svc := NewService(conn, invoice.NewInvoices(conn))
	ctx := reqctx.WithUser(tctx(tenantID), uid)

	sh, err := svc.Create(ctx, sessionInput(clientID, "2026-01-15"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if sh.AuthorUserID == nil || *sh.AuthorUserID != uid {
		t.Fatalf("author not attributed: %+v want %s", sh.AuthorUserID, uid)
	}
}

func TestSessionListClientRangeSvc(t *testing.T) {
	svc, tenantID, clientID := newSessionSvc(t)
	ctx := tctx(tenantID)

	for _, d := range []string{"2026-01-10", "2026-01-15", "2026-01-20"} {
		if _, err := svc.Create(ctx, sessionInput(clientID, d)); err != nil {
			t.Fatalf("Create %s: %v", d, err)
		}
	}
	rng, err := svc.ListClient(ctx, clientID, "2026-01-15", "2026-01-20")
	if err != nil {
		t.Fatalf("ListClient range: %v", err)
	}
	if len(rng) != 2 {
		t.Fatalf("range [15,20] = %d, want 2", len(rng))
	}
}

func TestSessionUpdateStatusSvc(t *testing.T) {
	svc, tenantID, clientID := newSessionSvc(t)
	ctx := tctx(tenantID)

	in := sessionInput(clientID, "2026-01-15")
	in.Status = "scheduled"
	sh, err := svc.Create(ctx, in)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := svc.UpdateStatus(ctx, sh.ID, "recorded"); err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}
	got, _ := svc.Get(ctx, sh.ID)
	if got == nil || got.Status != "recorded" {
		t.Fatalf("status after UpdateStatus = %+v, want recorded", got)
	}
}

func TestSessionDeleteSvc(t *testing.T) {
	svc, tenantID, clientID := newSessionSvc(t)
	ctx := tctx(tenantID)

	sh, err := svc.Create(ctx, sessionInput(clientID, "2026-01-15"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := svc.Delete(ctx, sh.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if got, _ := svc.Get(ctx, sh.ID); got != nil {
		t.Fatalf("session present after delete: %+v", got)
	}
}

func TestSessionToRecord(t *testing.T) {
	svc, tenantID, clientID := newSessionSvc(t)
	ctx := tctx(tenantID)

	sched := sessionInput(clientID, "2026-01-15")
	sched.Status = "scheduled"
	if _, err := svc.Create(ctx, sched); err != nil {
		t.Fatalf("Create scheduled: %v", err)
	}
	if _, err := svc.Create(ctx, sessionInput(clientID, "2026-01-16")); err != nil {
		t.Fatalf("Create recorded: %v", err)
	}

	toRec, err := svc.ToRecord(ctx)
	if err != nil {
		t.Fatalf("ToRecord: %v", err)
	}
	if len(toRec) != 1 || toRec[0].Status != "scheduled" {
		t.Fatalf("ToRecord = %+v, want 1 scheduled", toRec)
	}
}

func TestSessionSuggestions(t *testing.T) {
	conn := newTestDB(t)
	tenantID := seedTenant(t, conn, "Acme")
	p1 := seedClient(t, conn, tenantID, "Jane")
	p2 := seedClient(t, conn, tenantID, "Bob")
	svc := NewService(conn, invoice.NewInvoices(conn))
	ctx := tctx(tenantID)

	var p1ids []string
	for _, d := range []string{"2026-01-10", "2026-01-20"} {
		sh, err := svc.Create(ctx, sessionInput(p1, d))
		if err != nil {
			t.Fatalf("Create p1: %v", err)
		}
		p1ids = append(p1ids, sh.ID)
	}
	if _, err := svc.Create(ctx, sessionInput(p2, "2026-02-01")); err != nil {
		t.Fatalf("Create p2: %v", err)
	}

	sugs, err := svc.Suggestions(ctx)
	if err != nil {
		t.Fatalf("Suggestions: %v", err)
	}
	if len(sugs) != 2 {
		t.Fatalf("suggestions = %d, want 2: %+v", len(sugs), sugs)
	}
	byPID := map[string]Suggestion{}
	for _, s := range sugs {
		byPID[s.ClientID] = s
	}
	s1 := byPID[p1]
	if s1.Count != 2 || s1.From != "2026-01-10" || s1.To != "2026-01-20" || len(s1.IDs) != 2 {
		t.Fatalf("p1 suggestion = %+v", s1)
	}
	if !containsID(s1.IDs, p1ids[0]) || !containsID(s1.IDs, p1ids[1]) {
		t.Fatalf("p1 ids = %+v, want %+v", s1.IDs, p1ids)
	}
}

func TestSessionMarkDrafted(t *testing.T) {
	conn := newTestDB(t)
	tenantID := seedTenant(t, conn, "Acme")
	clientID := seedClient(t, conn, tenantID, "Jane Client")
	invID := seedDraftInvoice(t, conn, tenantID, clientID)
	svc := NewService(conn, invoice.NewInvoices(conn))
	ctx := tctx(tenantID)

	sh, err := svc.Create(ctx, sessionInput(clientID, "2026-01-15"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := svc.MarkDrafted(ctx, invID, []string{sh.ID}); err != nil {
		t.Fatalf("MarkDrafted: %v", err)
	}
	got, _ := svc.Get(ctx, sh.ID)
	if got == nil || got.Status != "drafted" || got.InvoiceID == nil || *got.InvoiceID != invID {
		t.Fatalf("after MarkDrafted = %+v, want drafted+invoice %s", got, invID)
	}
}

func TestSessionMarkDraftedRejectsCrossTenantInvoice(t *testing.T) {
	conn := newTestDB(t)

	tenantA := seedTenant(t, conn, "Acme")
	clientA := seedClient(t, conn, tenantA, "Jane")
	invA := seedDraftInvoice(t, conn, tenantA, clientA)

	tenantB := seedTenant(t, conn, "Beta")
	clientB := seedClient(t, conn, tenantB, "Bob")
	svc := NewService(conn, invoice.NewInvoices(conn))
	ctxB := tctx(tenantB)

	sh, err := svc.Create(ctxB, sessionInput(clientB, "2026-01-15"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := svc.MarkDrafted(ctxB, invA, []string{sh.ID}); err == nil {
		t.Fatal("MarkDrafted cross-tenant invoice: want error, got nil")
	}
	got, _ := svc.Get(ctxB, sh.ID)
	if got == nil || got.InvoiceID != nil || got.Status != "recorded" {
		t.Fatalf("session must remain unbilled: %+v", got)
	}
}

func TestSessionMarkDraftedEmpty(t *testing.T) {
	svc, tenantID, _ := newSessionSvc(t)

	if err := svc.MarkDrafted(tctx(tenantID), "some-invoice-uuid", nil); err != nil {
		t.Fatalf("MarkDrafted empty: %v", err)
	}
}
