package shift

import (
	"encoding/json"
	"testing"
)

// TestShiftJSONFKsAreUUIDs asserts the serialized shift never leaks the internal
// int FKs: invoiceId is the linked invoice's uuid (not an int), and the author
// user FK is dropped from the API surface entirely (nothing links on it).
func TestShiftJSONFKsAreUUIDs(t *testing.T) {
	invoiceID, authorID := int64(11), int64(22)
	invoiceUUID := "invoice-uuid"
	s := Shift{
		ID:           1,
		UUID:         "shift-uuid",
		InvoiceID:    &invoiceID,
		InvoiceUUID:  &invoiceUUID,
		AuthorUserID: &authorID,
	}
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got := m["invoiceId"]; got != invoiceUUID {
		t.Fatalf("invoiceId is not the invoice uuid: %v (json: %s)", got, b)
	}
	if _, ok := m["authorUserId"]; ok {
		t.Fatalf("shift JSON leaks authorUserId int FK: %s", b)
	}
	if m["id"] != "shift-uuid" {
		t.Fatalf("shift id is not the uuid: %v", m["id"])
	}
}
