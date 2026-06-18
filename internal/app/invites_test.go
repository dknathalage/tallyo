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

// newInviteServer spins up a real httptest.Server with the invite routes wired
// the same way production wires them: Create behind httpx.RequireAuth, Validate and
// Accept public (the invitee is not logged in). It returns the server and the
// users repo so tests can assert on created members.
func newInviteServer(t *testing.T) (*httptest.Server, *auth.UsersRepo) {
	t.Helper()
	conn := openMigratedDB(t, "i.db")
	users, tenantID, _ := seedTenantOwner(t, conn)
	hash, err := auth.HashPassword("password1")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if _, err := users.Create(t.Context(), tenantID, "m@x.com", hash, "", "member", false); err != nil {
		t.Fatalf("Create member: %v", err)
	}

	sm := auth.NewSessionManager(conn, false)
	authH := NewAuthHandler(sm, users, auth.NewTenants(conn))
	invH := NewInviteHandler(auth.NewInvites(conn), users)

	router := chi.NewRouter()
	router.Route("/api", func(api chi.Router) {
		api.Post("/auth/login", authH.Login)
		api.Get("/invites/{token}", invH.Validate)
		api.Post("/invites/{token}/accept", invH.Accept)
		api.Group(func(pr chi.Router) {
			pr.Use(httpx.RequireAuth(sm, users, auth.NewTenants(conn)))
			pr.Post("/invites", invH.Create)
		})
	})

	srv := httptest.NewServer(sm.LoadAndSave(router))
	t.Cleanup(srv.Close)
	return srv, users
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
func createInvite(t *testing.T, srv *httptest.Server) string {
	t.Helper()
	owner := loggedInClient(t, srv.URL)
	resp := postJSON(t, owner, srv.URL+"/api/invites", `{"email":"new@x.com","role":"member"}`)
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
	srv, _ := newInviteServer(t)
	_ = createInvite(t, srv)
}

func TestInviteCreateMemberForbidden(t *testing.T) {
	srv, _ := newInviteServer(t)
	c := jarClient(t)
	resp := login(t, c, srv.URL, "m@x.com", "password1")
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("member login: want 200 got %d", resp.StatusCode)
	}
	r2 := postJSON(t, c, srv.URL+"/api/invites", `{"email":"new@x.com","role":"member"}`)
	defer func() { _ = r2.Body.Close() }()
	if r2.StatusCode != http.StatusForbidden {
		t.Fatalf("member create: want 403 got %d", r2.StatusCode)
	}
}

func TestInviteCreateUnauthenticated(t *testing.T) {
	srv, _ := newInviteServer(t)
	c := jarClient(t)
	resp := postJSON(t, c, srv.URL+"/api/invites", `{"email":"new@x.com","role":"member"}`)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("anon create: want 401 got %d", resp.StatusCode)
	}
}

func TestInviteValidateOK(t *testing.T) {
	srv, _ := newInviteServer(t)
	token := createInvite(t, srv)
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
	srv, _ := newInviteServer(t)
	c := jarClient(t)
	resp := get(t, c, srv.URL+"/api/invites/not-a-real-token")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("bad token: want 404 got %d", resp.StatusCode)
	}
}

func TestInviteAcceptCreatesMemberThenLogin(t *testing.T) {
	srv, users := newInviteServer(t)
	token := createInvite(t, srv)

	c := jarClient(t)
	resp := postJSON(t, c, srv.URL+"/api/invites/"+token+"/accept", `{"name":"New Member","password":"password1"}`)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("accept: want 201 got %d", resp.StatusCode)
	}

	// A member user must now exist in the invite's tenant, with the form's name.
	creds, found, err := users.GetCredentialsGlobal(t.Context(), "new@x.com")
	if err != nil {
		t.Fatalf("GetCredentialsGlobal: %v", err)
	}
	if !found {
		t.Fatalf("accept: member user not created")
	}
	u, err := users.GetByID(t.Context(), creds.TenantID, creds.ID)
	if err != nil || u == nil {
		t.Fatalf("GetByID: %+v err=%v", u, err)
	}
	if u.Name != "New Member" {
		t.Fatalf("accept: display name not set, got %q", u.Name)
	}
	if u.Role != "member" {
		t.Fatalf("accept: role wrong, got %q", u.Role)
	}

	// And the new member can log in.
	c2 := jarClient(t)
	r2 := login(t, c2, srv.URL, "new@x.com", "password1")
	_ = r2.Body.Close()
	if r2.StatusCode != http.StatusOK {
		t.Fatalf("new member login: want 200 got %d", r2.StatusCode)
	}
}

func TestInviteAcceptTwiceConflict(t *testing.T) {
	srv, _ := newInviteServer(t)
	token := createInvite(t, srv)

	c := jarClient(t)
	r1 := postJSON(t, c, srv.URL+"/api/invites/"+token+"/accept", `{"name":"New Member","password":"password1"}`)
	_ = r1.Body.Close()
	if r1.StatusCode != http.StatusCreated {
		t.Fatalf("first accept: want 201 got %d", r1.StatusCode)
	}

	r2 := postJSON(t, c, srv.URL+"/api/invites/"+token+"/accept", `{"name":"New Member","password":"password1"}`)
	_ = r2.Body.Close()
	if r2.StatusCode != http.StatusConflict {
		t.Fatalf("second accept: want 409 got %d", r2.StatusCode)
	}
}

func TestInviteAcceptShortPassword400(t *testing.T) {
	srv, _ := newInviteServer(t)
	token := createInvite(t, srv)
	c := jarClient(t)
	resp := postJSON(t, c, srv.URL+"/api/invites/"+token+"/accept", `{"password":"short"}`)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("short password: want 400 got %d", resp.StatusCode)
	}
}
