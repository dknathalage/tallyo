// Package events gives each domain service a tiny, uniform way to broadcast the
// standard create/update/delete change notifications, replacing the inline
// realtime.Event{...} struct literals that were repeated across every slice.
//
// A Notifier is constructed once per service with its entity name; the service
// then calls n.Created/Updated/Deleted(tenantID, id) after a mutation commits.
// The tenant id is passed EXPLICITLY (not read from a context) so the same
// helper works on background-sweep paths that broadcast tenant-scoped events
// outside any request context.
package events

import "github.com/dknathalage/tallyo/internal/realtime"

// Notifier broadcasts change events for one entity type to the SSE hub.
type Notifier struct {
	hub    *realtime.Hub
	entity string // e.g. "client", "invoice" — the SSE Event.Entity value
}

// New returns a Notifier for the given entity. Both arguments are required;
// a nil hub or empty entity is a wiring (programmer) error.
func New(hub *realtime.Hub, entity string) Notifier {
	if hub == nil {
		panic("events.New: nil hub")
	}
	if entity == "" {
		panic("events.New: empty entity")
	}
	return Notifier{hub: hub, entity: entity}
}

// Created broadcasts a "create" change for the entity id under tenantID.
func (n Notifier) Created(tenantID, id string) { n.emit(tenantID, id, "create") }

// Updated broadcasts an "update" change for the entity id under tenantID.
func (n Notifier) Updated(tenantID, id string) { n.emit(tenantID, id, "update") }

// Deleted broadcasts a "delete" change for the entity id under tenantID.
func (n Notifier) Deleted(tenantID, id string) { n.emit(tenantID, id, "delete") }

// emit constructs and broadcasts the routed event. Non-CRUD broadcasts
// (bulk/sweep events with no single id, custom actions) stay explicit at the
// call site via hub.Broadcast — this helper only covers the common three.
func (n Notifier) emit(tenantID, id, action string) {
	n.hub.Broadcast(realtime.Event{TenantID: tenantID, Entity: n.entity, UUID: id, Action: action})
}
