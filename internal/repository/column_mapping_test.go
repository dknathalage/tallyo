package repository

import (
	"context"
	"path/filepath"
	"testing"

	appdb "github.com/dknathalage/tallyo/internal/db"
)

func newColumnMappingsRepo(t *testing.T) *ColumnMappingsRepo {
	t.Helper()
	conn, err := appdb.Open(filepath.Join(t.TempDir(), "cm.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	if err := appdb.Migrate(conn); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	return NewColumnMappings(conn)
}

func TestColumnMappingCreate(t *testing.T) {
	repo := newColumnMappingsRepo(t)
	ctx := context.Background()

	m, err := repo.Create(ctx, ColumnMappingInput{
		Name:            "Vendor CSV",
		EntityType:      "catalog",
		Mapping:         `{"name":"A"}`,
		TierMapping:     `{"std":"B"}`,
		MetadataMapping: `["C"]`,
		FileType:        "excel",
		SheetName:       "Sheet1",
		HeaderRow:       3,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if m == nil || m.ID <= 0 {
		t.Fatalf("Create returned %+v, want non-nil with ID>0", m)
	}
	if m.UUID == "" {
		t.Fatal("Create: UUID is empty")
	}
	if m.Name != "Vendor CSV" || m.EntityType != "catalog" || m.Mapping != `{"name":"A"}` {
		t.Fatalf("Create core fields = %+v", m)
	}
	if m.TierMapping != `{"std":"B"}` || m.MetadataMapping != `["C"]` {
		t.Fatalf("Create mapping fields = %+v", m)
	}
	if m.FileType != "excel" || m.SheetName != "Sheet1" || m.HeaderRow != 3 {
		t.Fatalf("Create file fields = %+v", m)
	}
}

func TestColumnMappingCreateRejectsEmptyName(t *testing.T) {
	repo := newColumnMappingsRepo(t)
	if _, err := repo.Create(context.Background(), ColumnMappingInput{Name: ""}); err == nil {
		t.Fatal("Create with empty name: want error, got nil")
	}
}

func TestColumnMappingCreateAppliesDefaults(t *testing.T) {
	repo := newColumnMappingsRepo(t)
	ctx := context.Background()

	m, err := repo.Create(ctx, ColumnMappingInput{Name: "Minimal"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if m.EntityType != "catalog" {
		t.Fatalf("EntityType = %q, want catalog", m.EntityType)
	}
	if m.Mapping != "{}" {
		t.Fatalf("Mapping = %q, want {}", m.Mapping)
	}
	if m.TierMapping != "{}" {
		t.Fatalf("TierMapping = %q, want {}", m.TierMapping)
	}
	if m.MetadataMapping != "[]" {
		t.Fatalf("MetadataMapping = %q, want []", m.MetadataMapping)
	}
	if m.FileType != "csv" {
		t.Fatalf("FileType = %q, want csv", m.FileType)
	}
	if m.HeaderRow != 1 {
		t.Fatalf("HeaderRow = %d, want 1", m.HeaderRow)
	}
}

func TestColumnMappingGet(t *testing.T) {
	repo := newColumnMappingsRepo(t)
	ctx := context.Background()

	created, err := repo.Create(ctx, ColumnMappingInput{Name: "X"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := repo.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got == nil || got.Name != "X" {
		t.Fatalf("Get = %+v, want Name=X", got)
	}

	missing, err := repo.Get(ctx, 99999)
	if err != nil {
		t.Fatalf("Get missing: %v", err)
	}
	if missing != nil {
		t.Fatalf("Get missing = %+v, want nil", missing)
	}
}

func TestColumnMappingListOrderedAndByEntity(t *testing.T) {
	repo := newColumnMappingsRepo(t)
	ctx := context.Background()

	if _, err := repo.Create(ctx, ColumnMappingInput{Name: "Beta", EntityType: "catalog"}); err != nil {
		t.Fatalf("Create Beta: %v", err)
	}
	if _, err := repo.Create(ctx, ColumnMappingInput{Name: "Alpha", EntityType: "catalog"}); err != nil {
		t.Fatalf("Create Alpha: %v", err)
	}
	if _, err := repo.Create(ctx, ColumnMappingInput{Name: "Gamma", EntityType: "payer"}); err != nil {
		t.Fatalf("Create Gamma: %v", err)
	}

	all, err := repo.List(ctx, "")
	if err != nil {
		t.Fatalf("List all: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("List all len = %d, want 3", len(all))
	}
	if all[0].Name != "Alpha" || all[1].Name != "Beta" || all[2].Name != "Gamma" {
		t.Fatalf("List all order = %q,%q,%q", all[0].Name, all[1].Name, all[2].Name)
	}

	catalog, err := repo.List(ctx, "catalog")
	if err != nil {
		t.Fatalf("List catalog: %v", err)
	}
	if len(catalog) != 2 {
		t.Fatalf("List catalog len = %d, want 2", len(catalog))
	}
	for _, m := range catalog {
		if m.EntityType != "catalog" {
			t.Fatalf("List catalog returned entity %q", m.EntityType)
		}
	}
}

func TestColumnMappingListEmptyNonNil(t *testing.T) {
	repo := newColumnMappingsRepo(t)
	list, err := repo.List(context.Background(), "")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if list == nil {
		t.Fatal("List returned nil, want empty slice")
	}
	if len(list) != 0 {
		t.Fatalf("List len = %d, want 0", len(list))
	}
}

func TestColumnMappingUpdate(t *testing.T) {
	repo := newColumnMappingsRepo(t)
	ctx := context.Background()

	created, err := repo.Create(ctx, ColumnMappingInput{Name: "Old", EntityType: "catalog"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	updated, err := repo.Update(ctx, created.ID, ColumnMappingInput{
		Name:       "New",
		EntityType: "payer",
		Mapping:    `{"k":"v"}`,
		HeaderRow:  2,
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated == nil {
		t.Fatal("Update returned nil")
	}
	if updated.Name != "New" || updated.EntityType != "payer" || updated.Mapping != `{"k":"v"}` || updated.HeaderRow != 2 {
		t.Fatalf("Update = %+v", updated)
	}

	missing, err := repo.Update(ctx, 99999, ColumnMappingInput{Name: "Nope"})
	if err != nil {
		t.Fatalf("Update missing: %v", err)
	}
	if missing != nil {
		t.Fatalf("Update missing = %+v, want nil", missing)
	}
}

func TestColumnMappingDelete(t *testing.T) {
	repo := newColumnMappingsRepo(t)
	ctx := context.Background()

	created, err := repo.Create(ctx, ColumnMappingInput{Name: "ToDelete"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := repo.Delete(ctx, created.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	got, err := repo.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("Get after delete: %v", err)
	}
	if got != nil {
		t.Fatalf("row still present after delete: %+v", got)
	}
}

func TestColumnMappingAuditRows(t *testing.T) {
	repo := newColumnMappingsRepo(t)
	ctx := context.Background()

	m, err := repo.Create(ctx, ColumnMappingInput{Name: "A"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := repo.Update(ctx, m.ID, ColumnMappingInput{Name: "A2"}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if err := repo.Delete(ctx, m.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	var n int
	if err := repo.db.QueryRow(
		"SELECT COUNT(*) FROM audit_log WHERE entity_type='column_mapping'",
	).Scan(&n); err != nil {
		t.Fatalf("count audit: %v", err)
	}
	if n != 3 {
		t.Fatalf("audit rows = %d, want 3 (create + update + delete)", n)
	}

	var createID int64
	if err := repo.db.QueryRow(
		"SELECT entity_id FROM audit_log WHERE entity_type='column_mapping' AND action='create'",
	).Scan(&createID); err != nil {
		t.Fatalf("read create audit id: %v", err)
	}
	if createID != m.ID {
		t.Fatalf("create audit entity_id = %d, want %d", createID, m.ID)
	}
}
