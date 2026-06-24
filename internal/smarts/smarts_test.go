package smarts

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/dknathalage/tallyo/internal/billing"
	"github.com/dknathalage/tallyo/internal/client"
	"github.com/dknathalage/tallyo/internal/invoice"
	"github.com/dknathalage/tallyo/internal/pricelist"
	"github.com/dknathalage/tallyo/internal/reqctx"
	"github.com/dknathalage/tallyo/internal/session"
)

// --- fakes ---------------------------------------------------------------

type fakeProposer struct {
	propose  json.RawMessage // returned by Propose
	grounded json.RawMessage // returned by ProposeGrounded
	lastUser string          // captures the user prompt of the last call
}

func (f *fakeProposer) Propose(_ context.Context, r ProposeRequest) (json.RawMessage, error) {
	f.lastUser = r.User
	return f.propose, nil
}
func (f *fakeProposer) ProposeGrounded(_ context.Context, r GroundedRequest) (json.RawMessage, error) {
	f.lastUser = r.User
	return f.grounded, nil
}

type fakeSessions struct{ rows []*session.Session }

func (f *fakeSessions) ListUnbilledForClient(_ context.Context, _, _ string) ([]*session.Session, error) {
	return f.rows, nil
}

type fakeCat struct {
	ver   *pricelist.PriceListVersion
	items map[string]*pricelist.Item // by code
}

func (f *fakeCat) ResolveVersionForDate(_ context.Context, _ string, _ string) (*pricelist.PriceListVersion, error) {
	return f.ver, nil
}
func (f *fakeCat) SearchItems(_ context.Context, _, _ string, _ string) ([]*pricelist.Item, error) {
	out := make([]*pricelist.Item, 0, len(f.items))
	for _, it := range f.items {
		out = append(out, it)
	}
	return out, nil
}
func (f *fakeCat) GetItemByCode(_ context.Context, _, _ string, code string) (*pricelist.Item, error) {
	return f.items[code], nil
}

type fakeInvoices struct {
	gotInput invoice.InvoiceInput
	gotItems []billing.LineItemInput
}

func (f *fakeInvoices) Create(_ context.Context, in invoice.InvoiceInput, items []billing.LineItemInput) (*invoice.Invoice, error) {
	f.gotInput = in
	f.gotItems = items
	return &invoice.Invoice{ID: "inv-uuid"}, nil
}
func (f *fakeInvoices) GetByUUID(_ context.Context, _ string) (*invoice.Invoice, error) {
	return &invoice.Invoice{Number: "INV-1", ClientName: "Acme", Total: 250, DueDate: "2026-06-01"}, nil
}

type fakeClients struct{ c *client.Client }

func (f *fakeClients) Get(_ context.Context, _ string) (*client.Client, error) { return f.c, nil }

func tctx() context.Context { return reqctx.WithTenant(context.Background(), "t-1") }

func price(p float64) *float64 { return &p }

// --- tests ---------------------------------------------------------------

// The model proposes a code + quantity; the catalogue sets the price. A model
// that tried to send its own price cannot — there is no price field in the
// proposal — so the invoice line is always priced from the catalogue.
func TestDraftInvoicePricesFromCatalogue(t *testing.T) {
	cat := &fakeCat{
		ver:   &pricelist.PriceListVersion{ID: "ver-uuid"},
		items: map[string]*pricelist.Item{"CONSULT": {ID: "item-uuid", Code: "CONSULT", Unit: "hour", UnitPrice: price(100), Taxable: true}},
	}
	inv := &fakeInvoices{}
	svc := NewService(
		&fakeProposer{grounded: json.RawMessage(`{"items":[{"code":"CONSULT","description":"Consulting","quantity":2,"serviceDate":"2026-06-01"}]}`)},
		&fakeSessions{rows: []*session.Session{{ID: "s1", ServiceDate: "2026-06-01", Note: "2h consulting"}}},
		cat, inv, inv, &fakeClients{c: &client.Client{ID: "c1"}},
	)

	uuid, err := svc.DraftInvoiceFromSessions(tctx(), "c1")
	if err != nil {
		t.Fatalf("DraftInvoiceFromSessions: %v", err)
	}
	if uuid != "inv-uuid" {
		t.Fatalf("uuid = %q, want inv-uuid", uuid)
	}
	if inv.gotInput.Status != "draft" {
		t.Fatalf("status = %q, want draft", inv.gotInput.Status)
	}
	if inv.gotInput.ClientID != "c1" {
		t.Fatalf("clientID = %q, want c1", inv.gotInput.ClientID)
	}
	if len(inv.gotItems) != 1 {
		t.Fatalf("items = %d, want 1", len(inv.gotItems))
	}
	li := inv.gotItems[0]
	if li.UnitPrice != 100 {
		t.Fatalf("unit price = %v, want 100 (from catalogue, not model)", li.UnitPrice)
	}
	if li.Code != "CONSULT" || li.Quantity != 2 || !li.Taxable {
		t.Fatalf("line = %+v, want CONSULT x2 taxable", li)
	}
	if li.ItemID == nil || *li.ItemID != "item-uuid" || li.PriceListVersionID == nil || *li.PriceListVersionID != "ver-uuid" {
		t.Fatalf("line not pinned to catalogue item/version: %+v", li)
	}
}

// A proposed code that is not in the catalogue is dropped, not guessed.
func TestDraftInvoiceDropsUnknownCodes(t *testing.T) {
	cat := &fakeCat{ver: &pricelist.PriceListVersion{ID: "v"}, items: map[string]*pricelist.Item{}}
	inv := &fakeInvoices{}
	svc := NewService(
		&fakeProposer{grounded: json.RawMessage(`{"items":[{"code":"NOPE","quantity":1}]}`)},
		&fakeSessions{rows: []*session.Session{{ServiceDate: "2026-06-01", Note: "x"}}},
		cat, inv, inv, &fakeClients{c: &client.Client{ID: "c1"}},
	)
	_, err := svc.DraftInvoiceFromSessions(tctx(), "c1")
	if err == nil {
		t.Fatal("want ErrNoData when no proposed code resolves, got nil")
	}
}

// No unbilled sessions → a clean data error, no model call needed.
func TestDraftInvoiceNoSessions(t *testing.T) {
	svc := NewService(
		&fakeProposer{},
		&fakeSessions{rows: nil},
		&fakeCat{}, &fakeInvoices{}, &fakeInvoices{}, &fakeClients{c: &client.Client{ID: "c1"}},
	)
	if _, err := svc.DraftInvoiceFromSessions(tctx(), "c1"); err == nil {
		t.Fatal("want error for no unbilled sessions")
	}
}

// The follow-up Smart returns the model's draft and includes invoice facts in
// the prompt.
func TestDraftFollowUp(t *testing.T) {
	fp := &fakeProposer{propose: json.RawMessage(`{"subject":"Reminder: INV-1","body":"Your invoice is overdue."}`)}
	inv := &fakeInvoices{}
	svc := NewService(fp, &fakeSessions{}, &fakeCat{}, inv, inv, &fakeClients{})
	fu, err := svc.DraftFollowUp(tctx(), "inv-1")
	if err != nil {
		t.Fatalf("DraftFollowUp: %v", err)
	}
	if fu.Subject == "" || fu.Body == "" {
		t.Fatalf("empty draft: %+v", fu)
	}
}

// Map-import keeps only known target fields and known headers.
func TestMapImportDropsUnknownTargets(t *testing.T) {
	fp := &fakeProposer{propose: json.RawMessage(`{"mappings":[{"header":"Item Code","field":"code"},{"header":"Price","field":"unitPrice"},{"header":"Junk","field":"bogus"},{"header":"Ghost","field":"name"}]}`)}
	svc := NewService(fp, &fakeSessions{}, &fakeCat{}, &fakeInvoices{}, &fakeInvoices{}, &fakeClients{})
	res, err := svc.MapImport(tctx(), MapInput{Headers: []string{"Item Code", "Price", "Junk"}})
	if err != nil {
		t.Fatalf("MapImport: %v", err)
	}
	if res.Mappings["Item Code"] != "code" || res.Mappings["Price"] != "unitPrice" {
		t.Fatalf("expected valid mappings kept, got %+v", res.Mappings)
	}
	if _, ok := res.Mappings["Junk"]; ok {
		t.Fatalf("unknown target field should be dropped: %+v", res.Mappings)
	}
	if _, ok := res.Mappings["Ghost"]; ok {
		t.Fatalf("unknown header should be dropped: %+v", res.Mappings)
	}
}

func TestSupportsTuning(t *testing.T) {
	// Haiku 4.5 rejects adaptive thinking + effort (400); frontier models accept them.
	if supportsTuning("claude-haiku-4-5") {
		t.Fatal("haiku must NOT be tuned (thinking/effort would 400)")
	}
	if !supportsTuning("claude-opus-4-8") {
		t.Fatal("opus must be tuned")
	}
}
