package importer

import (
	"regexp"
	"strings"
)

// Suggestion is the auto-detected mapping proposal from DetectMapping. Fields
// maps a source header to one of name|sku|unit|category|rate. BaseHeader is the
// header chosen as the base catalog rate ("" if none). PriceCols are the other
// detected price columns, in original header order, each proposable as a tier.
type Suggestion struct {
	Fields     map[string]string `json:"fields"`
	BaseHeader string            `json:"baseHeader"`
	PriceCols  []PriceColumn     `json:"priceCols"`
}

// PriceColumn is a non-base price column the user can map to a tier.
type PriceColumn struct {
	Header      string `json:"header"`
	SuggestName string `json:"suggestName"`
}

// tokenizeAll pre-tokenizes a list of needle phrases once at package init, so
// matchField/priceLikeByName never re-tokenize on every (header, needle) pair.
func tokenizeAll(ss ...string) [][]string {
	out := make([][]string, len(ss))
	for i, s := range ss { // bounded by len(ss)
		out[i] = tokens(s)
	}
	return out
}

// fieldSynonyms lists, per target field, the token-sequences that identify a
// header. Order matters: fields are claimed top-down, each at most once, and a
// header is consumed by the first field that claims it. Needles are matched as
// contiguous token sub-sequences (so "id" never matches "video") and are
// pre-tokenized once at package scope.
var fieldSynonyms = []struct {
	field   string
	needles [][]string
}{
	{"sku", tokenizeAll("support item number", "item number", "item code", "sku", "code", "ref")},
	{"name", tokenizeAll("support item name", "item name", "name", "description", "product", "service")},
	{"category", tokenizeAll("support category", "category", "group", "class", "type")},
	{"unit", tokenizeAll("unit of measure", "unit", "uom")},
	{"rate", tokenizeAll("unit price", "base price", "price limit", "price", "rate", "cost", "amount")},
}

// priceHeaderNeedles flag a header as price-like by name alone (pre-tokenized).
var priceHeaderNeedles = tokenizeAll("price", "rate", "cost", "amount", "cap", "limit", "fee")

var currencyRe = regexp.MustCompile(`^\$?\s*\d{1,3}(,\d{3})*(\.\d{1,2})?$|^\$?\s*\d+\.\d{1,2}$`)

// tokens normalizes a header to lower-case alphanumeric words. Non-ASCII
// characters are treated as word separators.
func tokens(h string) []string {
	var b strings.Builder
	for _, r := range strings.ToLower(h) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else {
			b.WriteRune(' ')
		}
	}
	return strings.Fields(b.String())
}

// containsSeq reports whether needle's tokens appear contiguously in hay.
func containsSeq(hay, needle []string) bool {
	if len(needle) == 0 || len(needle) > len(hay) {
		return false
	}
	for i := 0; i+len(needle) <= len(hay); i++ { // bounded by len(hay)
		match := true
		for j := range needle { // bounded by len(needle)
			if hay[i+j] != needle[j] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

// matchField returns the field a header maps to, given already-claimed fields.
func matchField(header string, claimed map[string]bool) string {
	hay := tokens(header)
	for _, fs := range fieldSynonyms { // bounded by len(fieldSynonyms)
		if claimed[fs.field] {
			continue
		}
		for _, n := range fs.needles { // bounded by len(needles)
			if containsSeq(hay, n) {
				return fs.field
			}
		}
	}
	return ""
}

// priceLikeByName reports whether the header name alone signals a price column.
func priceLikeByName(header string) bool {
	hay := tokens(header)
	for _, n := range priceHeaderNeedles { // bounded by len(priceHeaderNeedles)
		if containsSeq(hay, n) {
			return true
		}
	}
	return false
}

// priceLikeByValue reports whether the majority of sampled non-empty cells for a
// header parse as currency with a decimal/$, excluding integer-only code columns.
func priceLikeByValue(header string, sample []map[string]string) bool {
	nonEmpty, currency := 0, 0
	for _, row := range sample { // bounded by len(sample)
		v := strings.TrimSpace(row[header])
		if v == "" {
			continue
		}
		nonEmpty++
		if (strings.Contains(v, ".") || strings.Contains(v, "$")) && currencyRe.MatchString(v) {
			currency++
		}
	}
	return nonEmpty > 0 && currency*2 > nonEmpty
}

// DetectMapping infers a Suggestion from headers plus up to a handful of sample
// rows. It claims one header per field, then treats remaining price-like columns
// as the base rate (a rate-named column, else the left-most) plus tier columns.
func DetectMapping(headers []string, sample []map[string]string) Suggestion {
	s := Suggestion{Fields: map[string]string{}, PriceCols: []PriceColumn{}}
	claimed := map[string]bool{}
	rateHeader := ""
	for _, h := range headers { // bounded by len(headers)
		f := matchField(h, claimed)
		if f == "" {
			continue
		}
		claimed[f] = true
		if f == "rate" {
			rateHeader = h
			continue // base rate is tracked separately, not in Fields
		}
		s.Fields[h] = f
	}

	var priceCols []string
	for _, h := range headers { // bounded by len(headers)
		if _, taken := s.Fields[h]; taken || h == rateHeader {
			continue
		}
		if priceLikeByName(h) || priceLikeByValue(h, sample) {
			priceCols = append(priceCols, h)
		}
	}

	s.BaseHeader = rateHeader
	if s.BaseHeader == "" && len(priceCols) > 0 {
		s.BaseHeader = priceCols[0]
		priceCols = priceCols[1:]
	}
	if rateHeader != "" {
		s.Fields[rateHeader] = "rate"
	} else if s.BaseHeader != "" {
		s.Fields[s.BaseHeader] = "rate"
	}
	for _, h := range priceCols { // bounded by len(priceCols)
		s.PriceCols = append(s.PriceCols, PriceColumn{Header: h, SuggestName: h})
	}
	return s
}
