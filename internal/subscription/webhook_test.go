package subscription

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/dknathalage/tallyo/internal/auth"
	appdb "github.com/dknathalage/tallyo/internal/db"
	"github.com/dknathalage/tallyo/internal/db/gen"
	"github.com/stripe/stripe-go/v82/webhook"
)

const testWebhookSecret = "whsec_test_secret"

// postEvent signs payload like Stripe does and runs it through the webhook. The
// signature timestamp must be near now (ConstructEvent enforces a 300s
// tolerance); the payload's own "created" field — which drives idempotency
// ordering — is independent and stays whatever the test JSON sets.
func postEvent(t *testing.T, h *Handler, payload string, _ time.Time, secret string) *httptest.ResponseRecorder {
	t.Helper()
	now := time.Now()
	sig := webhook.ComputeSignature(now, []byte(payload), secret)
	header := fmt.Sprintf("t=%d,v1=%s", now.Unix(), hex.EncodeToString(sig))
	req := httptest.NewRequest(http.MethodPost, "/api/stripe/webhook", bytes.NewReader([]byte(payload)))
	req.Header.Set("Stripe-Signature", header)
	rec := httptest.NewRecorder()
	h.Webhook(rec, req)
	return rec
}

func newWebhookFixture(t *testing.T) (*Handler, *gen.Queries, string) {
	t.Helper()
	conn := appdb.OpenTestDB(t)
	tenant, err := auth.NewTenants(conn).Create(context.Background(), "Acme")
	if err != nil {
		t.Fatalf("create tenant: %v", err)
	}
	h := NewHandler(fakeClient(t), NewStore(conn), auth.NewTenants(conn))
	return h, gen.New(conn), tenant.ID
}

func statusOf(t *testing.T, q *gen.Queries, tenantID string) string {
	t.Helper()
	row, err := q.GetTenant(context.Background(), tenantID)
	if err != nil {
		t.Fatalf("GetTenant: %v", err)
	}
	return row.SubscriptionStatus
}

func TestWebhookBadSignature(t *testing.T) {
	h, q, tenantID := newWebhookFixture(t)
	payload := fmt.Sprintf(`{"id":"evt","type":"checkout.session.completed","created":1719480000,"data":{"object":{"client_reference_id":%q,"customer":{"id":"cus_1"},"subscription":{"id":"sub_1"}}}}`, tenantID)
	rec := postEvent(t, h, payload, time.Unix(1719480000, 0), "whsec_WRONG")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code = %d, want 400", rec.Code)
	}
	if s := statusOf(t, q, tenantID); s != StatusNone {
		t.Errorf("status = %q, want unchanged (none)", s)
	}
}

func TestWebhookCheckoutThenLifecycle(t *testing.T) {
	h, q, tenantID := newWebhookFixture(t)

	// 1. checkout.session.completed → trialing + customer linked.
	cs := fmt.Sprintf(`{"id":"evt1","type":"checkout.session.completed","created":1719480000,"data":{"object":{"client_reference_id":%q,"customer":{"id":"cus_1"},"subscription":{"id":"sub_1"}}}}`, tenantID)
	if rec := postEvent(t, h, cs, time.Unix(1719480000, 0), testWebhookSecret); rec.Code != http.StatusOK {
		t.Fatalf("checkout code = %d, want 200", rec.Code)
	}
	if s := statusOf(t, q, tenantID); s != StatusTrialing {
		t.Fatalf("after checkout status = %q, want trialing", s)
	}

	// 2. subscription.updated active (resolves via the now-linked customer).
	up := `{"id":"evt2","type":"customer.subscription.updated","created":1719490000,"data":{"object":{"id":"sub_1","status":"active","customer":{"id":"cus_1"}}}}`
	if rec := postEvent(t, h, up, time.Unix(1719490000, 0), testWebhookSecret); rec.Code != http.StatusOK {
		t.Fatalf("update code = %d, want 200", rec.Code)
	}
	if s := statusOf(t, q, tenantID); s != StatusActive {
		t.Fatalf("after update status = %q, want active", s)
	}

	// 3. Duplicate of the active event (same timestamp) → no-op, still active.
	if rec := postEvent(t, h, up, time.Unix(1719490000, 0), testWebhookSecret); rec.Code != http.StatusOK {
		t.Fatalf("dup code = %d, want 200", rec.Code)
	}

	// 4. Stale past_due (older timestamp) → must NOT clobber active.
	stale := `{"id":"evt3","type":"customer.subscription.updated","created":1719485000,"data":{"object":{"id":"sub_1","status":"past_due","customer":{"id":"cus_1"}}}}`
	if rec := postEvent(t, h, stale, time.Unix(1719485000, 0), testWebhookSecret); rec.Code != http.StatusOK {
		t.Fatalf("stale code = %d, want 200", rec.Code)
	}
	if s := statusOf(t, q, tenantID); s != StatusActive {
		t.Errorf("after stale status = %q, want active (no clobber)", s)
	}

	// 5. subscription.deleted → canceled.
	del := `{"id":"evt4","type":"customer.subscription.deleted","created":1719500000,"data":{"object":{"id":"sub_1","status":"canceled","customer":{"id":"cus_1"}}}}`
	if rec := postEvent(t, h, del, time.Unix(1719500000, 0), testWebhookSecret); rec.Code != http.StatusOK {
		t.Fatalf("delete code = %d, want 200", rec.Code)
	}
	if s := statusOf(t, q, tenantID); s != StatusCanceled {
		t.Errorf("after delete status = %q, want canceled", s)
	}
}

func TestWebhookSelfHealViaMetadata(t *testing.T) {
	h, q, tenantID := newWebhookFixture(t)
	// subscription.updated arrives before any checkout linked the customer; only
	// the metadata tenant_id can resolve it.
	up := fmt.Sprintf(`{"id":"evt","type":"customer.subscription.updated","created":1719490000,"data":{"object":{"id":"sub_9","status":"active","customer":{"id":"cus_unlinked"},"metadata":{"tenant_id":%q}}}}`, tenantID)
	if rec := postEvent(t, h, up, time.Unix(1719490000, 0), testWebhookSecret); rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200", rec.Code)
	}
	if s := statusOf(t, q, tenantID); s != StatusActive {
		t.Errorf("self-heal status = %q, want active", s)
	}
}

func TestWebhookUnhandledType(t *testing.T) {
	h, q, tenantID := newWebhookFixture(t)
	p := `{"id":"evt","type":"invoice.paid","created":1719490000,"data":{"object":{}}}`
	if rec := postEvent(t, h, p, time.Unix(1719490000, 0), testWebhookSecret); rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200", rec.Code)
	}
	if s := statusOf(t, q, tenantID); s != StatusNone {
		t.Errorf("status = %q, want unchanged (none)", s)
	}
}
