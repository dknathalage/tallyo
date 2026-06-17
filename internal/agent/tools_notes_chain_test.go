package agent

// End-to-end acceptance test for the notes → invoice workflow, driving the real
// tool chain (list_participant_notes → search_catalogue → create_invoice) over
// the reference nursing note exactly as the agent would, but deterministically
// (no LLM). It proves every layer the model depends on lines up to reproduce
// Invoice-2638907B.pdf: 8 GST-free lines, total $1905.76, and that the source
// notes are linked to the created invoice afterwards.

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/dknathalage/tallyo/internal/realtime"
	"github.com/dknathalage/tallyo/internal/repository"
	"github.com/dknathalage/tallyo/internal/reqctx"
	"github.com/dknathalage/tallyo/internal/service"
)

func TestNotesToInvoiceChain(t *testing.T) {
	conn, tenantID, participantID := noteToolsFixture(t)
	ctx := reqctx.WithTenant(context.Background(), tenantID)

	notes := service.NewNoteService(conn, realtime.NewHub())
	seedReferenceNotes(t, notes, ctx, participantID)

	listTool := NewListParticipantNotesTool(notes)
	catTool := NewSearchCatalogueTool(service.NewSupportCatalogService(conn))
	invTool := NewCreateInvoiceTool(service.NewInvoiceService(conn, realtime.NewHub()), nil)

	// Step 1: the agent reads the week's journal.
	views := runListNotes(t, listTool, ctx, participantID, "2026-06-09", "2026-06-12")
	if len(views) != len(referenceWeek) {
		t.Fatalf("notes listed = %d, want %d", len(views), len(referenceWeek))
	}

	// Step 2: resolve the two activity codes + caps from the catalogue (constant
	// across the week, so look them up once on the first service date).
	transport := findByCode(runSearch(t, catTool, ctx, "transport", views[0].ServiceDate, ""), codeTransport)
	selfCare := findByCode(runSearch(t, catTool, ctx, "self care", views[0].ServiceDate, ""), codeSelfCare)
	if transport == nil || selfCare == nil {
		t.Fatalf("catalogue search did not resolve both codes: transport=%v selfCare=%v", transport, selfCare)
	}
	if transport.PriceCap == nil || selfCare.PriceCap == nil {
		t.Fatal("expected fixed price caps for both codes")
	}

	// Step 3: assemble create_invoice from the tool outputs — one transport and
	// one self-care line per note, quantities taken from the structured tags,
	// unit prices at the catalogue cap.
	type line struct {
		Code        string  `json:"code"`
		ServiceDate string  `json:"serviceDate"`
		Quantity    float64 `json:"quantity"`
		UnitPrice   float64 `json:"unitPrice"`
	}
	in := struct {
		ParticipantID int64  `json:"participantId"`
		IssueDate     string `json:"issueDate"`
		Items         []line `json:"items"`
	}{ParticipantID: participantID, IssueDate: "2026-06-14"}

	for i := range views { // bounded by len(views)
		v := views[i]
		if v.TransportKm == nil || v.SupportHours == nil {
			t.Fatalf("note %s missing structured tags", v.ServiceDate)
		}
		in.Items = append(in.Items,
			line{Code: transport.Code, ServiceDate: v.ServiceDate, Quantity: *v.TransportKm, UnitPrice: *transport.PriceCap},
			line{Code: selfCare.Code, ServiceDate: v.ServiceDate, Quantity: *v.SupportHours, UnitPrice: *selfCare.PriceCap},
		)
	}
	raw, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal create_invoice input: %v", err)
	}

	res, err := invTool.Handler(ctx, raw)
	if err != nil {
		t.Fatalf("create_invoice: %v", err)
	}
	inv, ok := res.JSON.(*repository.Invoice)
	if !ok {
		t.Fatalf("result JSON is %T, want *repository.Invoice", res.JSON)
	}
	if len(inv.LineItems) != 2*len(referenceWeek) {
		t.Fatalf("line count = %d, want %d", len(inv.LineItems), 2*len(referenceWeek))
	}
	if inv.Tax != 0 {
		t.Fatalf("Tax = %.2f, want 0 (NDIS supports are GST-free)", inv.Tax)
	}
	if inv.Total != 1905.76 {
		t.Fatalf("Total = %.2f, want 1905.76 (matches Invoice-2638907B.pdf)", inv.Total)
	}

	// Step 4: link the source notes to the invoice (the frontend's bill step).
	noteIDs := make([]int64, 0, len(views))
	for i := range views { // bounded by len(views)
		noteIDs = append(noteIDs, views[i].ID)
	}
	if err := notes.Bill(ctx, inv.ID, noteIDs); err != nil {
		t.Fatalf("bill notes: %v", err)
	}
	billed := runListNotes(t, listTool, ctx, participantID, "2026-06-09", "2026-06-12")
	for i := range billed { // bounded by len(billed)
		if billed[i].BilledID == nil || *billed[i].BilledID != inv.ID {
			t.Fatalf("note %s not linked to invoice %d after billing", billed[i].ServiceDate, inv.ID)
		}
	}
}
