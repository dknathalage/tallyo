package app

import (
	"encoding/json"
	"github.com/dknathalage/tallyo/internal/httpx"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dknathalage/tallyo/internal/auth"
	"github.com/go-chi/chi/v5"
)

// inviteeToken is the bearer token for the invited person. They sign in with
// Firebase first (creating their identity); the stub maps this token to a fresh
// uid/email distinct from the seeded members.
const inviteeToken = "invitee-token"

// newInviteServer spins up a real httptest.Server with the invite routes wired
// the same way production wires them: Create is tenant-scoped + owner/admin only;
// Validate is public; Accept is Bearer-authed (the invitee has signed in). It
// returns the server and the users repo so tests can assert on created members.
func newInviteServer(t *testing.T) (*httptest.Server, *auth.UsersRepo, string) {
	t.Helper()
	conn := openMigratedDB(t, "i.db")
	users, tenantID, _, tenantUUID := seedTenantOwner(t, conn)
	if _, err := users.Create(t.Context(), tenantID, "member@x.com", "uid-member", "", "member", false); err != nil {
		t.Fatalf("Create member: %v", err)
	}

	v := newStubVerifier()
	v.add(inviteeToken, auth.Token{UID: "uid-new", Email: "new@x.com", Name: "Token Name"})
	tenants := auth.NewTenants(conn)
	invH := NewInviteHandler(auth.NewInvites(conn), users)

	router := chi.NewRouter()
	router.Route("/api", func(api chi.Router) {
		// Public invite validation: the invitee may not have an account yet.
		api.Get("/invites/{token}", invH.Validate)
		// Bearer-authed invite acceptance (needs the uid).
		api.Group(func(pr chi.Router) {
			pr.Use(httpx.RequireAuth(v))
			pr.Post("/invites/{token}/accept", invH.Accept)
		})
		// Invite create is tenant-scoped + owner/admin only (matches production).
		api.Route("/t/{tenantUUID}", func(pr chi.Router) {
			pr.Use(httpx.RequireAuth(v))
			pr.Use(httpx.ResolveTenant(users, tenants, false))
			pr.With(httpx.RequireRole("owner", "admin")).Post("/invites", invH.Create)
		})
	})

	srv := httptest.NewServer(router)
	t.Cleanup(srv.Close)
	return srv, users, tenantUUID
}

// postJSON posts a JSON body with the given client and returns the response.
func postJSON(t *testing.T, c *http.Client, url, body string) *http.Response {
	t.Helper()
	req, err := http.NewRequest("POST", url, strings.NewReader(body))
	if err != nil {
		t.Fatalf("new req %s: %v", url, err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.Do(req)
	if err != nil {
		t.Fatalf("do %s: %v", url, err)
	}
	return resp
}

// createInvite logs the owner in and creates an invite, returning its token.
func createInvite(t *testing.T, srv *httptest.Server, uuid string) string {
	t.Helper()
	owner := loggedInClient(t, srv.URL)
	resp := postJSON(t, owner, srv.URL+"/api/t/"+uuid+"/invites", `{"email":"new@x.com","role":"member"}`)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create invite: want 201 got %d", resp.StatusCode)
	}
	var out struct {
		Token     string `json:"token"`
		AcceptURL string `json:"acceptUrl"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode invite: %v", err)
	}
	if out.Token == "" {
		t.Fatalf("create invite: empty token")
	}
	if !strings.Contains(out.AcceptURL, "/accept-invite?token=") {
		t.Fatalf("create invite: acceptUrl missing path: %q", out.AcceptURL)
	}
	return out.Token
}

func TestInviteCreateOwner201(t *testing.T) {
	srv, _, uuid := newInviteServer(t)
	_ = createInvite(t, srv, uuid)
}

func TestInviteCreateMemberForbidden(t *testing.T) {
	srv, _, uuid := newInviteServer(t)
	c := bearerClient(memberToken)
	r2 := postJSON(t, c, srv.URL+"/api/t/"+uuid+"/invites", `{"email":"new@x.com","role":"member"}`)
	defer func() { _ = r2.Body.Close() }()
	if r2.StatusCode != http.StatusForbidden {
		t.Fatalf("member create: want 403 got %d", r2.StatusCode)
	}
}

func TestInviteCreateUnauthenticated(t *testing.T) {
	srv, _, uuid := newInviteServer(t)
	c := jarClient(t)
	resp := postJSON(t, c, srv.URL+"/api/t/"+uuid+"/invites", `{"email":"new@x.com","role":"member"}`)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("anon create: want 401 got %d", resp.StatusCode)
	}
}

func TestInviteValidateOK(t *testing.T) {
	srv, _, uuid := newInviteServer(t)
	token := createInvite(t, srv, uuid)
	c := jarClient(t)
	resp := get(t, c, srv.URL+"/api/invites/"+token)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("validate: want 200 got %d", resp.StatusCode)
	}
	var out map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode validate: %v", err)
	}
	if out["email"] != "new@x.com" {
		t.Fatalf("validate email wrong: %v", out)
	}
	if _, leaked := out["token"]; leaked {
		t.Fatalf("validate leaked token: %v", out)
	}
}

func TestInviteValidateBadToken404(t *testing.T) {
	srv, _, _ := newInviteServer(t)
	c := jarClient(t)
	resp := get(t, c, srv.URL+"/api/invites/not-a-real-token")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("bad token: want 404 got %d", resp.StatusCode)
	}
}

func TestInviteAcceptCreatesMember(t *testing.T) {
	srv, users, uuid := newInviteServer(t)
	token := createInvite(t, srv, uuid)

	c := bearerClient(inviteeToken)
	resp := postJSON(t, c, srv.URL+"/api/invites/"+token+"/accept", `{"name":"New Member"}`)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("accept: want 201 got %d", resp.StatusCode)
	}

	// A member user must now exist in the invite's tenant, linked to the token's
	// uid, with the form's name.
	u, err := users.GetByFirebaseUID(t.Context(), uuid, "uid-new")
	if err != nil || u == nil {
		t.Fatalf("GetByFirebaseUID: %+v err=%v", u, err)
	}
	if u.Email != "new@x.com" {
		t.Fatalf("accept: email wrong, got %q", u.Email)
	}
	if u.Name != "New Member" {
		t.Fatalf("accept: display name not set, got %q", u.Name)
	}
	if u.Role != "member" {
		t.Fatalf("accept: role wrong, got %q", u.Role)
	}
}

func TestInviteAcceptUnauthenticated401(t *testing.T) {
	srv, _, uuid := newInviteServer(t)
	token := createInvite(t, srv, uuid)
	c := jarClient(t) // no bearer token
	resp := postJSON(t, c, srv.URL+"/api/invites/"+token+"/accept", `{"name":"New Member"}`)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("anon accept: want 401 got %d", resp.StatusCode)
	}
}

func TestInviteAcceptTwiceConflict(t *testing.T) {
	srv, _, uuid := newInviteServer(t)
	token := createInvite(t, srv, uuid)

	c := bearerClient(inviteeToken)
	r1 := postJSON(t, c, srv.URL+"/api/invites/"+token+"/accept", `{"name":"New Member"}`)
	_ = r1.Body.Close()
	if r1.StatusCode != http.StatusCreated {
		t.Fatalf("first accept: want 201 got %d", r1.StatusCode)
	}

	r2 := postJSON(t, c, srv.URL+"/api/invites/"+token+"/accept", `{"name":"New Member"}`)
	_ = r2.Body.Close()
	if r2.StatusCode != http.StatusConflict {
		t.Fatalf("second accept: want 409 got %d", r2.StatusCode)
	}
}
