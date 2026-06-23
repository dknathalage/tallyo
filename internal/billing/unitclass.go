package billing

import "strings"

// UnitClass groups catalogue units of measure by how their quantity is captured.
type UnitClass int

const (
	UnitCount    UnitClass = iota // typed number (EA, D, WK, MON, YR, …)
	UnitTime                      // start+end → duration (H, hour)
	UnitDistance                  // typed distance (KM)
)

// Classify maps a catalogue unit_of_measure to its input class. Unknown units fall
// to UnitCount. ponytail: small switch; extend when a new unit class appears.
func Classify(unit string) UnitClass {
	switch strings.ToUpper(strings.TrimSpace(unit)) {
	case "H", "HOUR", "HR":
		return UnitTime
	case "KM":
		return UnitDistance
	default:
		return UnitCount
	}
}
