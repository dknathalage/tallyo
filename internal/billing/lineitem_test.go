package billing

import (
	"encoding/json"
	"testing"
)

func TestLineItemTypes(t *testing.T) {
	var in LineItemInput
	if in.Quantity != 0 || in.Taxable {
		t.Fatalf("unexpected zero value: %+v", in)
	}
	li := LineItem{Code: "01_011", Quantity: 2, UnitPrice: 10}
	if li.Code != "01_011" {
		t.Fatalf("LineItem field mismatch")
	}
}

// TestLineItemJSONNoParentIntFK asserts the serialized line item never leaks the
// internal int parent FKs (shiftId/invoiceId): a line item is always fetched
// embedded in its parent, so the parent pointer is dropped from the API surface.
func TestLineItemJSONNoParentIntFK(t *testing.T) {
	shiftID, invoiceID := int64(7), int64(9)
	li := LineItem{ID: 3, UUID: "item-uuid", ShiftID: &shiftID, InvoiceID: &invoiceID}
	b, err := json.Marshal(li)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, ok := m["shiftId"]; ok {
		t.Fatalf("line item JSON leaks shiftId: %s", b)
	}
	if _, ok := m["invoiceId"]; ok {
		t.Fatalf("line item JSON leaks invoiceId: %s", b)
	}
	if m["id"] != "item-uuid" {
		t.Fatalf("line item id is not the uuid: %v", m["id"])
	}
}
