// Package export renders catalog, invoice, and estimate collections to CSV
// (encoding/csv) and Excel (.xlsx via excelize). All renderers are pure-Go and
// cgo-free so the desktop binary stays statically linkable.
package export

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"strconv"

	"github.com/dknathalage/tallyo/internal/repository"
	"github.com/xuri/excelize/v2"
)

// money formats a monetary amount with two decimal places.
func money(v float64) string {
	return strconv.FormatFloat(v, 'f', 2, 64)
}

// CatalogCSV renders catalog items to CSV with a fixed header. A nil slice
// yields a header-only document.
func CatalogCSV(items []*repository.CatalogItem) ([]byte, error) {
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	if err := w.Write([]string{"name", "sku", "rate", "unit", "category"}); err != nil {
		return nil, fmt.Errorf("write header: %w", err)
	}
	for _, it := range items {
		if it == nil {
			continue
		}
		rec := []string{it.Name, it.Sku, money(it.Rate), it.Unit, it.Category}
		if err := w.Write(rec); err != nil {
			return nil, fmt.Errorf("write row: %w", err)
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return nil, fmt.Errorf("flush: %w", err)
	}
	return buf.Bytes(), nil
}

// InvoicesCSV renders invoices to CSV with a fixed header.
func InvoicesCSV(invoices []*repository.Invoice) ([]byte, error) {
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	header := []string{"invoiceNumber", "clientName", "date", "dueDate", "status", "subtotal", "taxAmount", "total", "currency"}
	if err := w.Write(header); err != nil {
		return nil, fmt.Errorf("write header: %w", err)
	}
	for _, inv := range invoices {
		if inv == nil {
			continue
		}
		rec := []string{
			inv.InvoiceNumber, inv.ClientName, inv.Date, inv.DueDate, inv.Status,
			money(inv.Subtotal), money(inv.TaxAmount), money(inv.Total), inv.CurrencyCode,
		}
		if err := w.Write(rec); err != nil {
			return nil, fmt.Errorf("write row: %w", err)
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return nil, fmt.Errorf("flush: %w", err)
	}
	return buf.Bytes(), nil
}

// EstimatesCSV renders estimates to CSV with a fixed header.
func EstimatesCSV(estimates []*repository.Estimate) ([]byte, error) {
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	header := []string{"estimateNumber", "clientName", "date", "validUntil", "status", "subtotal", "taxAmount", "total", "currency"}
	if err := w.Write(header); err != nil {
		return nil, fmt.Errorf("write header: %w", err)
	}
	for _, est := range estimates {
		if est == nil {
			continue
		}
		rec := []string{
			est.EstimateNumber, est.ClientName, est.Date, est.ValidUntil, est.Status,
			money(est.Subtotal), money(est.TaxAmount), money(est.Total), est.CurrencyCode,
		}
		if err := w.Write(rec); err != nil {
			return nil, fmt.Errorf("write row: %w", err)
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return nil, fmt.Errorf("flush: %w", err)
	}
	return buf.Bytes(), nil
}

// CatalogXLSX renders catalog items to an .xlsx workbook. The default sheet
// holds a header row followed by one row per item.
func CatalogXLSX(items []*repository.CatalogItem) ([]byte, error) {
	f := excelize.NewFile()
	defer func() { _ = f.Close() }()

	const sheet = "Sheet1"
	header := []interface{}{"name", "sku", "rate", "unit", "category"}
	if err := f.SetSheetRow(sheet, "A1", &header); err != nil {
		return nil, fmt.Errorf("write header: %w", err)
	}

	// Rows start at 2 (1-based) since row 1 is the header. The loop is bounded
	// by len(items).
	row := 2
	for _, it := range items {
		if it == nil {
			continue
		}
		rec := []interface{}{it.Name, it.Sku, it.Rate, it.Unit, it.Category}
		cell := fmt.Sprintf("A%d", row)
		if err := f.SetSheetRow(sheet, cell, &rec); err != nil {
			return nil, fmt.Errorf("write row %d: %w", row, err)
		}
		row++
	}

	buf, err := f.WriteToBuffer()
	if err != nil {
		return nil, fmt.Errorf("write buffer: %w", err)
	}
	return buf.Bytes(), nil
}
