package agent

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	appdb "github.com/dknathalage/tallyo/internal/db"
	"github.com/dknathalage/tallyo/internal/realtime"
	"github.com/dknathalage/tallyo/internal/reqctx"
	"github.com/dknathalage/tallyo/internal/service"
)

func noopHandler(context.Context, json.RawMessage) (Result, error) { return Result{}, nil }

func TestRegistryReadToolRuns(t *testing.T) {
	reg := NewRegistry()
	reg.Register(Tool{
		Name: "list_invoices", Risk: RiskRead, Render: "table",
		Schema: []byte(`{"type":"object","properties":{}}`),
		Handler: func(ctx context.Context, _ json.RawMessage) (Result, error) {
			return Result{JSON: []string{"INV-1"}, Render: "table"}, nil
		},
	})
	tl, ok := reg.Get("list_invoices")
	if !ok || tl.Risk != RiskRead {
		t.Fatal("tool not registered")
	}
	res, err := tl.Handler(context.Background(), []byte(`{}`))
	if err != nil || res.Render != "table" {
		t.Fatalf("handler: %+v %v", res, err)
	}
}

func TestRegistryRejectsDuplicate(t *testing.T) {
	reg := NewRegistry()
	tl := Tool{Name: "x", Risk: RiskRead, Schema: []byte(`{}`), Handler: noopHandler}
	reg.Register(tl)
	if err := reg.register(tl); err == nil {
		t.Fatal("expected duplicate error")
	}
}

func TestRegistryDefsExcludeMeta(t *testing.T) {
	reg := NewRegistry()
	reg.Register(Tool{Name: "r", Risk: RiskRead, Schema: []byte(`{}`), Handler: noopHandler})
	reg.Register(Tool{Name: "m", Risk: RiskMeta, Schema: []byte(`{}`), Handler: noopHandler})
	if len(reg.Defs(false)) != 1 {
		t.Fatalf("Defs(false) should exclude meta")
	}
	if len(reg.Defs(true)) != 2 {
		t.Fatalf("Defs(true) should include meta")
	}
}

// newTestInvoiceSvc opens a temp DB, migrates it, seeds a tenant + participant,
// and returns the InvoiceService plus the tenant id.
func newTestInvoiceSvc(t *testing.T) (*service.InvoiceService, int64) {
	t.Helper()
	conn, err := appdb.Open(filepath.Join(t.TempDir(), "tool.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	if err := appdb.Migrate(conn); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	hub := realtime.NewHub()
	svc := service.NewInvoiceService(conn, hub)

	// Seed a tenant directly via SQL (avoids importing gen from test file).
	res, err := conn.Exec(
		`INSERT INTO tenants (uuid, name, status, created_at, updated_at) VALUES (?, 'Test Org', 'active', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`,
		"test-uuid-tools")
	if err != nil {
		t.Fatalf("seed tenant: %v", err)
	}
	tenantID, _ := res.LastInsertId()
	return svc, tenantID
}

func TestListInvoicesToolHappyPath(t *testing.T) {
	svc, tenantID := newTestInvoiceSvc(t)
	tool := NewListInvoicesTool(svc)

	// Verify tool metadata.
	if tool.Name != "list_invoices" {
		t.Fatalf("Name = %q, want list_invoices", tool.Name)
	}
	if tool.Risk != RiskRead {
		t.Fatalf("Risk = %q, want read", tool.Risk)
	}
	if tool.Render != "table" {
		t.Fatalf("Render = %q, want table", tool.Render)
	}

	ctx := reqctx.WithTenant(context.Background(), tenantID)
	res, err := tool.Handler(ctx, []byte(`{}`))
	if err != nil {
		t.Fatalf("Handler (no filter): %v", err)
	}
	if res.Render != "table" {
		t.Fatalf("Render = %q, want table", res.Render)
	}
	// Empty DB: expect a non-nil slice.
	if res.JSON == nil {
		t.Fatal("Handler returned nil JSON")
	}
}

func TestListInvoicesToolStatusFilter(t *testing.T) {
	svc, tenantID := newTestInvoiceSvc(t)
	tool := NewListInvoicesTool(svc)
	ctx := reqctx.WithTenant(context.Background(), tenantID)

	res, err := tool.Handler(ctx, []byte(`{"status":"draft"}`))
	if err != nil {
		t.Fatalf("Handler (status=draft): %v", err)
	}
	if res.Render != "table" {
		t.Fatalf("Render = %q, want table", res.Render)
	}
	if res.JSON == nil {
		t.Fatal("Handler returned nil JSON")
	}
}

func TestListInvoicesToolBadJSON(t *testing.T) {
	svc, _ := newTestInvoiceSvc(t)
	tool := NewListInvoicesTool(svc)

	_, err := tool.Handler(context.Background(), []byte(`not-json`))
	if err == nil {
		t.Fatal("expected error for bad JSON input")
	}
}
