package invoice

import (
	"database/sql"
	"strconv"
)

// b2i maps a bool to the int64 column convention (true -> 1, false -> 0).
func b2i(b bool) int64 {
	if b {
		return 1
	}
	return 0
}

// nullID wraps an optional id into a sql.NullInt64 (invalid when nil).
func nullID(p *int64) sql.NullInt64 {
	if p == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: *p, Valid: true}
}

// ptrID unwraps a sql.NullInt64 into a *int64 (nil when invalid).
func ptrID(n sql.NullInt64) *int64 {
	if !n.Valid {
		return nil
	}
	v := n.Int64
	return &v
}

// nzMaybe wraps a string into a sql.NullString that is invalid (SQL NULL) when
// the string is empty, and valid otherwise.
func nzMaybe(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}

// parseIDFromString parses a decimal int64 from s. Used by the List handler
// to parse query-string ids (e.g. ?participantId=123).
func parseIDFromString(s string) (int64, bool) {
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil || n <= 0 {
		return 0, false
	}
	return n, true
}
