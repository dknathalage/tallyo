package auth

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/dknathalage/tallyo/internal/ids"
)

// TestUserJSONExposesUUIDsNotInts is the Task 2.8 guarantee for the /auth/me +
// login user payload: the serialized "id" is the user uuid (string) and
// "tenantId" is the tenant uuid (string). The int PKs never appear in the JSON.
func TestUserJSONExposesUUIDsNotInts(t *testing.T) {
	u := &User{
		ID:          ids.New(),
		TenantID:    ids.New(),
		Email:       "owner@x.com",
		Name:        "Owner",
		Role:        "owner",
		FirebaseUID: "uid-secret",
	}
	raw, err := json.Marshal(u)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var generic map[string]any
	if err := json.Unmarshal(raw, &generic); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	assertStringField(t, generic, "id", u.ID, raw)
	assertStringField(t, generic, "tenantId", u.TenantID, raw)
	// The Firebase uid is server-side only and must never be serialized.
	if _, leaked := generic["firebaseUid"]; leaked {
		t.Fatalf("firebaseUid leaked: %s", raw)
	}
	if _, leaked := generic["firebase_uid"]; leaked {
		t.Fatalf("firebase_uid leaked: %s", raw)
	}
	// No JSON value may be a number — every id is a uuid string.
	for k, v := range generic {
		if _, ok := v.(float64); ok {
			t.Fatalf("numeric value leaked under %q (=%v): %s", k, v, raw)
		}
	}
}

// TestEmailTenantJSONExposesUUIDNotInt is the Task 2.8 guarantee for the
// login-409 multi-tenant body and /auth/session: the tenant is identified by its
// uuid (serialized as "id"); no integer tenant id appears in the JSON.
func TestEmailTenantJSONExposesUUIDNotInt(t *testing.T) {
	et := EmailTenant{
		TenantID:   ids.New(),
		TenantUUID: ids.New(),
		TenantName: "Acme",
		Role:       "owner",
	}
	raw, err := json.Marshal(et)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var generic map[string]any
	if err := json.Unmarshal(raw, &generic); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	assertStringField(t, generic, "id", et.TenantUUID, raw)
	if _, leaked := generic["tenantId"]; leaked {
		t.Fatalf("tenantId leaked: %s", raw)
	}
	for k, v := range generic {
		if _, ok := v.(float64); ok {
			t.Fatalf("numeric value leaked under %q (=%v): %s", k, v, raw)
		}
	}
}

// TestGetByIDPopulatesTenantUUID confirms the repo stamps the public tenant uuid
// onto the User it returns (so /auth/me reports the tenant by uuid, not int PK).
func TestGetByIDPopulatesTenantUUID(t *testing.T) {
	conn := mustUserDB(t)
	defer conn.Close()
	tid := seedTenant(t, conn, "T")
	repo := NewUsers(conn)
	ctx := context.Background()

	created, err := repo.Create(ctx, tid, "owner@x.com", "uid-owner", "Owner", "owner", false)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.TenantID == "" {
		t.Fatal("Create did not populate TenantID")
	}
	got, err := repo.GetByID(ctx, tid, created.ID)
	if err != nil || got == nil {
		t.Fatalf("GetByID %+v err=%v", got, err)
	}
	if got.TenantID == "" || got.TenantID != created.TenantID {
		t.Fatalf("GetByID TenantID=%q, want %q", got.TenantID, created.TenantID)
	}
	// The serialized "tenantId" must be the tenant uuid.
	raw, _ := json.Marshal(got)
	var generic map[string]any
	_ = json.Unmarshal(raw, &generic)
	assertStringField(t, generic, "tenantId", got.TenantID, raw)
}

func assertStringField(t *testing.T, m map[string]any, key, want string, raw []byte) {
	t.Helper()
	v, ok := m[key]
	if !ok {
		t.Fatalf("missing %q in %s", key, raw)
	}
	s, isString := v.(string)
	if !isString {
		t.Fatalf("%q is not a string (leaked int?): %T %v in %s", key, v, v, raw)
	}
	if s != want {
		t.Fatalf("%q = %q, want %q", key, s, want)
	}
}
