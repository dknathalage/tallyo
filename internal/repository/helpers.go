package repository

// b2i maps a bool to the int64 column convention (true -> 1, false -> 0). Shared
// by the repositories that persist boolean flags as integers.
func b2i(b bool) int64 {
	if b {
		return 1
	}
	return 0
}
