package invoice

import "strconv"

// parseIDFromString parses a decimal int64 from s. Used by the List handler
// to parse query-string ids (e.g. ?participantId=123).
func parseIDFromString(s string) (int64, bool) {
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil || n <= 0 {
		return 0, false
	}
	return n, true
}
