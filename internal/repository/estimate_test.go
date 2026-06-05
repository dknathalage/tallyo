package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	appdb "github.com/dknathalage/tallyo/internal/db"
)

// estimateFixture spins up a migrated DB plus an estimates repo, seeding a
// business profile, a payer, a client (linked to that payer) and a 10% tax rate
// so snapshot building and tax math can be asserted end to end.
type estimateFixture struct {
	conn      *sql.DB
	repo      *EstimatesRepo
	clientID  int64
	clientNm  string
	payerID   int64
	taxRateID int64
}

func newEstimateFixture(t *testing.T) estimateFixture {
	t.Helper()
	conn, err := appdb.Open(filepath.Join(t.TempDir(), "estimate.db"))
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
	return estimateFixture{
		conn:      conn,
		repo:      NewEstimates(conn),
		clientID:  client.ID,
		clientNm:  client.Name,
		payerID:   payer.ID,
		taxRateID: tax.ID,
	}
}

func TestEstimateCreateComputesTotals(t *testing.T) {
	f := newEstimateFixture(t)
	ctx := context.Background()

	est, err := f.repo.Create(ctx, EstimateInput{
		ClientID: f.clientID, Date: "2026-01-01", ValidUntil: "2026-02-01", TaxRate: 10,
	}, sampleItems())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if est.Subtotal != 25 {
		t.Fatalf("Subtotal = %v, want 25", est.Subtotal)
	}
	if est.TaxAmount != 2.5 {
		t.Fatalf("TaxAmount = %v, want 2.5", est.TaxAmount)
	}
	if est.Total != 27.5 {
		t.Fatalf("Total = %v, want 27.5", est.Total)
	}
	if est.EstimateNumber != "EST-0001" {
		t.Fatalf("EstimateNumber = %q, want EST-0001", est.EstimateNumber)
	}
	if est.Status != "draft" {
		t.Fatalf("Status = %q, want draft", est.Status)
	}
	if est.CurrencyCode != "USD" {
		t.Fatalf("CurrencyCode = %q, want USD", est.CurrencyCode)
	}
	if est.ClientID != f.clientID {
		t.Fatalf("ClientID = %d, want %d", est.ClientID, f.clientID)
	}
	if len(est.LineItems) != 2 {
		t.Fatalf("LineItems = %d, want 2", len(est.LineItems))
	}
	if est.LineItems[0].Amount != 20 || est.LineItems[1].Amount != 5 {
		t.Fatalf("line amounts = %v/%v, want 20/5", est.LineItems[0].Amount, est.LineItems[1].Amount)
	}
	if est.ConvertedInvoiceID != nil {
		t.Fatalf("ConvertedInvoiceID = %v, want nil", est.ConvertedInvoiceID)
	}
	// default snapshots populated.
	var cs map[string]any
	if err := json.Unmarshal([]byte(est.ClientSnapshot), &cs); err != nil {
		t.Fatalf("client snapshot not JSON: %v (%q)", err, est.ClientSnapshot)
	}
	if cs["name"] != "Acme" {
		t.Fatalf("client snapshot name = %v, want Acme", cs["name"])
	}
	var bs map[string]any
	if err := json.Unmarshal([]byte(est.BusinessSnapshot), &bs); err != nil {
		t.Fatalf("business snapshot not JSON: %v", err)
	}
	if bs["name"] != "My Biz" {
		t.Fatalf("business snapshot name = %v, want My Biz", bs["name"])
	}
	var ps map[string]any
	if err := json.Unmarshal([]byte(est.PayerSnapshot), &ps); err != nil {
		t.Fatalf("payer snapshot not JSON: %v", err)
	}
	if ps["name"] != "PayCo" {
		t.Fatalf("payer snapshot name = %v, want PayCo", ps["name"])
	}
}

func TestEstimateCreateTaxRateFromID(t *testing.T) {
	f := newEstimateFixture(t)
	ctx := context.Background()
	est, err := f.repo.Create(ctx, EstimateInput{
		ClientID: f.clientID, Date: "2026-01-01", ValidUntil: "2026-02-01", TaxRateID: &f.taxRateID,
	}, sampleItems())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if est.TaxRate != 10 || est.TaxAmount != 2.5 || est.Total != 27.5 {
		t.Fatalf("rate/tax/total = %v/%v/%v, want 10/2.5/27.5", est.TaxRate, est.TaxAmount, est.Total)
	}
	if est.TaxRateID == nil || *est.TaxRateID != f.taxRateID {
		t.Fatalf("TaxRateID = %v, want %d", est.TaxRateID, f.taxRateID)
	}
}

func TestEstimateCreateExplicitSnapshots(t *testing.T) {
	f := newEstimateFixture(t)
	ctx := context.Background()
	const customCS = `{"name":"Override","email":"o@x.com"}`
	est, err := f.repo.Create(ctx, EstimateInput{
		ClientID: f.clientID, Date: "2026-01-01", ValidUntil: "2026-02-01", TaxRate: 10,
		BusinessSnapshot: `{"name":"BizOverride"}`,
		ClientSnapshot:   customCS,
		PayerSnapshot:    `{"name":"PayerOverride"}`,
	}, sampleItems())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if est.ClientSnapshot != customCS {
		t.Fatalf("ClientSnapshot = %q, want verbatim %q", est.ClientSnapshot, customCS)
	}
	if est.BusinessSnapshot != `{"name":"BizOverride"}` {
		t.Fatalf("BusinessSnapshot = %q, want verbatim", est.BusinessSnapshot)
	}
	if est.PayerSnapshot != `{"name":"PayerOverride"}` {
		t.Fatalf("PayerSnapshot = %q, want verbatim", est.PayerSnapshot)
	}
}

func TestEstimateSequentialNumbers(t *testing.T) {
	f := newEstimateFixture(t)
	ctx := context.Background()
	first, err := f.repo.Create(ctx, EstimateInput{ClientID: f.clientID, Date: "2026-01-01", ValidUntil: "2026-02-01"}, sampleItems())
	if err != nil {
		t.Fatalf("Create 1: %v", err)
	}
	second, err := f.repo.Create(ctx, EstimateInput{ClientID: f.clientID, Date: "2026-01-01", ValidUntil: "2026-02-01"}, sampleItems())
	if err != nil {
		t.Fatalf("Create 2: %v", err)
	}
	if first.EstimateNumber != "EST-0001" || second.EstimateNumber != "EST-0002" {
		t.Fatalf("numbers = %q/%q, want EST-0001/EST-0002", first.EstimateNumber, second.EstimateNumber)
	}
}

func TestEstimateGetReturnsLineItems(t *testing.T) {
	f := newEstimateFixture(t)
	ctx := context.Background()
	created, err := f.repo.Create(ctx, EstimateInput{ClientID: f.clientID, Date: "2026-01-01", ValidUntil: "2026-02-01"}, sampleItems())
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

func TestEstimateGetMissing(t *testing.T) {
	f := newEstimateFixture(t)
	got, err := f.repo.Get(context.Background(), 9999)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != nil {
		t.Fatalf("Get = %+v, want nil", got)
	}
}

func TestEstimateListVariants(t *testing.T) {
	f := newEstimateFixture(t)
	ctx := context.Background()
	a, _ := f.repo.Create(ctx, EstimateInput{ClientID: f.clientID, Date: "2026-01-01", ValidUntil: "2026-02-01"}, sampleItems())
	_, _ = f.repo.Create(ctx, EstimateInput{ClientID: f.clientID, Date: "2026-01-01", ValidUntil: "2026-02-01", Status: "accepted"}, sampleItems())

	all, err := f.repo.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("List = %d, want 2", len(all))
	}

	accepted, err := f.repo.ListByStatus(ctx, "accepted")
	if err != nil {
		t.Fatalf("ListByStatus: %v", err)
	}
	if len(accepted) != 1 || accepted[0].Status != "accepted" {
		t.Fatalf("ListByStatus = %+v, want 1 accepted", accepted)
	}

	byClient, err := f.repo.ListClientEstimates(ctx, f.clientID)
	if err != nil {
		t.Fatalf("ListClientEstimates: %v", err)
	}
	if len(byClient) != 2 {
		t.Fatalf("ListClientEstimates = %d, want 2", len(byClient))
	}
	if a.LineItems == nil {
		t.Fatal("created estimate should carry line items")
	}
	if all[0].LineItems != nil && len(all[0].LineItems) != 0 {
		t.Fatalf("list rows should not embed line items, got %d", len(all[0].LineItems))
	}
}

func TestEstimateUpdateRecomputesAndReplacesItems(t *testing.T) {
	f := newEstimateFixture(t)
	ctx := context.Background()
	created, err := f.repo.Create(ctx, EstimateInput{ClientID: f.clientID, Date: "2026-01-01", ValidUntil: "2026-02-01", TaxRate: 10}, sampleItems())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	newItems := []LineItemInput{{Description: "One item", Quantity: 3, Rate: 100, SortOrder: 0}}
	updated, err := f.repo.Update(ctx, created.ID, EstimateInput{
		ClientID: f.clientID, Date: "2026-03-01", ValidUntil: "2026-04-01", TaxRate: 10, Status: "accepted",
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
	if updated.Status != "accepted" || updated.Date != "2026-03-01" {
		t.Fatalf("update fields = %q/%q", updated.Status, updated.Date)
	}
	if updated.ValidUntil != "2026-04-01" {
		t.Fatalf("ValidUntil = %q, want 2026-04-01", updated.ValidUntil)
	}
	if updated.ClientSnapshot == "" {
		t.Fatal("client snapshot should be preserved on update")
	}
}

func TestEstimateUpdateMissing(t *testing.T) {
	f := newEstimateFixture(t)
	got, err := f.repo.Update(context.Background(), 9999, EstimateInput{ClientID: f.clientID, Date: "x", ValidUntil: "y"}, sampleItems())
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if got != nil {
		t.Fatalf("Update = %+v, want nil for missing", got)
	}
}

func TestEstimateUpdateStatus(t *testing.T) {
	f := newEstimateFixture(t)
	ctx := context.Background()
	created, _ := f.repo.Create(ctx, EstimateInput{ClientID: f.clientID, Date: "2026-01-01", ValidUntil: "2026-02-01"}, sampleItems())
	if err := f.repo.UpdateStatus(ctx, created.ID, "declined"); err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}
	got, _ := f.repo.Get(ctx, created.ID)
	if got.Status != "declined" {
		t.Fatalf("Status = %q, want declined", got.Status)
	}
}

func TestEstimateDeleteCascadesLineItems(t *testing.T) {
	f := newEstimateFixture(t)
	ctx := context.Background()
	created, _ := f.repo.Create(ctx, EstimateInput{ClientID: f.clientID, Date: "2026-01-01", ValidUntil: "2026-02-01"}, sampleItems())
	if err := f.repo.Delete(ctx, created.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	got, _ := f.repo.Get(ctx, created.ID)
	if got != nil {
		t.Fatalf("Get after delete = %+v, want nil", got)
	}
	var count int
	if err := f.repo.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM estimate_line_items WHERE estimate_id = ?", created.ID).Scan(&count); err != nil {
		t.Fatalf("count line items: %v", err)
	}
	if count != 0 {
		t.Fatalf("estimate_line_items count = %d, want 0 (cascade)", count)
	}
}

func TestEstimateBulkDelete(t *testing.T) {
	f := newEstimateFixture(t)
	ctx := context.Background()
	a, _ := f.repo.Create(ctx, EstimateInput{ClientID: f.clientID, Date: "2026-01-01", ValidUntil: "2026-02-01"}, sampleItems())
	b, _ := f.repo.Create(ctx, EstimateInput{ClientID: f.clientID, Date: "2026-01-01", ValidUntil: "2026-02-01"}, sampleItems())
	if err := f.repo.BulkDelete(ctx, []int64{a.ID, b.ID}); err != nil {
		t.Fatalf("BulkDelete: %v", err)
	}
	all, _ := f.repo.List(ctx)
	if len(all) != 0 {
		t.Fatalf("List after bulk delete = %d, want 0", len(all))
	}
	if err := f.repo.BulkDelete(ctx, nil); err != nil {
		t.Fatalf("BulkDelete(nil): %v", err)
	}
}

func TestEstimateBulkUpdateStatus(t *testing.T) {
	f := newEstimateFixture(t)
	ctx := context.Background()
	a, _ := f.repo.Create(ctx, EstimateInput{ClientID: f.clientID, Date: "2026-01-01", ValidUntil: "2026-02-01"}, sampleItems())
	b, _ := f.repo.Create(ctx, EstimateInput{ClientID: f.clientID, Date: "2026-01-01", ValidUntil: "2026-02-01"}, sampleItems())
	if err := f.repo.BulkUpdateStatus(ctx, []int64{a.ID, b.ID}, "accepted"); err != nil {
		t.Fatalf("BulkUpdateStatus: %v", err)
	}
	accepted, _ := f.repo.ListByStatus(ctx, "accepted")
	if len(accepted) != 2 {
		t.Fatalf("accepted = %d, want 2", len(accepted))
	}
}

func TestEstimateDuplicate(t *testing.T) {
	f := newEstimateFixture(t)
	ctx := context.Background()
	src, err := f.repo.Create(ctx, EstimateInput{
		ClientID: f.clientID, Date: "2026-01-01", ValidUntil: "2026-02-01", TaxRate: 10,
		Notes: "original", CurrencyCode: "EUR", Status: "accepted",
	}, sampleItems())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	dup, err := f.repo.Duplicate(ctx, src.ID)
	if err != nil {
		t.Fatalf("Duplicate: %v", err)
	}
	if dup.EstimateNumber == src.EstimateNumber {
		t.Fatalf("Duplicate number = %q, want different from %q", dup.EstimateNumber, src.EstimateNumber)
	}
	if dup.EstimateNumber != "EST-0002" {
		t.Fatalf("Duplicate number = %q, want EST-0002", dup.EstimateNumber)
	}
	if dup.Status != "draft" {
		t.Fatalf("Duplicate status = %q, want draft", dup.Status)
	}
	if dup.ValidUntil != "" {
		t.Fatalf("Duplicate validUntil = %q, want empty", dup.ValidUntil)
	}
	if dup.TaxRateID != nil {
		t.Fatalf("Duplicate taxRateId = %v, want nil", dup.TaxRateID)
	}
	today := time.Now().UTC().Format("2006-01-02")
	if dup.Date != today {
		t.Fatalf("Duplicate date = %q, want today %q", dup.Date, today)
	}
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

func TestEstimateValidation(t *testing.T) {
	f := newEstimateFixture(t)
	ctx := context.Background()
	if _, err := f.repo.Create(ctx, EstimateInput{ClientID: 0, Date: "x", ValidUntil: "y"}, sampleItems()); err == nil {
		t.Fatal("Create with no client should error")
	}
	if _, err := f.repo.Create(ctx, EstimateInput{ClientID: f.clientID, Date: "x", ValidUntil: "y"}, nil); err == nil {
		t.Fatal("Create with no items should error")
	}
}

func TestEstimateConvertNotAccepted(t *testing.T) {
	f := newEstimateFixture(t)
	ctx := context.Background()
	created, err := f.repo.Create(ctx, EstimateInput{ClientID: f.clientID, Date: "2026-01-01", ValidUntil: "2026-02-01", TaxRate: 10}, sampleItems())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	_, err = f.repo.Convert(ctx, created.ID)
	if !errors.Is(err, ErrNotAccepted) {
		t.Fatalf("Convert draft err = %v, want ErrNotAccepted", err)
	}
}

func TestEstimateConvertCreatesInvoice(t *testing.T) {
	f := newEstimateFixture(t)
	ctx := context.Background()
	created, err := f.repo.Create(ctx, EstimateInput{ClientID: f.clientID, Date: "2026-01-01", ValidUntil: "2026-02-01", TaxRate: 10}, sampleItems())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := f.repo.UpdateStatus(ctx, created.ID, "accepted"); err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}

	res, err := f.repo.Convert(ctx, created.ID)
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	if res == nil {
		t.Fatal("Convert = nil result")
	}
	if res.InvoiceNumber != "INV-0001" {
		t.Fatalf("InvoiceNumber = %q, want INV-0001", res.InvoiceNumber)
	}
	if res.EstimateNumber != created.EstimateNumber {
		t.Fatalf("EstimateNumber = %q, want %q", res.EstimateNumber, created.EstimateNumber)
	}

	// estimate now converted with the invoice id set.
	gotEst, _ := f.repo.Get(ctx, created.ID)
	if gotEst.Status != "converted" {
		t.Fatalf("estimate status = %q, want converted", gotEst.Status)
	}
	if gotEst.ConvertedInvoiceID == nil || *gotEst.ConvertedInvoiceID != res.InvoiceID {
		t.Fatalf("ConvertedInvoiceID = %v, want %d", gotEst.ConvertedInvoiceID, res.InvoiceID)
	}

	// a real invoice exists with copied items and matching totals.
	inv, err := NewInvoices(f.conn).Get(ctx, res.InvoiceID)
	if err != nil {
		t.Fatalf("Get invoice: %v", err)
	}
	if inv == nil {
		t.Fatal("converted invoice missing")
	}
	if inv.Total != 27.5 {
		t.Fatalf("invoice total = %v, want 27.5", inv.Total)
	}
	if inv.Status != "draft" {
		t.Fatalf("invoice status = %q, want draft", inv.Status)
	}
	if inv.DueDate != created.ValidUntil {
		t.Fatalf("invoice dueDate = %q, want validUntil %q", inv.DueDate, created.ValidUntil)
	}
	if len(inv.LineItems) != 2 {
		t.Fatalf("invoice items = %d, want 2", len(inv.LineItems))
	}
	if inv.ClientID != f.clientID {
		t.Fatalf("invoice clientID = %d, want %d", inv.ClientID, f.clientID)
	}
}

func TestEstimateConvertAlreadyConverted(t *testing.T) {
	f := newEstimateFixture(t)
	ctx := context.Background()
	created, _ := f.repo.Create(ctx, EstimateInput{ClientID: f.clientID, Date: "2026-01-01", ValidUntil: "2026-02-01", TaxRate: 10}, sampleItems())
	if err := f.repo.UpdateStatus(ctx, created.ID, "accepted"); err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}
	if _, err := f.repo.Convert(ctx, created.ID); err != nil {
		t.Fatalf("first Convert: %v", err)
	}
	_, err := f.repo.Convert(ctx, created.ID)
	if !errors.Is(err, ErrAlreadyConverted) {
		t.Fatalf("second Convert err = %v, want ErrAlreadyConverted", err)
	}
}

func TestEstimateConcurrentDistinctNumbers(t *testing.T) {
	f := newEstimateFixture(t)
	ctx := context.Background()
	const workers = 8
	var wg sync.WaitGroup
	errs := make(chan error, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := f.repo.Create(ctx, EstimateInput{
				ClientID: f.clientID, Date: "2026-01-01", ValidUntil: "2026-02-01",
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
	if err := f.repo.db.QueryRowContext(ctx, "SELECT COUNT(DISTINCT estimate_number) FROM estimates").Scan(&distinct); err != nil {
		t.Fatalf("count distinct: %v", err)
	}
	if distinct != workers {
		t.Fatalf("distinct estimate numbers = %d, want %d", distinct, workers)
	}
}
