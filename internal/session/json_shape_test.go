package session

import (
	"encoding/json"
	"testing"
)

// TestSessionJSONFKsAreUUIDs asserts the serialized session never leaks the internal
// int FKs: invoiceId is the linked invoice's uuid (not an int), and the author
// user FK is dropped from the API surface entirely (nothing links on it).
func TestSessionJSONFKsAreUUIDs(t *testing.T) {
	invoiceID, authorID := "invoice-int-fk", "author-int-fk"
	invoiceUUID := "invoice-uuid"
	s := Session{
		ID:           "session-uuid",
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
		t.Fatalf("session JSON leaks authorUserId int FK: %s", b)
	}
	if m["id"] != "session-uuid" {
		t.Fatalf("session id is not the uuid: %v", m["id"])
	}
}
