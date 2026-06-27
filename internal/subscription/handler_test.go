package subscription

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dknathalage/tallyo/internal/auth"
	appdb "github.com/dknathalage/tallyo/internal/db"
	"github.com/dknathalage/tallyo/internal/reqctx"
)

func fakeClient(t *testing.T) *Client {
	t.Helper()
	c, err := NewClient(Config{SecretKey: "sk_test_x", PriceID: "price_x", TrialDays: 90})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return c
}

func TestStatusEndpoint(t *testing.T) {
	conn := appdb.OpenTestDB(t)
	ctx := context.Background()
	tenant, err := auth.NewTenants(conn).Create(ctx, "Acme")
	if err != nil {
		t.Fatalf("create tenant: %v", err)
	}
	store := NewStore(conn)
	if _, err := store.Apply(ctx, Update{
		TenantID: tenant.ID, StripeCustomerID: "cus_1", Status: StatusTrialing,
		TrialEnd: "2026-09-25T00:00:00Z", SyncedAt: "2026-06-27T10:00:00Z",
	}); err != nil {
		t.Fatalf("apply: %v", err)
	}

	h := NewHandler(fakeClient(t), store, auth.NewTenants(conn))

	req := httptest.NewRequest(http.MethodGet, "/billing", nil)
	req = req.WithContext(reqctx.WithTenant(req.Context(), tenant.ID))
	rec := httptest.NewRecorder()
	h.Status(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status code = %d, want 200", rec.Code)
	}
	var body struct {
		Status   string `json:"status"`
		TrialEnd string `json:"trialEnd"`
		Entitled bool   `json:"entitled"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Status != StatusTrialing || !body.Entitled {
		t.Errorf("body = %+v, want trialing+entitled", body)
	}
}
