package billing

import (
	"context"
	"encoding/json"

	"github.com/dknathalage/tallyo/internal/db"
	"github.com/dknathalage/tallyo/internal/db/gen"
)

// SnapshotBuilder reads entity rows from the database and renders the default
// JSON snapshot for each. It is used by InvoicesRepo, EstimatesRepo, and
// RecurringRepo to build snapshot fields at create time without requiring
// those repos to embed each other.
type SnapshotBuilder struct {
	db db.Executor
}

// NewSnapshotBuilder constructs a SnapshotBuilder. A nil db is a programmer
// error.
func NewSnapshotBuilder(db db.Executor) *SnapshotBuilder {
	if db == nil {
		panic("billing: NewSnapshotBuilder requires a non-nil *sql.DB")
	}
	return &SnapshotBuilder{db: db}
}

// SnapshotJSON builds the default snapshot JSON for an entity. metadata is
// parsed into an object (or {} on failure) so the stored shape is uniform.
func SnapshotJSON(name, email, phone, address, metadata string) string {
	var meta any
	if err := json.Unmarshal([]byte(metadata), &meta); err != nil || metadata == "" {
		meta = map[string]any{}
	}
	b, err := json.Marshal(map[string]any{
		"name": name, "email": email, "phone": phone, "address": address, "metadata": meta,
	})
	if err != nil {
		return "{}"
	}
	return string(b)
}

// Business reads the tenant's business profile and renders a default snapshot.
func (b *SnapshotBuilder) Business(ctx context.Context, tenantID int64) string {
	bp, err := gen.New(b.db).GetBusinessProfile(ctx, tenantID)
	if err != nil {
		return "{}"
	}
	return SnapshotJSON(bp.Name, bp.Email.String, bp.Phone.String, bp.Address.String, bp.Metadata.String)
}

// Client reads the client and renders a default snapshot.
func (b *SnapshotBuilder) Client(ctx context.Context, tenantID, clientID int64) string {
	p, err := gen.New(b.db).GetClientByID(ctx, gen.GetClientByIDParams{TenantID: tenantID, ID: clientID})
	if err != nil {
		return "{}"
	}
	return SnapshotJSON(p.Name, p.Email.String, p.Phone.String, p.Address.String, p.Metadata.String)
}

// Payer renders a default snapshot for the given payer, or "{}"
// when none is set.
func (b *SnapshotBuilder) Payer(ctx context.Context, tenantID int64, payerID *int64) string {
	if payerID == nil {
		return "{}"
	}
	pm, err := gen.New(b.db).GetPayerByID(ctx, gen.GetPayerByIDParams{TenantID: tenantID, ID: *payerID})
	if err != nil {
		return "{}"
	}
	return SnapshotJSON(pm.Name, pm.Email.String, pm.Phone.String, pm.Address.String, pm.Metadata.String)
}
