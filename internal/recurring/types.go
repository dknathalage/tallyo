package recurring

import "github.com/dknathalage/tallyo/internal/billing"

// RecurringTemplate is the domain view of a recurring invoice template. Line
// items are stored as a JSON string column and unmarshalled into the slice. The
// public identifier is the uuid (json "id"); the internal row id stays out of the
// JSON. The client/payer FKs are exposed as the related entities' uuids (json
// "clientId"/"payerId"), resolved via LEFT JOIN, never the internal row ids.
// clientID is the internal client row id (uuid), retained for generation
// snapshots (not serialized).
type RecurringTemplate struct {
	ID         string           `json:"id"`
	clientID   *string          // internal client FK, used for generation snapshots
	ClientUUID *string          `json:"clientId"`
	ClientName string           `json:"clientName"`
	PayerID    *string          `json:"-"`
	PayerUUID  *string          `json:"payerId"`
	Name       string           `json:"name"`
	Frequency  string           `json:"frequency"`
	NextDue    string           `json:"nextDue"`
	LineItems  []*RecurringLine `json:"lineItems"`
	TaxRate    float64          `json:"taxRate"`
	Notes      string           `json:"notes"`
	IsActive   bool             `json:"isActive"`
	CreatedAt  string           `json:"createdAt"`
	UpdatedAt  string           `json:"updatedAt"`
}

// RecurringLine is one line in a template's stored line_items JSON.
type RecurringLine struct {
	ItemID       *string `json:"itemId"`       // tenant price-list item uuid
	CustomItemID *string `json:"customItemId"` // tenant custom_items.uuid
	Code         string  `json:"code"`
	Description  string  `json:"description"`
	Unit         string  `json:"unit"`
	Quantity     float64 `json:"quantity"`
	UnitPrice    float64 `json:"unitPrice"`
	Taxable      bool    `json:"taxable"`
	SortOrder    int64   `json:"sortOrder"`
}

// RecurringInput is the writable subset of a recurring template. Client and
// payer arrive as uuid strings (the public identifiers) and are resolved
// to the related row ids (uuid) before insert/update. ClientUUID is required; an empty/nil
// PayerUUID maps to a NULL FK.
type RecurringInput struct {
	ClientUUID *string         `json:"clientId"`
	PayerUUID  *string         `json:"payerId"`
	Name       string          `json:"name"`
	Frequency  string          `json:"frequency"`
	NextDue    string          `json:"nextDue"`
	LineItems  []RecurringLine `json:"lineItems"`
	TaxRate    float64         `json:"taxRate"`
	Notes      string          `json:"notes"`
	IsActive   bool            `json:"isActive"`
}

// GeneratedInvoice identifies an invoice produced by the due sweep.
type GeneratedInvoice struct {
	TemplateID    string `json:"templateId"`
	InvoiceID     string `json:"invoiceId"`
	InvoiceNumber string `json:"invoiceNumber"`
}

// GeneratedInvoiceDoc is the full domain view of an invoice generated from a
// template, returned by GenerateOne. Its JSON shape is identical to the invoice
// slice's Invoice (the SPA lands the user on this document), but recurring owns
// it and assembles it from the central db/gen rows + shared billing mappers, so
// recurring never imports the invoice slice (the "no slice imports another"
// rule). A freshly generated invoice has no payments, so TotalPaid is 0 and
// Balance equals Total.
type GeneratedInvoiceDoc struct {
	ID               string              `json:"id"`
	Number           string              `json:"number"`
	ClientUUID       string              `json:"clientId"`
	ClientName       string              `json:"clientName"`
	PayerUUID        *string             `json:"payerId"`
	Status           string              `json:"status"`
	IssueDate        string              `json:"issueDate"`
	DueDate          string              `json:"dueDate"`
	Subtotal         float64             `json:"subtotal"`
	Tax              float64             `json:"tax"`
	Total            float64             `json:"total"`
	Notes            string              `json:"notes"`
	BusinessSnapshot string              `json:"businessSnapshot"`
	ClientSnapshot   string              `json:"clientSnapshot"`
	PayerSnapshot    string              `json:"payerSnapshot"`
	CreatedAt        string              `json:"createdAt"`
	UpdatedAt        string              `json:"updatedAt"`
	TotalPaid        float64             `json:"totalPaid"`
	Balance          float64             `json:"balance"`
	LineItems        []*billing.LineItem `json:"lineItems"`
}
