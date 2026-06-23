package agent

import (
	"context"

	"github.com/dknathalage/tallyo/internal/billing"
	"github.com/dknathalage/tallyo/internal/pricelist"
	"github.com/dknathalage/tallyo/internal/session"
)

// SessionLister is satisfied by *session.Service; it covers the tool that lists
// a client's sessions in an optional date range (used by the import-shifts
// dedup path).
type SessionLister interface {
	ListClient(ctx context.Context, clientID int64, from, to string) ([]*session.Session, error)
}

// SessionCreator is satisfied by *session.Service; the import-shifts Smart
// creates recorded sessions from extracted drafts.
type SessionCreator interface {
	Create(ctx context.Context, in session.SessionInput) (*session.Session, error)
}

// SessionReader is satisfied by *session.Service; the divide Smart loads ONE
// session's note + service date to divide it into line items.
type SessionReader interface {
	Get(ctx context.Context, id int64) (*session.Session, error)
}

// SessionItemWriter is satisfied by *session.Service; the divide Smart persists
// each proposed line on the session (AddItem prices coded lines from the
// price list) and clears the prior unbilled items so a re-divide is idempotent.
type SessionItemWriter interface {
	AddItem(ctx context.Context, sessionID int64, in billing.LineItemInput) (*billing.LineItem, error)
	ClearUnbilledItems(ctx context.Context, sessionID int64) error
}

// SessionWorker composes SessionLister, SessionCreator, SessionReader and
// SessionItemWriter; it is the interface used by the import-shifts Smart
// (list/create) and the divide Smart (read/write items).
type SessionWorker interface {
	SessionLister
	SessionCreator
	SessionReader
	SessionItemWriter
}

// CatalogueSearcher is satisfied by *pricelist.Service; it covers the
// search tool and the inline price-list enrichment of session rows.
type CatalogueSearcher interface {
	SearchForDate(ctx context.Context, query, serviceDate, zone string, limit int) ([]*pricelist.Match, error)
}
