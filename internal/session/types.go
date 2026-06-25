package session

import "github.com/dknathalage/tallyo/internal/apperr"

// Session is the domain view of a row in the sessions table — the delivered-support
// unit a provider records for a client. A session's billable quantities live
// on its line_items rows (see ListItems), not on the session itself. Tags is
// stored as JSON TEXT and is never nil. Status moves through the lifecycle
// scheduled→recorded→drafted→sent→paid; InvoiceID is set once the session is
// drafted onto an invoice.
type Session struct {
	ID           string   `json:"id"`
	ClientID     string   `json:"-"`
	ClientUUID   string   `json:"clientId"`
	ServiceDate  string   `json:"serviceDate"`
	Note         string   `json:"note"`
	Tags         []string `json:"tags"`
	Status       string   `json:"status"`
	InvoiceID    *string  `json:"-"`         // internal FK; the public ref is invoiceId (the linked invoice's uuid)
	InvoiceUUID  *string  `json:"invoiceId"` // linked invoice uuid (nil until the session is drafted onto an invoice)
	AuthorUserID *string  `json:"-"`         // internal author user FK; not linked from the SPA
	CreatedAt    string   `json:"createdAt"`
	UpdatedAt    string   `json:"updatedAt"`
}

// SessionInput is the writable subset of a session.
type SessionInput struct {
	ClientID    string   `json:"clientId"`
	ServiceDate string   `json:"serviceDate"`
	Note        string   `json:"note"`
	Tags        []string `json:"tags"`
	Status      string   `json:"status"`
}

// Validate runs the cheap shape checks the service enforces before the
// repository: a resolved client id and a strict YYYY-MM-DD service date. A
// failure is returned as an *apperr.ValidationError (HTTP 422 with per-field
// detail). The handler still resolves the inbound client uuid to ClientID (a
// DB-resolved rule → 400) before the service is called; the line-item validity
// rule lives in the billing.LineValidator on the service.
func (in SessionInput) Validate() error {
	ve := &apperr.ValidationError{}
	if in.ClientID == "" {
		ve.Errors = append(ve.Errors, apperr.FieldError{Line: 0, Field: "clientId", Message: "required"})
	}
	if !validISODate(in.ServiceDate) {
		ve.Errors = append(ve.Errors, apperr.FieldError{Line: 0, Field: "serviceDate", Message: "must be a valid YYYY-MM-DD date"})
	}
	if len(ve.Errors) > 0 {
		return ve
	}
	return nil
}

// UnbilledAgg summarises a client's recorded-but-unbilled sessions: how many there
// are and the service-date span they cover.
type UnbilledAgg struct {
	ClientID string `json:"clientId"`
	Count    int64  `json:"count"`
	From     string `json:"from"`
	To       string `json:"to"`
}
