package agent

import (
	"context"

	"github.com/dknathalage/tallyo/internal/billing"
	"github.com/dknathalage/tallyo/internal/catalog"
	"github.com/dknathalage/tallyo/internal/shift"
)

// ShiftLister is satisfied by *shift.Service; it covers the tool that lists a
// participant's shifts in an optional date range (used by the import-shifts
// dedup path).
type ShiftLister interface {
	ListParticipant(ctx context.Context, participantID int64, from, to string) ([]*shift.Shift, error)
}

// ShiftCreator is satisfied by *shift.Service; the import-shifts Smart creates
// recorded shifts from extracted drafts.
type ShiftCreator interface {
	Create(ctx context.Context, in shift.ShiftInput) (*shift.Shift, error)
}

// ShiftReader is satisfied by *shift.Service; the divide Smart loads ONE shift's
// note + service date to divide it into line items.
type ShiftReader interface {
	Get(ctx context.Context, id int64) (*shift.Shift, error)
}

// ShiftItemWriter is satisfied by *shift.Service; the divide Smart persists each
// proposed line on the shift (AddItem prices coded lines from the catalogue) and
// clears the prior unbilled items so a re-divide is idempotent.
type ShiftItemWriter interface {
	AddItem(ctx context.Context, shiftID int64, in billing.LineItemInput) (*billing.LineItem, error)
	ClearUnbilledItems(ctx context.Context, shiftID int64) error
}

// ShiftWorker composes ShiftLister, ShiftCreator, ShiftReader and ShiftItemWriter;
// it is the interface used by the import-shifts Smart (list/create) and the
// divide Smart (read/write items).
type ShiftWorker interface {
	ShiftLister
	ShiftCreator
	ShiftReader
	ShiftItemWriter
}

// CatalogueSearcher is satisfied by *catalog.Service; it covers the
// search_catalogue tool and the inline catalogue enrichment of shift rows.
type CatalogueSearcher interface {
	SearchForDate(ctx context.Context, query, serviceDate, zone string, limit int) ([]*catalog.CatalogMatch, error)
}
