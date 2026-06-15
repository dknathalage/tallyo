// Package importer provides pure-Go (encoding/csv + excelize, both cgo-free)
// parsing primitives for tabular catalogue files (CSV/XLSX). It reads a file
// into headers plus header→cell maps; the NDIS catalogue ingest builds its
// fixed-format parser on top of these primitives.
package importer

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/xuri/excelize/v2"
)

// ParseRows reads a CSV or XLSX file into headers plus a slice of
// header→cell maps. headerRow is 1-based; the row at index headerRow-1 is the
// header and subsequent rows become data. A headerRow < 1 defaults to 1.
func ParseRows(data []byte, fileType, sheetName string, headerRow int) ([]string, []map[string]string, error) {
	if len(data) == 0 {
		return nil, nil, fmt.Errorf("importer.ParseRows: empty file")
	}
	if headerRow < 1 {
		headerRow = 1
	}
	if strings.EqualFold(fileType, "xlsx") {
		return parseXLSX(data, sheetName, headerRow)
	}
	return parseCSV(data, headerRow)
}

// parseCSV parses CSV bytes with a variable field count per record.
func parseCSV(data []byte, headerRow int) ([]string, []map[string]string, error) {
	reader := csv.NewReader(bytes.NewReader(data))
	reader.FieldsPerRecord = -1
	var records [][]string
	for { // bounded by the file's line count
		rec, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, nil, fmt.Errorf("importer.parseCSV: %w", err)
		}
		records = append(records, rec)
	}
	return rowsFromRecords(records, headerRow)
}

// parseXLSX parses XLSX bytes, choosing sheetName or the first sheet.
func parseXLSX(data []byte, sheetName string, headerRow int) ([]string, []map[string]string, error) {
	f, err := excelize.OpenReader(bytes.NewReader(data))
	if err != nil {
		return nil, nil, fmt.Errorf("importer.parseXLSX: open: %w", err)
	}
	defer func() { _ = f.Close() }()

	sheet := sheetName
	if sheet == "" {
		sheets := f.GetSheetList()
		if len(sheets) == 0 {
			return nil, nil, fmt.Errorf("importer.parseXLSX: workbook has no sheets")
		}
		sheet = sheets[0]
	}
	records, err := f.GetRows(sheet)
	if err != nil {
		return nil, nil, fmt.Errorf("importer.parseXLSX: get rows: %w", err)
	}
	return rowsFromRecords(records, headerRow)
}

// rowsFromRecords turns a 2-D record slice into headers plus header→cell maps.
// Data cells are padded/truncated to the header length.
func rowsFromRecords(records [][]string, headerRow int) ([]string, []map[string]string, error) {
	if len(records) < headerRow {
		return nil, nil, fmt.Errorf("importer: file has fewer than %d rows", headerRow)
	}
	headers := records[headerRow-1]
	if len(headers) == 0 {
		return nil, nil, fmt.Errorf("importer: header row is empty")
	}
	out := make([]map[string]string, 0, len(records)-headerRow)
	for i := headerRow; i < len(records); i++ { // bounded by len(records)
		rec := records[i]
		row := make(map[string]string, len(headers))
		for j := range headers { // bounded by len(headers)
			if j < len(rec) {
				row[headers[j]] = rec[j]
			} else {
				row[headers[j]] = ""
			}
		}
		out = append(out, row)
	}
	return headers, out, nil
}

// ParseFloat strips currency/grouping noise and parses a float, returning 0 for
// blank or unparseable input (a lenient parse suited to messy spreadsheet cells).
func ParseFloat(s string) float64 {
	cleaned := strings.NewReplacer("$", "", ",", "", " ", "").Replace(s)
	cleaned = strings.TrimSpace(cleaned)
	if cleaned == "" {
		return 0
	}
	v, err := strconv.ParseFloat(cleaned, 64)
	if err != nil {
		return 0
	}
	return v
}
