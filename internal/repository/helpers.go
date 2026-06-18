package repository

import "database/sql"

// b2i maps a bool to the int64 column convention (true -> 1, false -> 0). Shared
// by the repositories that persist boolean flags as integers.
func b2i(b bool) int64 {
	if b {
		return 1
	}
	return 0
}

// nz wraps a string into a valid sql.NullString. Shared by repositories that
// persist optional string columns as nullable SQL values.
func nz(s string) sql.NullString {
	return sql.NullString{String: s, Valid: true}
}

// nullID wraps an optional id into a sql.NullInt64 (invalid when nil). Shared
// by repositories that hold nullable FK columns.
func nullID(p *int64) sql.NullInt64 {
	if p == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: *p, Valid: true}
}

// ptrID unwraps a sql.NullInt64 into a *int64 (nil when invalid). Shared by
// repositories that project nullable FK columns onto domain types.
func ptrID(n sql.NullInt64) *int64 {
	if !n.Valid {
		return nil
	}
	v := n.Int64
	return &v
}

// nzMaybe wraps a string into a sql.NullString that is invalid (SQL NULL) when
// the string is empty, and valid otherwise. Used for genuinely optional columns.
func nzMaybe(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}

// orDefault returns s when non-empty, otherwise returns def.
func orDefault(s, def string) string {
	if s != "" {
		return s
	}
	return def
}
