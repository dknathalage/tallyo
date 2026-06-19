package db

import "database/sql"

// NullID wraps an optional id into a sql.NullInt64 (invalid when nil).
func NullID(p *int64) sql.NullInt64 {
	if p == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: *p, Valid: true}
}

// PtrID unwraps a sql.NullInt64 into a *int64 (nil when invalid).
func PtrID(n sql.NullInt64) *int64 {
	if !n.Valid {
		return nil
	}
	v := n.Int64
	return &v
}

// NzMaybe wraps a string into a sql.NullString that is invalid (SQL NULL) when
// the string is empty, and valid otherwise. Used for genuinely optional columns.
func NzMaybe(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}

// Nz wraps a string into a valid sql.NullString.
func Nz(s string) sql.NullString {
	return sql.NullString{String: s, Valid: true}
}

// B2i maps a bool to the int64 column convention (true -> 1, false -> 0).
func B2i(b bool) int64 {
	if b {
		return 1
	}
	return 0
}
