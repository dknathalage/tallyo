// Package pdf renders invoices and estimates to PDF documents using maroto v2
// (pure-Go, no cgo). Documents are rendered from the point-in-time JSON
// snapshots carried on the domain types, never from live data.
package pdf

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/dknathalage/tallyo/internal/billing"
	"github.com/johnfercher/maroto/v2"
	"github.com/johnfercher/maroto/v2/pkg/components/col"
	"github.com/johnfercher/maroto/v2/pkg/components/text"
	"github.com/johnfercher/maroto/v2/pkg/config"
	"github.com/johnfercher/maroto/v2/pkg/consts/align"
	"github.com/johnfercher/maroto/v2/pkg/consts/fontstyle"
	"github.com/johnfercher/maroto/v2/pkg/consts/pagesize"
	"github.com/johnfercher/maroto/v2/pkg/core"
	"github.com/johnfercher/maroto/v2/pkg/props"
)

// party is a rendered counterparty (business or client) parsed from a snapshot.
type party struct {
	Name     string            `json:"name"`
	Email    string            `json:"email"`
	Phone    string            `json:"phone"`
	Address  string            `json:"address"`
	Metadata map[string]string `json:"metadata"`
}

// parseSnapshot decodes a snapshot JSON string into a party. An empty or
// malformed snapshot yields a zero-value party (rendering simply omits blanks).
func parseSnapshot(s string) party {
	var p party
	if s == "" {
		return p
	}
	if err := json.Unmarshal([]byte(s), &p); err != nil {
		return party{}
	}
	return p
}

// docData is the document-type-agnostic view consumed by render. NDIS amounts
// are in AUD; tax is a precomputed amount (no rate is carried on the header).
type docData struct {
	Title    string
	Number   string
	Date     string
	DueLabel string
	DueValue string
	Status   string
	Business party
	Client   party
	Rows     [][4]string // {desc, qty, unitPrice, lineTotal}
	Subtotal float64
	Tax      float64
	Total    float64
	Notes    string
}

// money formats an AUD amount, e.g. "AUD 27.50".
func money(v float64) string {
	return fmt.Sprintf("AUD %.2f", v)
}

// InvoiceDoc is the minimal flat shape needed to render an invoice PDF. It is
// defined here (not in the invoice package) so the pdf package does not import
// the invoice slice, avoiding a cycle. The invoice handler constructs one from
// its *invoice.Invoice before calling RenderInvoice.
type InvoiceDoc struct {
	Number           string
	IssueDate        string
	DueDate          string
	Status           string
	BusinessSnapshot string
	ClientSnapshot   string
	LineItems        []*billing.LineItem
	Subtotal         float64
	Tax              float64
	Total            float64
	Notes            string
}

// RenderInvoice renders an invoice to PDF bytes from its snapshots.
func RenderInvoice(inv *InvoiceDoc) ([]byte, error) {
	if inv == nil {
		return nil, errors.New("pdf: nil invoice")
	}
	d := docData{
		Title:    "INVOICE",
		Number:   inv.Number,
		Date:     inv.IssueDate,
		DueLabel: "Due",
		DueValue: inv.DueDate,
		Status:   inv.Status,
		Business: parseSnapshot(inv.BusinessSnapshot),
		Client:   parseSnapshot(inv.ClientSnapshot),
		Rows:     lineItemRows(inv.LineItems),
		Subtotal: inv.Subtotal, Tax: inv.Tax,
		Total: inv.Total, Notes: inv.Notes,
	}
	return render(d)
}

// EstimateDoc is the minimal flat shape needed to render an estimate PDF. It is
// defined here (not in the repository package) so the pdf package does not
// import the repository slice, avoiding a cycle.
type EstimateDoc struct {
	Number           string
	IssueDate        string
	ValidUntil       string
	Status           string
	BusinessSnapshot string
	ClientSnapshot   string
	LineItems        []*billing.LineItem
	Subtotal         float64
	Tax              float64
	Total            float64
	Notes            string
}

// RenderEstimate renders an estimate to PDF bytes from its snapshots.
func RenderEstimate(est *EstimateDoc) ([]byte, error) {
	if est == nil {
		return nil, errors.New("pdf: nil estimate")
	}
	d := docData{
		Title:    "ESTIMATE",
		Number:   est.Number,
		Date:     est.IssueDate,
		DueLabel: "Valid Until",
		DueValue: est.ValidUntil,
		Status:   est.Status,
		Business: parseSnapshot(est.BusinessSnapshot),
		Client:   parseSnapshot(est.ClientSnapshot),
		Rows:     lineItemRows(est.LineItems),
		Subtotal: est.Subtotal, Tax: est.Tax,
		Total: est.Total, Notes: est.Notes,
	}
	return render(d)
}

// lineItemRows projects line items into renderable string rows. Invoices and
// estimates share the billing.LineItem domain type.
func lineItemRows(items []*billing.LineItem) [][4]string {
	rows := make([][4]string, 0, len(items))
	for _, it := range items {
		if it == nil {
			continue
		}
		rows = append(rows, [4]string{
			it.Description,
			fmt.Sprintf("%g", it.Quantity),
			money(it.UnitPrice),
			money(it.LineTotal),
		})
	}
	return rows
}

// render builds the maroto document and returns its PDF bytes.
func render(d docData) ([]byte, error) {
	if d.Title == "" {
		return nil, errors.New("pdf: missing document title")
	}
	cfg := config.NewBuilder().WithPageSize(pagesize.A4).Build()
	m := maroto.New(cfg)

	addHeader(m, d)
	addParties(m, d)
	addTable(m, d)
	addTotals(m, d)
	addNotes(m, d)

	doc, err := m.Generate()
	if err != nil {
		return nil, fmt.Errorf("pdf: generate: %w", err)
	}
	b := doc.GetBytes()
	if len(b) == 0 {
		return nil, errors.New("pdf: empty document")
	}
	return b, nil
}

// addHeader renders the business identity block and the document title block.
func addHeader(m core.Maroto, d docData) {
	bold := props.Text{Style: fontstyle.Bold, Size: 16}
	right := props.Text{Align: align.Right, Size: 10}
	m.AddRow(10,
		col.New(7).Add(text.New(d.Business.Name, bold)),
		col.New(5).Add(text.New(d.Title, props.Text{Style: fontstyle.Bold, Size: 16, Align: align.Right})),
	)
	for _, line := range nonEmpty(d.Business.Email, d.Business.Phone, d.Business.Address) {
		m.AddRow(5, col.New(7).Add(text.New(line, props.Text{Size: 9})))
	}
	m.AddRow(6,
		col.New(7).Add(text.New("Status: "+d.Status, props.Text{Size: 9})),
		col.New(5).Add(text.New("No: "+d.Number, right)),
	)
	m.AddRow(5, col.New(12).Add(text.New("Date: "+d.Date+"   "+d.DueLabel+": "+d.DueValue, right)))
}

// addParties renders the "Bill To" client block.
func addParties(m core.Maroto, d docData) {
	m.AddRow(8, col.New(12).Add(text.New("Bill To:", props.Text{Style: fontstyle.Bold, Size: 11, Top: 2})))
	m.AddRow(5, col.New(12).Add(text.New(d.Client.Name, props.Text{Style: fontstyle.Bold, Size: 10})))
	for _, line := range nonEmpty(d.Client.Email, d.Client.Phone, d.Client.Address) {
		m.AddRow(5, col.New(12).Add(text.New(line, props.Text{Size: 9})))
	}
}

// addTable renders the line-item header row followed by one row per item.
func addTable(m core.Maroto, d docData) {
	hdr := props.Text{Style: fontstyle.Bold, Size: 9}
	hdrR := props.Text{Style: fontstyle.Bold, Size: 9, Align: align.Right}
	m.AddRow(7,
		col.New(6).Add(text.New("Description", props.Text{Style: fontstyle.Bold, Size: 9, Top: 2})),
		col.New(2).Add(text.New("Qty", hdr)),
		col.New(2).Add(text.New("Unit Price", hdrR)),
		col.New(2).Add(text.New("Total", hdrR)),
	)
	cellR := props.Text{Size: 9, Align: align.Right}
	for _, r := range d.Rows {
		m.AddRow(6,
			col.New(6).Add(text.New(r[0], props.Text{Size: 9})),
			col.New(2).Add(text.New(r[1], props.Text{Size: 9})),
			col.New(2).Add(text.New(r[2], cellR)),
			col.New(2).Add(text.New(r[3], cellR)),
		)
	}
}

// addTotals renders the subtotal, tax and total summary lines (right-aligned).
func addTotals(m core.Maroto, d docData) {
	right := props.Text{Size: 9, Align: align.Right}
	boldR := props.Text{Size: 11, Align: align.Right, Style: fontstyle.Bold}
	m.AddRow(6,
		col.New(8),
		col.New(2).Add(text.New("Subtotal", right)),
		col.New(2).Add(text.New(money(d.Subtotal), right)),
	)
	m.AddRow(6,
		col.New(8),
		col.New(2).Add(text.New("GST", right)),
		col.New(2).Add(text.New(money(d.Tax), right)),
	)
	m.AddRow(7,
		col.New(8),
		col.New(2).Add(text.New("Total", boldR)),
		col.New(2).Add(text.New(money(d.Total), boldR)),
	)
}

// addNotes renders a trailing notes block when present.
func addNotes(m core.Maroto, d docData) {
	if d.Notes == "" {
		return
	}
	m.AddRow(8, col.New(12).Add(text.New("Notes: "+d.Notes, props.Text{Size: 9, Top: 3})))
}

// nonEmpty returns the subset of args that are not the empty string.
func nonEmpty(vals ...string) []string {
	out := make([]string, 0, len(vals))
	for _, v := range vals {
		if v != "" {
			out = append(out, v)
		}
	}
	return out
}
