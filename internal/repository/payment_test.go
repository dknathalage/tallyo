package repository

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	appdb "github.com/dknathalage/tallyo/internal/db"
)

// paymentFixture spins up a migrated DB with a clients repo, an invoices repo
// and a payments repo, plus a seeded client so invoices can be created.
type paymentFixture struct {
	conn     *sql.DB
	clients  *ClientsRepo
	invoices *InvoicesRepo
	payments *PaymentsRepo
	clientID int64
}

func newPaymentFixture(t *testing.T) paymentFixture {
	t.Helper()
	conn, err := appdb.Open(filepath.Join(t.TempDir(), "payment.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	if err := appdb.Migrate(conn); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	ctx := context.Background()
	client, err := NewClients(conn).Create(ctx, ClientInput{Name: "Acme", Email: "a@x.com"})
	if err != nil {
		t.Fatalf("Create client: %v", err)
	}
	return paymentFixture{
		conn:     conn,
		clients:  NewClients(conn),
		invoices: NewInvoices(conn),
		payments: NewPayments(conn),
		clientID: client.ID,
	}
}

// newInvoice creates an invoice with a single line item (rate 25, qty 1, no tax)
// so the total is a clean 25.
func (f paymentFixture) newInvoice(t *testing.T) *Invoice {
	t.Helper()
	inv, err := f.invoices.Create(context.Background(), InvoiceInput{
		ClientID: f.clientID, Date: "2026-06-01", DueDate: "2026-07-01",
	}, []LineItemInput{{Description: "Work", Quantity: 1, Rate: 25}})
	if err != nil {
		t.Fatalf("Create invoice: %v", err)
	}
	if inv.Total != 25 {
		t.Fatalf("invoice total = %v, want 25", inv.Total)
	}
	return inv
}

func TestPaymentCreate(t *testing.T) {
	f := newPaymentFixture(t)
	ctx := context.Background()
	inv := f.newInvoice(t)

	p, err := f.payments.Create(ctx, PaymentInput{
		InvoiceID: inv.ID, Amount: 10, PaymentDate: "2026-06-05", Method: "cash", Notes: "deposit",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if p.ID <= 0 {
		t.Fatalf("ID = %d, want > 0", p.ID)
	}
	if p.InvoiceID != inv.ID || p.Amount != 10 || p.PaymentDate != "2026-06-05" {
		t.Fatalf("payment = %+v", p)
	}
	if p.Method != "cash" || p.Notes != "deposit" {
		t.Fatalf("method/notes = %q/%q", p.Method, p.Notes)
	}
	if p.UUID == "" || p.CreatedAt == "" || p.UpdatedAt == "" {
		t.Fatalf("missing uuid/timestamps: %+v", p)
	}

	var n int
	if err := f.conn.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM audit_log WHERE entity_type='payment' AND action='create' AND entity_id=?",
		p.ID,
	).Scan(&n); err != nil {
		t.Fatalf("count audit: %v", err)
	}
	if n != 1 {
		t.Fatalf("audit create rows = %d, want 1", n)
	}
}

func TestPaymentCreateValidation(t *testing.T) {
	f := newPaymentFixture(t)
	ctx := context.Background()
	inv := f.newInvoice(t)

	if _, err := f.payments.Create(ctx, PaymentInput{InvoiceID: 0, Amount: 10, PaymentDate: "2026-06-05"}); err == nil {
		t.Fatal("expected error for missing invoice id")
	}
	if _, err := f.payments.Create(ctx, PaymentInput{InvoiceID: inv.ID, Amount: 0, PaymentDate: "2026-06-05"}); err == nil {
		t.Fatal("expected error for zero amount")
	}
	if _, err := f.payments.Create(ctx, PaymentInput{InvoiceID: inv.ID, Amount: -5, PaymentDate: "2026-06-05"}); err == nil {
		t.Fatal("expected error for negative amount")
	}
}

func TestPaymentListForInvoiceAndTotal(t *testing.T) {
	f := newPaymentFixture(t)
	ctx := context.Background()
	inv := f.newInvoice(t)

	if _, err := f.payments.Create(ctx, PaymentInput{InvoiceID: inv.ID, Amount: 10, PaymentDate: "2026-06-05"}); err != nil {
		t.Fatalf("Create 1: %v", err)
	}
	list, err := f.payments.ListForInvoice(ctx, inv.ID)
	if err != nil {
		t.Fatalf("ListForInvoice: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("list = %d, want 1", len(list))
	}

	if _, err := f.payments.Create(ctx, PaymentInput{InvoiceID: inv.ID, Amount: 5, PaymentDate: "2026-06-06"}); err != nil {
		t.Fatalf("Create 2: %v", err)
	}
	list, err = f.payments.ListForInvoice(ctx, inv.ID)
	if err != nil {
		t.Fatalf("ListForInvoice 2: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("list = %d, want 2", len(list))
	}
	// ordered by payment_date.
	if list[0].PaymentDate != "2026-06-05" || list[1].PaymentDate != "2026-06-06" {
		t.Fatalf("order = %q,%q", list[0].PaymentDate, list[1].PaymentDate)
	}

	tp, err := f.payments.TotalPaid(ctx, inv.ID)
	if err != nil {
		t.Fatalf("TotalPaid: %v", err)
	}
	if tp != 15 {
		t.Fatalf("TotalPaid = %v, want 15", tp)
	}
}

func TestInvoiceGetReflectsPayments(t *testing.T) {
	f := newPaymentFixture(t)
	ctx := context.Background()
	inv := f.newInvoice(t)

	if _, err := f.payments.Create(ctx, PaymentInput{InvoiceID: inv.ID, Amount: 10, PaymentDate: "2026-06-05"}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	got, err := f.invoices.Get(ctx, inv.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.TotalPaid != 10 {
		t.Fatalf("TotalPaid = %v, want 10", got.TotalPaid)
	}
	if got.Balance != 15 {
		t.Fatalf("Balance = %v, want 15", got.Balance)
	}
}

func TestClientStatsTotalPaid(t *testing.T) {
	f := newPaymentFixture(t)
	ctx := context.Background()
	inv1 := f.newInvoice(t)
	inv2 := f.newInvoice(t)

	if _, err := f.payments.Create(ctx, PaymentInput{InvoiceID: inv1.ID, Amount: 10, PaymentDate: "2026-06-05"}); err != nil {
		t.Fatalf("Create 1: %v", err)
	}
	if _, err := f.payments.Create(ctx, PaymentInput{InvoiceID: inv2.ID, Amount: 5, PaymentDate: "2026-06-06"}); err != nil {
		t.Fatalf("Create 2: %v", err)
	}

	stats, err := f.invoices.ClientStats(ctx, f.clientID)
	if err != nil {
		t.Fatalf("ClientStats: %v", err)
	}
	if stats.InvoiceCount != 2 {
		t.Fatalf("InvoiceCount = %d, want 2", stats.InvoiceCount)
	}
	if stats.TotalInvoiced != 50 {
		t.Fatalf("TotalInvoiced = %v, want 50", stats.TotalInvoiced)
	}
	if stats.TotalPaid != 15 {
		t.Fatalf("TotalPaid = %v, want 15", stats.TotalPaid)
	}
}

func TestPaymentDelete(t *testing.T) {
	f := newPaymentFixture(t)
	ctx := context.Background()
	inv := f.newInvoice(t)

	p, err := f.payments.Create(ctx, PaymentInput{InvoiceID: inv.ID, Amount: 10, PaymentDate: "2026-06-05"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	invoiceID, err := f.payments.Delete(ctx, p.ID)
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if invoiceID != inv.ID {
		t.Fatalf("Delete returned invoice id %d, want %d", invoiceID, inv.ID)
	}

	list, err := f.payments.ListForInvoice(ctx, inv.ID)
	if err != nil {
		t.Fatalf("ListForInvoice: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("payments remaining = %d, want 0", len(list))
	}
	tp, err := f.payments.TotalPaid(ctx, inv.ID)
	if err != nil {
		t.Fatalf("TotalPaid: %v", err)
	}
	if tp != 0 {
		t.Fatalf("TotalPaid = %v, want 0", tp)
	}

	// deleting a non-existent payment surfaces sql.ErrNoRows.
	if _, err := f.payments.Delete(ctx, 99999); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("Delete missing = %v, want sql.ErrNoRows", err)
	}
}
