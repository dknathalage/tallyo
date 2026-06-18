package invoice

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/dknathalage/tallyo/internal/audit"
	"github.com/dknathalage/tallyo/internal/db/gen"
	"github.com/google/uuid"
)

// Payment is the domain view of a row in the payments table. Nullable columns
// are unwrapped to plain strings.
type Payment struct {
	ID        int64   `json:"id"`
	UUID      string  `json:"uuid"`
	InvoiceID int64   `json:"invoiceId"`
	Amount    float64 `json:"amount"`
	PaidAt    string  `json:"paidAt"`
	Method    string  `json:"method"`
	Reference string  `json:"reference"`
	Notes     string  `json:"notes"`
	CreatedAt string  `json:"createdAt"`
	UpdatedAt string  `json:"updatedAt"`
}

// PaymentInput is the writable subset of a payment.
type PaymentInput struct {
	InvoiceID int64   `json:"invoiceId"`
	Amount    float64 `json:"amount"`
	PaidAt    string  `json:"paidAt"`
	Method    string  `json:"method"`
	Reference string  `json:"reference"`
	Notes     string  `json:"notes"`
}

// PaymentsRepo reads and writes the payments table (tenant-scoped) with audited
// mutations.
type PaymentsRepo struct {
	db *sql.DB
}

// NewPayments constructs a repository. A nil db is a programmer error.
func NewPayments(db *sql.DB) *PaymentsRepo {
	if db == nil {
		panic("invoice: NewPayments requires a non-nil *sql.DB")
	}
	return &PaymentsRepo{db: db}
}

// Create inserts a payment and writes one audit row, atomically. The invoice id
// is required and the amount must be positive.
func (r *PaymentsRepo) Create(ctx context.Context, tenantID int64, in PaymentInput) (*Payment, error) {
	if tenantID == 0 {
		return nil, errors.New("create payment: tenant id required")
	}
	if in.InvoiceID == 0 {
		return nil, errors.New("create payment: invoice id required")
	}
	if in.Amount <= 0 {
		return nil, errors.New("create payment: amount must be positive")
	}

	var created gen.Payment
	err := audit.WithTx(ctx, r.db, audit.Entry{Action: ""}, func(tx *sql.Tx) error {
		now := time.Now().UTC().Format(time.RFC3339)
		p, e := gen.New(tx).CreatePayment(ctx, gen.CreatePaymentParams{
			Uuid:      uuid.NewString(),
			TenantID:  tenantID,
			InvoiceID: in.InvoiceID,
			Amount:    in.Amount,
			PaidAt:    in.PaidAt,
			Method:    nzMaybe(in.Method),
			Reference: nzMaybe(in.Reference),
			Notes:     nzMaybe(in.Notes),
			CreatedAt: now,
			UpdatedAt: now,
		})
		if e != nil {
			return fmt.Errorf("insert: %w", e)
		}
		created = p
		return audit.Log(ctx, tx, audit.Entry{
			EntityType: "payment",
			EntityID:   p.ID,
			Action:     "create",
			Changes:    audit.Changes(map[string]any{"invoiceId": in.InvoiceID, "amount": in.Amount}),
		})
	})
	if err != nil {
		return nil, fmt.Errorf("create payment: %w", err)
	}
	return toPayment(created), nil
}

// ListForInvoice returns one invoice's payments ordered by paid date.
func (r *PaymentsRepo) ListForInvoice(ctx context.Context, tenantID, invoiceID int64) ([]*Payment, error) {
	rows, err := gen.New(r.db).ListInvoicePayments(ctx, gen.ListInvoicePaymentsParams{
		TenantID:  tenantID,
		InvoiceID: invoiceID,
	})
	if err != nil {
		return nil, fmt.Errorf("list invoice payments: %w", err)
	}
	out := make([]*Payment, 0, len(rows))
	for i := range rows { // bounded by len(rows)
		out = append(out, toPayment(rows[i]))
	}
	return out, nil
}

// TotalPaid returns the summed amount of an invoice's payments.
func (r *PaymentsRepo) TotalPaid(ctx context.Context, tenantID, invoiceID int64) (float64, error) {
	total, err := gen.New(r.db).InvoiceTotalPaid(ctx, gen.InvoiceTotalPaidParams{
		TenantID:  tenantID,
		InvoiceID: invoiceID,
	})
	if err != nil {
		return 0, fmt.Errorf("invoice total paid: %w", err)
	}
	return total, nil
}

// Delete removes a payment and writes one audit row, atomically. It returns the
// deleted payment's invoice id so the caller can broadcast an invoice update.
// A missing payment surfaces sql.ErrNoRows so the caller can 404.
func (r *PaymentsRepo) Delete(ctx context.Context, tenantID, id int64) (int64, error) {
	var invoiceID int64
	err := audit.WithTx(ctx, r.db, audit.Entry{Action: ""}, func(tx *sql.Tx) error {
		q := gen.New(tx)
		p, e := q.GetPayment(ctx, gen.GetPaymentParams{TenantID: tenantID, ID: id})
		if e != nil {
			return e // sql.ErrNoRows surfaces unwrapped for errors.Is
		}
		if e := q.DeletePayment(ctx, gen.DeletePaymentParams{TenantID: tenantID, ID: id}); e != nil {
			return fmt.Errorf("delete: %w", e)
		}
		invoiceID = p.InvoiceID
		return audit.Log(ctx, tx, audit.Entry{
			EntityType: "payment",
			EntityID:   id,
			Action:     "delete",
			Changes:    audit.Changes(map[string]any{"invoiceId": p.InvoiceID}),
		})
	})
	if errors.Is(err, sql.ErrNoRows) {
		return 0, sql.ErrNoRows
	}
	if err != nil {
		return 0, fmt.Errorf("delete payment: %w", err)
	}
	return invoiceID, nil
}

// toPayment maps a generated row to the domain Payment.
func toPayment(row gen.Payment) *Payment {
	return &Payment{
		ID:        row.ID,
		UUID:      row.Uuid,
		InvoiceID: row.InvoiceID,
		Amount:    row.Amount,
		PaidAt:    row.PaidAt,
		Method:    row.Method.String,
		Reference: row.Reference.String,
		Notes:     row.Notes.String,
		CreatedAt: row.CreatedAt,
		UpdatedAt: row.UpdatedAt,
	}
}
