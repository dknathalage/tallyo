package repository

import (
	"context"
	"encoding/json"
	"path/filepath"
	"sync"
	"testing"
	"time"

	appdb "github.com/dknathalage/tallyo/internal/db"
)

// invoiceFixture spins up a migrated DB plus an invoices repo, and seeds a
// business profile, a payer, a client (linked to that payer) and a 10% tax rate
// so snapshot building and tax math can be asserted end to end.
type invoiceFixture struct {
	repo      *InvoicesRepo
	clientID  int64
	clientNm  string
	payerID   int64
	taxRateID int64
}

func newInvoiceFixture(t *testing.T) invoiceFixture {
	t.Helper()
	conn, err := appdb.Open(filepath.Join(t.TempDir(), "invoice.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	if err := appdb.Migrate(conn); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	ctx := context.Background()
	if err := NewBusinessProfile(conn).Save(ctx, BusinessProfileInput{
		Name: "My Biz", Email: "biz@x.com", Phone: "555", Address: "1 St",
	}); err != nil {
		t.Fatalf("Save business: %v", err)
	}
	payer, err := NewPayers(conn).Create(ctx, PayerInput{Name: "PayCo", Email: "pay@x.com"})
	if err != nil {
		t.Fatalf("Create payer: %v", err)
	}
	client, err := NewClients(conn).Create(ctx, ClientInput{
		Name: "Acme", Email: "acme@x.com", Phone: "111", Address: "2 Ave", PayerID: &payer.ID,
	})
	if err != nil {
		t.Fatalf("Create client: %v", err)
	}
	tax, err := NewTaxRates(conn).Create(ctx, TaxRateInput{Name: "GST", Rate: 10})
	if err != nil {
		t.Fatalf("Create tax: %v", err)
	}
	return invoiceFixture{
		repo:      NewInvoices(conn),
		clientID:  client.ID,
		clientNm:  client.Name,
		payerID:   payer.ID,
		taxRateID: tax.ID,
	}
}

func sampleItems() []LineItemInput {
	return []LineItemInput{
		{Description: "Design", Quantity: 2, Rate: 10, SortOrder: 0},
		{Description: "Hosting", Quantity: 1, Rate: 5, SortOrder: 1},
	}
}

func TestInvoiceCreateComputesTotals(t *testing.T) {
	f := newInvoiceFixture(t)
	ctx := context.Background()

	inv, err := f.repo.Create(ctx, InvoiceInput{
		ClientID: f.clientID, Date: "2026-01-01", DueDate: "2026-02-01", TaxRate: 10,
	}, sampleItems())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if inv.Subtotal != 25 {
		t.Fatalf("Subtotal = %v, want 25", inv.Subtotal)
	}
	if inv.TaxAmount != 2.5 {
		t.Fatalf("TaxAmount = %v, want 2.5", inv.TaxAmount)
	}
	if inv.Total != 27.5 {
		t.Fatalf("Total = %v, want 27.5", inv.Total)
	}
	if inv.InvoiceNumber != "INV-0001" {
		t.Fatalf("InvoiceNumber = %q, want INV-0001", inv.InvoiceNumber)
	}
	if inv.Status != "draft" {
		t.Fatalf("Status = %q, want draft", inv.Status)
	}
	if inv.CurrencyCode != "USD" {
		t.Fatalf("CurrencyCode = %q, want USD", inv.CurrencyCode)
	}
	if inv.PaymentTerms != "custom" {
		t.Fatalf("PaymentTerms = %q, want custom", inv.PaymentTerms)
	}
	if len(inv.LineItems) != 2 {
		t.Fatalf("LineItems = %d, want 2", len(inv.LineItems))
	}
	if inv.LineItems[0].Amount != 20 || inv.LineItems[1].Amount != 5 {
		t.Fatalf("line amounts = %v/%v, want 20/5", inv.LineItems[0].Amount, inv.LineItems[1].Amount)
	}
	// default snapshots populated with the client name.
	var cs map[string]any
	if err := json.Unmarshal([]byte(inv.ClientSnapshot), &cs); err != nil {
		t.Fatalf("client snapshot not JSON: %v (%q)", err, inv.ClientSnapshot)
	}
	if cs["name"] != "Acme" {
		t.Fatalf("client snapshot name = %v, want Acme", cs["name"])
	}
	var bs map[string]any
	if err := json.Unmarshal([]byte(inv.BusinessSnapshot), &bs); err != nil {
		t.Fatalf("business snapshot not JSON: %v", err)
	}
	if bs["name"] != "My Biz" {
		t.Fatalf("business snapshot name = %v, want My Biz", bs["name"])
	}
	var ps map[string]any
	if err := json.Unmarshal([]byte(inv.PayerSnapshot), &ps); err != nil {
		t.Fatalf("payer snapshot not JSON: %v", err)
	}
	if ps["name"] != "PayCo" {
		t.Fatalf("payer snapshot name = %v, want PayCo", ps["name"])
	}
}

func TestInvoiceCreateTaxRateFromID(t *testing.T) {
	f := newInvoiceFixture(t)
	ctx := context.Background()
	// TaxRate 0 + TaxRateID set -> rate looked up from tax_rates (10%).
	inv, err := f.repo.Create(ctx, InvoiceInput{
		ClientID: f.clientID, Date: "2026-01-01", DueDate: "2026-02-01", TaxRateID: &f.taxRateID,
	}, sampleItems())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if inv.TaxRate != 10 || inv.TaxAmount != 2.5 || inv.Total != 27.5 {
		t.Fatalf("rate/tax/total = %v/%v/%v, want 10/2.5/27.5", inv.TaxRate, inv.TaxAmount, inv.Total)
	}
	if inv.TaxRateID == nil || *inv.TaxRateID != f.taxRateID {
		t.Fatalf("TaxRateID = %v, want %d", inv.TaxRateID, f.taxRateID)
	}
}

func TestInvoiceCreateExplicitSnapshots(t *testing.T) {
	f := newInvoiceFixture(t)
	ctx := context.Background()
	const customCS = `{"name":"Override","email":"o@x.com"}`
	inv, err := f.repo.Create(ctx, InvoiceInput{
		ClientID: f.clientID, Date: "2026-01-01", DueDate: "2026-02-01", TaxRate: 10,
		BusinessSnapshot: `{"name":"BizOverride"}`,
		ClientSnapshot:   customCS,
		PayerSnapshot:    `{"name":"PayerOverride"}`,
	}, sampleItems())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if inv.ClientSnapshot != customCS {
		t.Fatalf("ClientSnapshot = %q, want verbatim %q", inv.ClientSnapshot, customCS)
	}
	if inv.BusinessSnapshot != `{"name":"BizOverride"}` {
		t.Fatalf("BusinessSnapshot = %q, want verbatim", inv.BusinessSnapshot)
	}
	if inv.PayerSnapshot != `{"name":"PayerOverride"}` {
		t.Fatalf("PayerSnapshot = %q, want verbatim", inv.PayerSnapshot)
	}
}

func TestInvoiceSequentialNumbers(t *testing.T) {
	f := newInvoiceFixture(t)
	ctx := context.Background()
	first, err := f.repo.Create(ctx, InvoiceInput{ClientID: f.clientID, Date: "2026-01-01", DueDate: "2026-02-01"}, sampleItems())
	if err != nil {
		t.Fatalf("Create 1: %v", err)
	}
	second, err := f.repo.Create(ctx, InvoiceInput{ClientID: f.clientID, Date: "2026-01-01", DueDate: "2026-02-01"}, sampleItems())
	if err != nil {
		t.Fatalf("Create 2: %v", err)
	}
	if first.InvoiceNumber != "INV-0001" || second.InvoiceNumber != "INV-0002" {
		t.Fatalf("numbers = %q/%q, want INV-0001/INV-0002", first.InvoiceNumber, second.InvoiceNumber)
	}
}

func TestInvoiceGetReturnsLineItems(t *testing.T) {
	f := newInvoiceFixture(t)
	ctx := context.Background()
	created, err := f.repo.Create(ctx, InvoiceInput{ClientID: f.clientID, Date: "2026-01-01", DueDate: "2026-02-01"}, sampleItems())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	got, err := f.repo.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got == nil {
		t.Fatal("Get = nil")
	}
	if got.ClientName != "Acme" {
		t.Fatalf("ClientName = %q, want Acme", got.ClientName)
	}
	if len(got.LineItems) != 2 {
		t.Fatalf("LineItems = %d, want 2", len(got.LineItems))
	}
}

func TestInvoiceGetMissing(t *testing.T) {
	f := newInvoiceFixture(t)
	got, err := f.repo.Get(context.Background(), 9999)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != nil {
		t.Fatalf("Get = %+v, want nil", got)
	}
}

func TestInvoiceListVariants(t *testing.T) {
	f := newInvoiceFixture(t)
	ctx := context.Background()
	a, _ := f.repo.Create(ctx, InvoiceInput{ClientID: f.clientID, Date: "2026-01-01", DueDate: "2026-02-01"}, sampleItems())
	_, _ = f.repo.Create(ctx, InvoiceInput{ClientID: f.clientID, Date: "2026-01-01", DueDate: "2026-02-01", Status: "sent"}, sampleItems())

	all, err := f.repo.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("List = %d, want 2", len(all))
	}

	sent, err := f.repo.ListByStatus(ctx, "sent")
	if err != nil {
		t.Fatalf("ListByStatus: %v", err)
	}
	if len(sent) != 1 || sent[0].Status != "sent" {
		t.Fatalf("ListByStatus = %+v, want 1 sent", sent)
	}

	byClient, err := f.repo.ListClientInvoices(ctx, f.clientID)
	if err != nil {
		t.Fatalf("ListClientInvoices: %v", err)
	}
	if len(byClient) != 2 {
		t.Fatalf("ListClientInvoices = %d, want 2", len(byClient))
	}
	// list rows carry no line items.
	if a.LineItems == nil {
		t.Fatal("created invoice should carry line items")
	}
	if all[0].LineItems != nil && len(all[0].LineItems) != 0 {
		t.Fatalf("list rows should not embed line items, got %d", len(all[0].LineItems))
	}
}

func TestInvoiceUpdateRecomputesAndReplacesItems(t *testing.T) {
	f := newInvoiceFixture(t)
	ctx := context.Background()
	created, err := f.repo.Create(ctx, InvoiceInput{ClientID: f.clientID, Date: "2026-01-01", DueDate: "2026-02-01", TaxRate: 10}, sampleItems())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	newItems := []LineItemInput{{Description: "One item", Quantity: 3, Rate: 100, SortOrder: 0}}
	updated, err := f.repo.Update(ctx, created.ID, InvoiceInput{
		ClientID: f.clientID, Date: "2026-03-01", DueDate: "2026-04-01", TaxRate: 10, Status: "sent",
	}, newItems)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.Subtotal != 300 || updated.TaxAmount != 30 || updated.Total != 330 {
		t.Fatalf("totals = %v/%v/%v, want 300/30/330", updated.Subtotal, updated.TaxAmount, updated.Total)
	}
	if len(updated.LineItems) != 1 {
		t.Fatalf("LineItems = %d, want 1 (replaced)", len(updated.LineItems))
	}
	if updated.Status != "sent" || updated.Date != "2026-03-01" {
		t.Fatalf("update fields = %q/%q", updated.Status, updated.Date)
	}
	// snapshots preserved (kept from create when input empty).
	if updated.ClientSnapshot == "" {
		t.Fatal("client snapshot should be preserved on update")
	}
}

func TestInvoiceUpdateMissing(t *testing.T) {
	f := newInvoiceFixture(t)
	got, err := f.repo.Update(context.Background(), 9999, InvoiceInput{ClientID: f.clientID, Date: "x", DueDate: "y"}, sampleItems())
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if got != nil {
		t.Fatalf("Update = %+v, want nil for missing", got)
	}
}

func TestInvoiceUpdateStatus(t *testing.T) {
	f := newInvoiceFixture(t)
	ctx := context.Background()
	created, _ := f.repo.Create(ctx, InvoiceInput{ClientID: f.clientID, Date: "2026-01-01", DueDate: "2026-02-01"}, sampleItems())
	if err := f.repo.UpdateStatus(ctx, created.ID, "paid"); err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}
	got, _ := f.repo.Get(ctx, created.ID)
	if got.Status != "paid" {
		t.Fatalf("Status = %q, want paid", got.Status)
	}
}

func TestInvoiceDeleteCascadesLineItems(t *testing.T) {
	f := newInvoiceFixture(t)
	ctx := context.Background()
	created, _ := f.repo.Create(ctx, InvoiceInput{ClientID: f.clientID, Date: "2026-01-01", DueDate: "2026-02-01"}, sampleItems())
	if err := f.repo.Delete(ctx, created.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	got, _ := f.repo.Get(ctx, created.ID)
	if got != nil {
		t.Fatalf("Get after delete = %+v, want nil", got)
	}
	var count int
	if err := f.repo.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM line_items WHERE invoice_id = ?", created.ID).Scan(&count); err != nil {
		t.Fatalf("count line items: %v", err)
	}
	if count != 0 {
		t.Fatalf("line_items count = %d, want 0 (cascade)", count)
	}
}

func TestInvoiceBulkDelete(t *testing.T) {
	f := newInvoiceFixture(t)
	ctx := context.Background()
	a, _ := f.repo.Create(ctx, InvoiceInput{ClientID: f.clientID, Date: "2026-01-01", DueDate: "2026-02-01"}, sampleItems())
	b, _ := f.repo.Create(ctx, InvoiceInput{ClientID: f.clientID, Date: "2026-01-01", DueDate: "2026-02-01"}, sampleItems())
	if err := f.repo.BulkDelete(ctx, []int64{a.ID, b.ID}); err != nil {
		t.Fatalf("BulkDelete: %v", err)
	}
	all, _ := f.repo.List(ctx)
	if len(all) != 0 {
		t.Fatalf("List after bulk delete = %d, want 0", len(all))
	}
	// empty no-op.
	if err := f.repo.BulkDelete(ctx, nil); err != nil {
		t.Fatalf("BulkDelete(nil): %v", err)
	}
}

func TestInvoiceBulkUpdateStatus(t *testing.T) {
	f := newInvoiceFixture(t)
	ctx := context.Background()
	a, _ := f.repo.Create(ctx, InvoiceInput{ClientID: f.clientID, Date: "2026-01-01", DueDate: "2026-02-01"}, sampleItems())
	b, _ := f.repo.Create(ctx, InvoiceInput{ClientID: f.clientID, Date: "2026-01-01", DueDate: "2026-02-01"}, sampleItems())
	if err := f.repo.BulkUpdateStatus(ctx, []int64{a.ID, b.ID}, "sent"); err != nil {
		t.Fatalf("BulkUpdateStatus: %v", err)
	}
	sent, _ := f.repo.ListByStatus(ctx, "sent")
	if len(sent) != 2 {
		t.Fatalf("sent = %d, want 2", len(sent))
	}
}

func TestInvoiceDuplicate(t *testing.T) {
	f := newInvoiceFixture(t)
	ctx := context.Background()
	src, err := f.repo.Create(ctx, InvoiceInput{
		ClientID: f.clientID, Date: "2026-01-01", DueDate: "2026-02-01", TaxRate: 10,
		Notes: "original", CurrencyCode: "EUR",
	}, sampleItems())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	dup, err := f.repo.Duplicate(ctx, src.ID)
	if err != nil {
		t.Fatalf("Duplicate: %v", err)
	}
	if dup.InvoiceNumber == src.InvoiceNumber {
		t.Fatalf("Duplicate number = %q, want different from %q", dup.InvoiceNumber, src.InvoiceNumber)
	}
	if dup.InvoiceNumber != "INV-0002" {
		t.Fatalf("Duplicate number = %q, want INV-0002", dup.InvoiceNumber)
	}
	if dup.Status != "draft" {
		t.Fatalf("Duplicate status = %q, want draft", dup.Status)
	}
	if dup.DueDate != "" {
		t.Fatalf("Duplicate dueDate = %q, want empty", dup.DueDate)
	}
	if dup.PaymentTerms != "custom" {
		t.Fatalf("Duplicate paymentTerms = %q, want custom", dup.PaymentTerms)
	}
	if dup.TaxRateID != nil {
		t.Fatalf("Duplicate taxRateId = %v, want nil", dup.TaxRateID)
	}
	today := time.Now().UTC().Format("2006-01-02")
	if dup.Date != today {
		t.Fatalf("Duplicate date = %q, want today %q", dup.Date, today)
	}
	// totals recomputed from copied items + source rate.
	if dup.Subtotal != 25 || dup.Total != 27.5 {
		t.Fatalf("Duplicate totals = %v/%v, want 25/27.5", dup.Subtotal, dup.Total)
	}
	if len(dup.LineItems) != 2 {
		t.Fatalf("Duplicate items = %d, want 2", len(dup.LineItems))
	}
	if dup.CurrencyCode != "EUR" || dup.Notes != "original" {
		t.Fatalf("Duplicate currency/notes = %q/%q", dup.CurrencyCode, dup.Notes)
	}
}

func TestInvoiceMarkOverdue(t *testing.T) {
	f := newInvoiceFixture(t)
	ctx := context.Background()
	yesterday := time.Now().UTC().AddDate(0, 0, -1).Format("2006-01-02")
	created, _ := f.repo.Create(ctx, InvoiceInput{ClientID: f.clientID, Date: "2026-01-01", DueDate: yesterday, Status: "sent"}, sampleItems())

	rows, err := f.repo.MarkOverdue(ctx)
	if err != nil {
		t.Fatalf("MarkOverdue: %v", err)
	}
	if len(rows) != 1 || rows[0].ID != created.ID {
		t.Fatalf("MarkOverdue = %+v, want 1 row for id %d", rows, created.ID)
	}
	if rows[0].InvoiceNumber != created.InvoiceNumber {
		t.Fatalf("overdue number = %q, want %q", rows[0].InvoiceNumber, created.InvoiceNumber)
	}
	got, _ := f.repo.Get(ctx, created.ID)
	if got.Status != "overdue" {
		t.Fatalf("Status = %q, want overdue", got.Status)
	}
	// idempotent: no longer 'sent', so empty.
	again, err := f.repo.MarkOverdue(ctx)
	if err != nil {
		t.Fatalf("MarkOverdue 2: %v", err)
	}
	if len(again) != 0 {
		t.Fatalf("MarkOverdue again = %d, want 0", len(again))
	}
}

func TestInvoiceClientStats(t *testing.T) {
	f := newInvoiceFixture(t)
	ctx := context.Background()
	_, _ = f.repo.Create(ctx, InvoiceInput{ClientID: f.clientID, Date: "2026-01-01", DueDate: "2026-02-01", TaxRate: 10}, sampleItems())
	_, _ = f.repo.Create(ctx, InvoiceInput{ClientID: f.clientID, Date: "2026-01-01", DueDate: "2026-02-01", TaxRate: 10}, sampleItems())
	stats, err := f.repo.ClientStats(ctx, f.clientID)
	if err != nil {
		t.Fatalf("ClientStats: %v", err)
	}
	if stats.InvoiceCount != 2 {
		t.Fatalf("InvoiceCount = %d, want 2", stats.InvoiceCount)
	}
	if stats.TotalInvoiced != 55 {
		t.Fatalf("TotalInvoiced = %v, want 55", stats.TotalInvoiced)
	}
}

func TestInvoiceValidation(t *testing.T) {
	f := newInvoiceFixture(t)
	ctx := context.Background()
	if _, err := f.repo.Create(ctx, InvoiceInput{ClientID: 0, Date: "x", DueDate: "y"}, sampleItems()); err == nil {
		t.Fatal("Create with no client should error")
	}
	if _, err := f.repo.Create(ctx, InvoiceInput{ClientID: f.clientID, Date: "x", DueDate: "y"}, nil); err == nil {
		t.Fatal("Create with no items should error")
	}
}

func TestInvoiceConcurrentDistinctNumbers(t *testing.T) {
	f := newInvoiceFixture(t)
	ctx := context.Background()
	const workers = 8
	var wg sync.WaitGroup
	errs := make(chan error, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := f.repo.Create(ctx, InvoiceInput{
				ClientID: f.clientID, Date: "2026-01-01", DueDate: "2026-02-01",
			}, sampleItems())
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		if e != nil {
			t.Fatalf("concurrent Create: %v", e)
		}
	}
	var distinct int
	if err := f.repo.db.QueryRowContext(ctx, "SELECT COUNT(DISTINCT invoice_number) FROM invoices").Scan(&distinct); err != nil {
		t.Fatalf("count distinct: %v", err)
	}
	if distinct != workers {
		t.Fatalf("distinct invoice numbers = %d, want %d", distinct, workers)
	}
}
