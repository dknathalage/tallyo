package agent

import (
	"context"

	"github.com/dknathalage/tallyo/internal/billing"
	"github.com/dknathalage/tallyo/internal/catalog"
	"github.com/dknathalage/tallyo/internal/invoice"
	"github.com/dknathalage/tallyo/internal/shift"
)

// InvoiceCreator is satisfied by *invoice.Service; it covers the write tool
// that creates an invoice with catalogue-authoritative pricing.
type InvoiceCreator interface {
	CreateWithCatalogPricing(ctx context.Context, in invoice.InvoiceInput, items []billing.LineItemInput) (*invoice.Invoice, error)
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

// ShiftCreator is satisfied by *shift.Service; the import-shifts Smart creates
// recorded shifts from extracted drafts.
type ShiftCreator interface {
	Create(ctx context.Context, in shift.ShiftInput) (*shift.Shift, error)
}

// ShiftWorker composes ShiftLister, ShiftDrafter and ShiftCreator; it is the
// interface used by the create_invoice tool on the shifts lifecycle path, the
// list_participant_shifts tool, and the import-shifts Smart.
type ShiftWorker interface {
	ShiftLister
	ShiftDrafter
	ShiftCreator
}

// CatalogueSearcher is satisfied by *catalog.Service; it covers the
// search_catalogue tool and the inline catalogue enrichment of shift rows.
type CatalogueSearcher interface {
	SearchForDate(ctx context.Context, query, serviceDate, zone string, limit int) ([]*catalog.CatalogMatch, error)
}
