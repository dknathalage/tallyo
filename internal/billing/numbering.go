package billing

// Shared billing-document numbering. Invoices ("INV-") and estimates ("EST-")
// share one per-tenant numbering implementation: read the current max suffix for
// the prefix inside the caller's tx (gen.MaxInvoiceNumberLike scans the invoices
// table, which holds both converted-estimate and recurring-generated documents),
// then format the next number. Callers wrap their create tx in
// numbering.WithRetry so a UNIQUE collision from a concurrent creator is retried.

import (
	"context"
	"fmt"

	"github.com/dknathalage/tallyo/internal/db/gen"
	"github.com/dknathalage/tallyo/internal/numbering"
)

// NextNumber allocates the next per-tenant document number for prefix (e.g.
// "INV-NNNN"). It is shared by the invoice, estimate and recurring slices via
// this billing package so none of them import each other.
func NextNumber(ctx context.Context, q *gen.Queries, tenantID, prefix string) (string, error) {
	max, err := q.MaxInvoiceNumberLike(ctx, gen.MaxInvoiceNumberLikeParams{
		PrefixLen: int64(len(prefix)),
		TenantID:  tenantID,
		Pattern:   prefix + "%",
	})
	if err != nil {
		return "", fmt.Errorf("next number: %w", err)
	}
	return numbering.Format(prefix, max), nil
}
