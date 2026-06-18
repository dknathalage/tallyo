package agent

import (
	"context"

	"github.com/dknathalage/tallyo/internal/billing"
	"github.com/dknathalage/tallyo/internal/catalog"
	"github.com/dknathalage/tallyo/internal/invoice"
	"github.com/dknathalage/tallyo/internal/shift"
)

// InvoiceLister is satisfied by *invoice.Service; it covers the read-only tool
// that lists invoices optionally filtered by status.
type InvoiceLister interface {
	List(ctx context.Context) ([]*invoice.Invoice, error)
	ListByStatus(ctx context.Context, status string) ([]*invoice.Invoice, error)
}

// InvoiceCreator is satisfied by *invoice.Service; it covers the write tool
// that creates an invoice with catalogue-authoritative pricing.
type InvoiceCreator interface {
	CreateWithCatalogPricing(ctx context.Context, in invoice.InvoiceInput, items []billing.LineItemInput) (*invoice.Invoice, error)
}

// InvoiceAccessor is satisfied by *invoice.Service; it covers the restore
// function that reads and deletes an invoice for checkpoint revert.
type InvoiceAccessor interface {
	Get(ctx context.Context, id int64) (*invoice.Invoice, error)
	Delete(ctx context.Context, id int64) error
}

// ShiftLister is satisfied by *shift.Service; it covers the tool that lists a
// participant's recorded shifts in an optional date range.
type ShiftLister interface {
	ListParticipant(ctx context.Context, participantID int64, from, to string) ([]*shift.Shift, error)
}

// ShiftDrafter is satisfied by *shift.Service; it covers the post-create step
// that links covered shifts to the new invoice (status → drafted).
type ShiftDrafter interface {
	MarkDrafted(ctx context.Context, invoiceID int64, shiftIDs []int64) error
}

// ShiftWorker composes ShiftLister and ShiftDrafter; it is the interface used
// by the create_invoice tool on the shifts lifecycle path and the
// list_participant_shifts tool.
type ShiftWorker interface {
	ShiftLister
	ShiftDrafter
}

// CatalogueSearcher is satisfied by *catalog.Service; it covers the
// search_catalogue tool and the inline catalogue enrichment of shift rows.
type CatalogueSearcher interface {
	SearchForDate(ctx context.Context, query, serviceDate, zone string, limit int) ([]*catalog.CatalogMatch, error)
}
