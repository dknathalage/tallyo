package db

import (
	"strconv"
	"strings"
)

// Rebind converts a query written with sequential `?` placeholders into the
// Postgres `$1, $2, …` positional form, numbering left to right. Hand-assembled
// dynamic SQL (the listquery list/count builders) is authored with `?` for
// readability and rebound here at the single point where the final string is
// built; sqlc-generated queries already use `$n` and never pass through here.
//
// It assumes every `?` is a bind placeholder — true for our generated fragments,
// where identifiers and operators are author-controlled constants and only
// values are bound. There are no `?` inside string literals in those fragments.
func Rebind(query string) string {
	n := strings.Count(query, "?")
	if n == 0 {
		return query
	}
	var b strings.Builder
	b.Grow(len(query) + n) // each ? grows by ~1-2 chars
	i := 0
	for j := 0; j < len(query); j++ { // bounded by len(query)
		c := query[j]
		if c == '?' {
			i++
			b.WriteByte('$')
			b.WriteString(strconv.Itoa(i))
			continue
		}
		b.WriteByte(c)
	}
	return b.String()
}
