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
		ID:         42,
		UUID:       ids.New(),
		TenantID:   7,
		TenantUUID: ids.New(),
		Email:      "owner@x.com",
		Name:       "Owner",
		Role:       "owner",
	}
	raw, err := json.Marshal(u)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var generic map[string]any
	if err := json.Unmarshal(raw, &generic); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	assertStringField(t, generic, "id", u.UUID, raw)
	assertStringField(t, generic, "tenantId", u.TenantUUID, raw)
	// The int PKs must not leak under any key.
	for k, v := range generic {
		if f, ok := v.(float64); ok && (f == 42 || f == 7) {
			t.Fatalf("int PK leaked under %q (=%v): %s", k, v, raw)
		}
	}
}

// TestEmailTenantJSONExposesUUIDNotInt is the Task 2.8 guarantee for the
// login-409 multi-tenant body and /auth/session: the tenant is identified by its
// uuid (serialized as "id"); no integer tenant id appears in the JSON.
func TestEmailTenantJSONExposesUUIDNotInt(t *testing.T) {
	et := EmailTenant{
		TenantID:   9,
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
		t.Fatalf("int tenantId leaked: %s", raw)
	}
	for k, v := range generic {
		if f, ok := v.(float64); ok && f == 9 {
			t.Fatalf("int tenant PK leaked under %q (=%v): %s", k, v, raw)
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

	hash, _ := HashPassword("pw123456")
	created, err := repo.Create(ctx, tid, "owner@x.com", hash, "Owner", "owner", false)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.TenantUUID == "" {
		t.Fatal("Create did not populate TenantUUID")
	}
	got, err := repo.GetByID(ctx, tid, created.ID)
	if err != nil || got == nil {
		t.Fatalf("GetByID %+v err=%v", got, err)
	}
	if got.TenantUUID == "" || got.TenantUUID != created.TenantUUID {
		t.Fatalf("GetByID TenantUUID=%q, want %q", got.TenantUUID, created.TenantUUID)
	}
	// The serialized "tenantId" must be the tenant uuid.
	raw, _ := json.Marshal(got)
	var generic map[string]any
	_ = json.Unmarshal(raw, &generic)
	assertStringField(t, generic, "tenantId", got.TenantUUID, raw)
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
