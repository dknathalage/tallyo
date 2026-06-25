package catalogue

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/dknathalage/tallyo/internal/listquery"
)

// Query returns one page of current catalogue items plus the total row count for
// the filter (ignoring pagination). The clause is built by listquery from an
// allowlisted spec, so its Where/Order fragments are injection-safe.
func (r *Repo) Query(ctx context.Context, tenantID string, c listquery.Clause) ([]*CatalogueItem, int64, error) {
	if tenantID == "" {
		return nil, 0, errors.New("query catalogue: tenant id required")
	}
	var total int64
	countSQL := "SELECT count(*) FROM (" + catalogueListSelect + c.Where + ")"
	countArgs := append([]any{tenantID}, c.CountArgs()...)
	if err := r.db.QueryRowContext(ctx, countSQL, countArgs...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count catalogue: %w", err)
	}
	order := c.Order
	if order == "" {
		order = " ORDER BY name"
	}
	sqlText := catalogueListSelect + c.Where + order + c.Limit
	pageArgs := append([]any{tenantID}, c.Args...)
	rows, err := r.db.QueryContext(ctx, sqlText, pageArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("query catalogue: %w", err)
	}
	defer rows.Close()
	out := make([]*CatalogueItem, 0, 50)
	for rows.Next() { // bounded by LIMIT in the query
		var (
			id        string
			logicalID string
			tenant    string
			code      sql.NullString
			name      string
			unit      sql.NullString
			category  sql.NullString
			unitPrice float64
			taxable   int64
			metadata  string
			version   int64
			isCurrent int64
			createdAt string
			updatedAt string
		)
		if err := rows.Scan(&id, &logicalID, &tenant, &code, &name, &unit, &category,
			&unitPrice, &taxable, &metadata, &version, &isCurrent, &createdAt, &updatedAt); err != nil {
			return nil, 0, fmt.Errorf("scan catalogue item: %w", err)
		}
		out = append(out, &CatalogueItem{
			ID:        id,
			LogicalID: logicalID,
			Code:      code.String,
			Name:      name,
			Unit:      unit.String,
			Category:  category.String,
			UnitPrice: unitPrice,
			Taxable:   taxable == 1,
			Metadata:  metadata,
			Version:   version,
			IsCurrent: isCurrent == 1,
			CreatedAt: createdAt,
			UpdatedAt: updatedAt,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("query catalogue: %w", err)
	}
	return out, total, nil
}

// escapeLike escapes the LIKE wildcards so a user search term is matched
// literally (paired with ESCAPE backslash in the query).
func escapeLike(s string) string {
	r := strings.NewReplacer(
		`\`, `\\`,
		`%`, `\%`,
		`_`, `\_`,
	)
	return r.Replace(s)
}
