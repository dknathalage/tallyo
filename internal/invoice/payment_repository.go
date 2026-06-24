package invoice

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/dknathalage/tallyo/internal/db"
	"time"

	"github.com/dknathalage/tallyo/internal/audit"
	"github.com/dknathalage/tallyo/internal/db/gen"
	"github.com/dknathalage/tallyo/internal/ids"
)

// Payment is the domain view of a row in the payments table. Nullable columns
// are unwrapped to plain strings.
type Payment struct {
	ID        string  `json:"id"` // public identifier (payment uuid)
	InvoiceID string  `json:"-"`  // internal FK (the payment hangs under its invoice's uuid path)
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
	InvoiceID string  `json:"invoiceId"`
	Amount    float64 `json:"amount"`
	PaidAt    string  `json:"paidAt"`
	Method    string  `json:"method"`
	Reference string  `json:"reference"`
	Notes     string  `json:"notes"`
}

// PaymentsRepo reads and writes the payments table (tenant-scoped) with audited
// mutations.
type PaymentsRepo struct {
	db db.Executor
}

// NewPayments constructs a repository. A nil db is a programmer error.
func NewPayments(db db.Executor) *PaymentsRepo {
	if db == nil {
		panic("invoice: NewPayments requires a non-nil *sql.DB")
	}
	return &PaymentsRepo{db: db}
}

// Create inserts a payment and writes one audit row, atomically. The invoice id
// is required and the amount must be positive.
func (r *PaymentsRepo) Create(ctx context.Context, tenantID string, in PaymentInput) (*Payment, error) {
	if tenantID == "" {
		return nil, errors.New("create payment: tenant id required")
	}
	if in.InvoiceID == "" {
		return nil, errors.New("create payment: invoice id required")
	}
	if in.Amount <= 0 {
		return nil, errors.New("create payment: amount must be positive")
	}

	var created gen.Payment
	err := audit.WithTx(ctx, r.db, audit.Entry{Action: ""}, func(tx *sql.Tx) error {
		now := time.Now().UTC().Format(time.RFC3339)
		p, e := gen.New(tx).CreatePayment(ctx, gen.CreatePaymentParams{
			ID:        ids.New(),
			TenantID:  tenantID,
			InvoiceID: in.InvoiceID,
			Amount:    in.Amount,
			PaidAt:    in.PaidAt,
			Method:    db.NzMaybe(in.Method),
			Reference: db.NzMaybe(in.Reference),
			Notes:     db.NzMaybe(in.Notes),
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
func (r *PaymentsRepo) ListForInvoice(ctx context.Context, tenantID, invoiceID string) ([]*Payment, error) {
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
func (r *PaymentsRepo) TotalPaid(ctx context.Context, tenantID, invoiceID string) (float64, error) {
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
func (r *PaymentsRepo) Delete(ctx context.Context, tenantID, id string) (paymentUUID string, invoiceID string, err error) {
	err = audit.WithTx(ctx, r.db, audit.Entry{Action: ""}, func(tx *sql.Tx) error {
		q := gen.New(tx)
		p, e := q.GetPayment(ctx, gen.GetPaymentParams{TenantID: tenantID, ID: id})
		if e != nil {
			return e // sql.ErrNoRows surfaces unwrapped for errors.Is
		}
		if e := q.DeletePayment(ctx, gen.DeletePaymentParams{TenantID: tenantID, ID: id}); e != nil {
			return fmt.Errorf("delete: %w", e)
		}
		paymentUUID = p.ID
		invoiceID = p.InvoiceID
		return audit.Log(ctx, tx, audit.Entry{
			EntityType: "payment",
			EntityID:   id,
			Action:     "delete",
			Changes:    audit.Changes(map[string]any{"invoiceId": p.InvoiceID}),
		})
	})
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", sql.ErrNoRows
	}
	if err != nil {
		return "", "", fmt.Errorf("delete payment: %w", err)
	}
	return paymentUUID, invoiceID, nil
}

// ResolveInvoiceID translates an invoice uuid into its int PK, scoped to the
// tenant. Returns (0, nil) when no invoice matches (caller 404s).
func (r *PaymentsRepo) ResolveInvoiceID(ctx context.Context, tenantID string, invoiceUUID string) (string, error) {
	id, err := gen.New(r.db).GetInvoiceIDByUUID(ctx, gen.GetInvoiceIDByUUIDParams{TenantID: tenantID, ID: invoiceUUID})
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("resolve invoice uuid: %w", err)
	}
	return id, nil
}

// InvoiceUUID returns the public uuid of an invoice by its int PK (tenant-scoped),
// or "" when no invoice matches. Used to broadcast the invoice-update SSE event
// after a payment mutation without leaking the int PK.
func (r *PaymentsRepo) InvoiceUUID(ctx context.Context, tenantID, invoiceID string) (string, error) {
	row, err := gen.New(r.db).GetInvoiceByID(ctx, gen.GetInvoiceByIDParams{TenantID: tenantID, ID: invoiceID})
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("resolve invoice uuid: %w", err)
	}
	return row.ID, nil
}

// DeleteByUUID removes a payment addressed by its uuid, scoped to the owning
// invoice's int id (so a payment uuid from another invoice 404s), writing one
// audit row. Returns the invoice id so the caller can broadcast an invoice
// update. A missing payment surfaces sql.ErrNoRows so the caller can 404.
func (r *PaymentsRepo) DeleteByUUID(ctx context.Context, tenantID, invoiceID string, paymentUUID string) (string, error) {
	var deletedInvoiceID string
	err := audit.WithTx(ctx, r.db, audit.Entry{Action: ""}, func(tx *sql.Tx) error {
		q := gen.New(tx)
		p, e := q.GetPaymentByUUID(ctx, gen.GetPaymentByUUIDParams{TenantID: tenantID, InvoiceID: invoiceID, ID: paymentUUID})
		if e != nil {
			return e // sql.ErrNoRows surfaces unwrapped for errors.Is
		}
		if e := q.DeletePaymentByUUID(ctx, gen.DeletePaymentByUUIDParams{TenantID: tenantID, InvoiceID: invoiceID, ID: paymentUUID}); e != nil {
			return fmt.Errorf("delete: %w", e)
		}
		deletedInvoiceID = p.InvoiceID
		return audit.Log(ctx, tx, audit.Entry{
			EntityType: "payment",
			EntityID:   p.ID,
			Action:     "delete",
			Changes:    audit.Changes(map[string]any{"invoiceId": p.InvoiceID}),
		})
	})
	if errors.Is(err, sql.ErrNoRows) {
		return "", sql.ErrNoRows
	}
	if err != nil {
		return "", fmt.Errorf("delete payment: %w", err)
	}
	return deletedInvoiceID, nil
}

// toPayment maps a generated row to the domain Payment.
func toPayment(row gen.Payment) *Payment {
	return &Payment{
		ID:        row.ID,
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
