package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/dknathalage/tallyo/internal/audit"
	"github.com/dknathalage/tallyo/internal/db/gen"
	"github.com/google/uuid"
)

// ColumnMapping is the domain view of a row in the column_mappings table. All
// nullable columns are unwrapped to plain values.
type ColumnMapping struct {
	ID              int64  `json:"id"`
	UUID            string `json:"uuid"`
	Name            string `json:"name"`
	EntityType      string `json:"entityType"`
	Mapping         string `json:"mapping"`
	TierMapping     string `json:"tierMapping"`
	MetadataMapping string `json:"metadataMapping"`
	FileType        string `json:"fileType"`
	SheetName       string `json:"sheetName"`
	HeaderRow       int64  `json:"headerRow"`
	CreatedAt       string `json:"createdAt"`
	UpdatedAt       string `json:"updatedAt"`
}

// ColumnMappingInput is the writable subset of a column mapping.
type ColumnMappingInput struct {
	Name            string `json:"name"`
	EntityType      string `json:"entityType"`
	Mapping         string `json:"mapping"`
	TierMapping     string `json:"tierMapping"`
	MetadataMapping string `json:"metadataMapping"`
	FileType        string `json:"fileType"`
	SheetName       string `json:"sheetName"`
	HeaderRow       int64  `json:"headerRow"`
}

// ColumnMappingsRepo reads and writes the column_mappings table with audited
// mutations.
type ColumnMappingsRepo struct {
	db *sql.DB
}

// NewColumnMappings constructs a repository. A nil db is a programmer error.
func NewColumnMappings(db *sql.DB) *ColumnMappingsRepo {
	if db == nil {
		panic("repository: NewColumnMappings requires a non-nil *sql.DB")
	}
	return &ColumnMappingsRepo{db: db}
}

// List returns column mappings ordered by name. An empty entityType returns all
// mappings; otherwise only those matching the entity type. Never nil.
func (r *ColumnMappingsRepo) List(ctx context.Context, entityType string) ([]*ColumnMapping, error) {
	if entityType == "" {
		rows, err := gen.New(r.db).ListColumnMappings(ctx)
		if err != nil {
			return nil, fmt.Errorf("list column mappings: %w", err)
		}
		out := make([]*ColumnMapping, 0, len(rows))
		for i := range rows {
			out = append(out, toColumnMapping(rows[i]))
		}
		return out, nil
	}
	rows, err := gen.New(r.db).ListColumnMappingsByEntity(ctx, entityType)
	if err != nil {
		return nil, fmt.Errorf("list column mappings by entity: %w", err)
	}
	out := make([]*ColumnMapping, 0, len(rows))
	for i := range rows {
		out = append(out, toColumnMapping(rows[i]))
	}
	return out, nil
}

// Get returns the mapping, or (nil, nil) when none matches.
func (r *ColumnMappingsRepo) Get(ctx context.Context, id int64) (*ColumnMapping, error) {
	row, err := gen.New(r.db).GetColumnMapping(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get column mapping: %w", err)
	}
	return toColumnMapping(row), nil
}

// Create inserts a mapping and writes one audit row, atomically. Missing
// optional fields fall back to schema defaults.
func (r *ColumnMappingsRepo) Create(ctx context.Context, in ColumnMappingInput) (*ColumnMapping, error) {
	if in.Name == "" {
		return nil, errors.New("create column mapping: name is required")
	}
	applyColumnMappingDefaults(&in)

	var created gen.ColumnMapping
	err := audit.WithTx(ctx, r.db, audit.Entry{Action: ""}, func(tx *sql.Tx) error {
		now := time.Now().UTC().Format(time.RFC3339)
		m, e := gen.New(tx).CreateColumnMapping(ctx, gen.CreateColumnMappingParams{
			Uuid:            uuid.NewString(),
			Name:            in.Name,
			EntityType:      in.EntityType,
			Mapping:         in.Mapping,
			TierMapping:     nz(in.TierMapping),
			MetadataMapping: nz(in.MetadataMapping),
			FileType:        nz(in.FileType),
			SheetName:       nz(in.SheetName),
			HeaderRow:       nzInt(in.HeaderRow),
			CreatedAt:       now,
			UpdatedAt:       now,
		})
		if e != nil {
			return fmt.Errorf("insert: %w", e)
		}
		created = m
		return audit.Log(ctx, tx, audit.Entry{
			EntityType: "column_mapping",
			EntityID:   m.ID,
			Action:     "create",
			Changes:    audit.Changes(map[string]any{"name": in.Name}),
		})
	})
	if err != nil {
		return nil, fmt.Errorf("create column mapping: %w", err)
	}
	return toColumnMapping(created), nil
}

// Update writes the mapping's fields and one audit row, atomically. Returns
// (nil, nil) when the mapping does not exist so the caller can 404.
func (r *ColumnMappingsRepo) Update(ctx context.Context, id int64, in ColumnMappingInput) (*ColumnMapping, error) {
	if in.Name == "" {
		return nil, errors.New("update column mapping: name is required")
	}
	applyColumnMappingDefaults(&in)

	var updated gen.ColumnMapping
	var missing bool
	err := audit.WithTx(ctx, r.db, audit.Entry{
		EntityType: "column_mapping",
		EntityID:   id,
		Action:     "update",
		Changes:    audit.Changes(map[string]any{"name": in.Name}),
	}, func(tx *sql.Tx) error {
		now := time.Now().UTC().Format(time.RFC3339)
		m, e := gen.New(tx).UpdateColumnMapping(ctx, gen.UpdateColumnMappingParams{
			Name:            in.Name,
			EntityType:      in.EntityType,
			Mapping:         in.Mapping,
			TierMapping:     nz(in.TierMapping),
			MetadataMapping: nz(in.MetadataMapping),
			FileType:        nz(in.FileType),
			SheetName:       nz(in.SheetName),
			HeaderRow:       nzInt(in.HeaderRow),
			UpdatedAt:       now,
			ID:              id,
		})
		if errors.Is(e, sql.ErrNoRows) {
			missing = true
			return e
		}
		if e != nil {
			return fmt.Errorf("update: %w", e)
		}
		updated = m
		return nil
	})
	if missing {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("update column mapping: %w", err)
	}
	return toColumnMapping(updated), nil
}

// Delete removes a mapping and writes one audit row, atomically.
func (r *ColumnMappingsRepo) Delete(ctx context.Context, id int64) error {
	return audit.WithTx(ctx, r.db, audit.Entry{Action: ""}, func(tx *sql.Tx) error {
		if e := gen.New(tx).DeleteColumnMapping(ctx, id); e != nil {
			return fmt.Errorf("delete: %w", e)
		}
		return audit.Log(ctx, tx, audit.Entry{
			EntityType: "column_mapping",
			EntityID:   id,
			Action:     "delete",
		})
	})
}

// applyColumnMappingDefaults fills empty optional fields with schema defaults.
func applyColumnMappingDefaults(in *ColumnMappingInput) {
	if in.EntityType == "" {
		in.EntityType = "catalog"
	}
	if in.Mapping == "" {
		in.Mapping = "{}"
	}
	if in.TierMapping == "" {
		in.TierMapping = "{}"
	}
	if in.MetadataMapping == "" {
		in.MetadataMapping = "[]"
	}
	if in.FileType == "" {
		in.FileType = "csv"
	}
	if in.HeaderRow == 0 {
		in.HeaderRow = 1
	}
}

// toColumnMapping maps a generated row to the domain ColumnMapping.
func toColumnMapping(row gen.ColumnMapping) *ColumnMapping {
	return &ColumnMapping{
		ID:              row.ID,
		UUID:            row.Uuid,
		Name:            row.Name,
		EntityType:      row.EntityType,
		Mapping:         row.Mapping,
		TierMapping:     row.TierMapping.String,
		MetadataMapping: row.MetadataMapping.String,
		FileType:        row.FileType.String,
		SheetName:       row.SheetName.String,
		HeaderRow:       row.HeaderRow.Int64,
		CreatedAt:       row.CreatedAt,
		UpdatedAt:       row.UpdatedAt,
	}
}
