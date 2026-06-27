// Package listquery builds safe, parameterized SQL fragments (WHERE/ORDER/LIMIT)
// for list endpoints from a per-resource column allowlist. Client requests never
// supply SQL identifiers or operators — only allowlisted column KEYS and bound
// VALUES — which is what makes dynamic list SQL injection-safe here.
package listquery

import (
	"net/url"
	"strconv"
	"strings"
)

// FilterType selects the operator and value parsing for a column.
type FilterType int

const (
	None   FilterType = iota // sortable but not filterable
	Text                     // LIKE %v%
	Enum                     // IN (?,?,…) from a comma-separated value
	Date                     // >= from AND <= to (f.<key>.from / .to)
	Number                   // >= min AND <= max (f.<key>.min / .max)
)

// ColSpec maps an API key to a CONSTANT SQL column expression we authored.
type ColSpec struct {
	Col    string // e.g. "p.name" — author-controlled constant, never client input
	Filter FilterType
}

// Spec is the allowlist: API key -> ColSpec. Only keys present here are
// filterable/sortable; anything else is rejected.
type Spec map[string]ColSpec

// Pagination bounds.
const (
	DefaultLimit = 50
	MaxLimit     = 200
)

// Clause is the assembled, parameterized SQL tail plus its bound args. Where
// begins with " AND …" so callers splice it after their mandatory tenant filter.
type Clause struct {
	Where string // "" or " AND (...) AND (...)"
	Order string // "" or " ORDER BY <col> ASC|DESC"
	Limit string // " LIMIT ? OFFSET ?"
	Args  []any  // WHERE args first, then limit, offset (always the last two)
}

// Build assembles a safe Clause from request params using spec as the allowlist.
// Invariants: identifiers come only from spec.Col (constants); every value is a
// bound ? arg; operators are fixed per FilterType; dir ∈ {ASC,DESC}; limit/offset
// are clamped ints. Unknown keys are ignored, never interpolated.
func Build(q url.Values, spec Spec) Clause {
	if spec == nil { // a nil spec is a programmer error
		panic("listquery.Build: nil spec")
	}
	if q == nil {
		panic("listquery.Build: nil values")
	}

	var where []string
	var args []any

	// Iterate the SPEC (controlled), never the query, so an unknown key can
	// never reach the SQL.
	for key, col := range spec {
		switch col.Filter {
		case Text:
			if v := q.Get("f." + key); v != "" {
				// ILIKE for case-insensitive matching (Postgres LIKE is
				// case-sensitive; SQLite's default LIKE was not).
				where = append(where, col.Col+" ILIKE ?")
				args = append(args, "%"+v+"%")
			}
		case Enum:
			if v := q.Get("f." + key); v != "" {
				parts := strings.Split(v, ",")
				ph := make([]string, 0, len(parts))
				for _, p := range parts { // bounded by len(parts)
					ph = append(ph, "?")
					args = append(args, p)
				}
				where = append(where, col.Col+" IN ("+strings.Join(ph, ",")+")")
			}
		case Date, Number:
			lo, hi := "from", "to"
			if col.Filter == Number {
				lo, hi = "min", "max"
			}
			if v := q.Get("f." + key + "." + lo); v != "" {
				where = append(where, col.Col+" >= ?")
				args = append(args, v)
			}
			if v := q.Get("f." + key + "." + hi); v != "" {
				where = append(where, col.Col+" <= ?")
				args = append(args, v)
			}
		}
	}

	// Order: sort key must be in spec; dir is asc/desc only.
	order := ""
	if sk := q.Get("sort"); sk != "" {
		if col, ok := spec[sk]; ok {
			dir := "ASC"
			if strings.EqualFold(q.Get("dir"), "desc") {
				dir = "DESC"
			}
			order = " ORDER BY " + col.Col + " " + dir
		}
	}

	// Limit/offset: clamped ints.
	limit := DefaultLimit
	if n, err := strconv.Atoi(q.Get("limit")); err == nil && n > 0 {
		limit = n
	}
	if limit > MaxLimit {
		limit = MaxLimit
	}
	offset := 0
	if p, err := strconv.Atoi(q.Get("page")); err == nil && p > 1 {
		offset = (p - 1) * limit
	}

	whereSQL := ""
	if len(where) > 0 {
		whereSQL = " AND " + strings.Join(where, " AND ")
	}
	args = append(args, limit, offset)
	return Clause{
		Where: whereSQL,
		Order: order,
		Limit: " LIMIT ? OFFSET ?",
		Args:  args,
	}
}

// CountArgs returns the WHERE args only (drops the trailing limit, offset) for
// reuse in a count(*) query. A caller prepends its own fixed args (e.g. tenant).
func (c Clause) CountArgs() []any {
	if len(c.Args) < 2 {
		return nil
	}
	return c.Args[:len(c.Args)-2]
}

// IsListQuery reports whether a request carries DataTable query params (and so
// should return a paged {rows,total} rather than a legacy full array).
func IsListQuery(q url.Values) bool {
	if q.Has("sort") || q.Has("page") || q.Has("limit") {
		return true
	}
	for k := range q {
		if strings.HasPrefix(k, "f.") {
			return true
		}
	}
	return false
}

// Result is one page of a list query plus the unpaginated total. Rows must be
// non-nil so it serializes as [] not null.
type Result[T any] struct {
	Rows  []T   `json:"rows"`
	Total int64 `json:"total"`
}
