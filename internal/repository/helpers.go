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
