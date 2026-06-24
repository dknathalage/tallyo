package smarts

import (
	"context"
	"errors"
	"strings"

	"github.com/dknathalage/tallyo/internal/billing"
	"github.com/dknathalage/tallyo/internal/client"
	"github.com/dknathalage/tallyo/internal/invoice"
	"github.com/dknathalage/tallyo/internal/pricelist"
	"github.com/dknathalage/tallyo/internal/session"
)

// Typed errors a Smart can return; the handler maps them to HTTP status. Model
// failures map to 502; these data/precondition errors map to 422.
var (
	// ErrNoData — the Smart had nothing to work with (e.g. no unbilled sessions).
	ErrNoData = errors.New("nothing to work with")
	// ErrNoPriceList — no price-list version is in effect for the service date.
	ErrNoPriceList = errors.New("no price list in effect for that date")
	// ErrNotFound — a referenced entity (client/invoice) does not exist.
	ErrNotFound = errors.New("not found")
)

// Consumer interfaces — declared here, satisfied by the concrete domain services,
// wired in internal/app. smarts is the designated consumer slice (successor to
// the agent slice) and is the one place allowed to depend on other slices; no
// domain slice imports smarts.

// SessionReader loads a client's unbilled (recorded) sessions for the
// draft-invoice gather. Satisfied by *session.Service.
type SessionReader interface {
	ListUnbilledForClient(ctx context.Context, tenantID, clientID string) ([]*session.Session, error)
}

// CatalogueSearcher is the tenant-scoped, all-fields grounding capability plus
// version resolution. Satisfied by *pricelist.ItemsRepo.
type CatalogueSearcher interface {
	ResolveVersionForDate(ctx context.Context, tenantID string, serviceDate string) (*pricelist.PriceListVersion, error)
	SearchItems(ctx context.Context, tenantID, versionID string, query string) ([]*pricelist.Item, error)
	GetItemByCode(ctx context.Context, tenantID, versionID string, code string) (*pricelist.Item, error)
}

// InvoiceDrafter creates an invoice through the trusted, self-validating service
// path (it recomputes tax and prices internally). Satisfied by *invoice.Service.
type InvoiceDrafter interface {
	Create(ctx context.Context, in invoice.InvoiceInput, items []billing.LineItemInput) (*invoice.Invoice, error)
}

// InvoiceReader loads one invoice for the follow-up Smart. Satisfied by
// *invoice.Service.
type InvoiceReader interface {
	GetByUUID(ctx context.Context, invoiceUUID string) (*invoice.Invoice, error)
}

// ClientReader loads one client by uuid. Satisfied by *client.Service.
type ClientReader interface {
	Get(ctx context.Context, uuid string) (*client.Client, error)
}

// Service is the Smarts capability surface. Each Smart is gather → propose →
// apply; there is no persistent state.
type Service struct {
	llm      Proposer
	sessions SessionReader
	cat      CatalogueSearcher
	invoices InvoiceDrafter
	invRead  InvoiceReader
	clients  ClientReader
}

// NewService constructs the Smarts service. A nil dependency is a programmer
// error (NASA rule 5).
func NewService(llm Proposer, sessions SessionReader, cat CatalogueSearcher, invoices InvoiceDrafter, invRead InvoiceReader, clients ClientReader) *Service {
	if llm == nil || sessions == nil || cat == nil || invoices == nil || invRead == nil || clients == nil {
		panic("smarts.NewService: nil dependency")
	}
	return &Service{llm: llm, sessions: sessions, cat: cat, invoices: invoices, invRead: invRead, clients: clients}
}

// wrapUntrusted fences arbitrary record text so the model treats it as data, not
// instructions (prompt-injection hygiene for free-text notes).
func wrapUntrusted(label, body string) string {
	sanitised := strings.ReplaceAll(body, "</untrusted-content", "&lt;/untrusted-content")
	return "<untrusted-content source=\"" + label + "\">\n" + sanitised + "\n</untrusted-content>"
}
