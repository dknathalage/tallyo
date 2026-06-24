package estimate

import (
	"encoding/json"
	"testing"
)

// TestEstimateJSONConvertedInvoiceIsUUID asserts the serialized estimate exposes
// convertedInvoiceId as the produced invoice's uuid (so the SPA can link to it),
// never the internal int FK.
func TestEstimateJSONConvertedInvoiceIsUUID(t *testing.T) {
	convertedID := "invoice-int-fk"
	convertedUUID := "invoice-uuid"
	e := Estimate{
		ID:                   "estimate-uuid",
		ConvertedInvoiceID:   &convertedID,
		ConvertedInvoiceUUID: &convertedUUID,
	}
	b, err := json.Marshal(e)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got := m["convertedInvoiceId"]; got != convertedUUID {
		t.Fatalf("convertedInvoiceId is not the invoice uuid: %v (json: %s)", got, b)
	}
	if m["id"] != "estimate-uuid" {
		t.Fatalf("estimate id is not the uuid: %v", m["id"])
	}
}
