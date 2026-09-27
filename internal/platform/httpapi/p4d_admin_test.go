package httpapi_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/L4C99/dota2-arcade-platform/internal/platform/httpapi"
	"github.com/L4C99/dota2-arcade-platform/internal/platform/store"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestP4DAdminHTTPAuthorizationSessionAndCSRF(t *testing.T) {
	dsn := os.Getenv("PLATFORM_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("PLATFORM_TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	base, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := store.NewID()
	schema := "p4dhttp_" + strings.ReplaceAll(id, "-", "")
	if _, err := base.Exec(ctx, `CREATE SCHEMA "`+schema+`"`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := base.Exec(ctx, `DROP SCHEMA "`+schema+`" CASCADE`); err != nil {
			t.Errorf("cleanup schema: %v", err)
		}
		base.Close()
	})
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	config.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	s := &store.Store{Pool: pool}
	t.Cleanup(s.Close)
	if err := s.ApplyMigrations(ctx); err != nil {
		t.Fatal(err)
	}
	adminID, err := s.CreateAdmin(ctx, "p4d-http", "a long test password")
	if err != nil {
		t.Fatal(err)
	}
	handler, err := httpapi.NewHandler(s, httpapi.Config{PublicOrigin: "https://example.org"})
	if err != nil {
		t.Fatal(err)
	}
	call := func(method, path, body, origin string, cookie *http.Cookie) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.RemoteAddr = "198.51.100.23:43210"
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		if body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		if cookie != nil {
			req.AddCookie(cookie)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		return w
	}
	if got := call("GET", "/api/v1/admin/overview", "", "", nil).Code; got != http.StatusUnauthorized {
		t.Fatalf("anonymous overview: %d", got)
	}
	login := call("POST", "/api/v1/admin/login", `{"username":"p4d-http","password":"a long test password"}`, "https://example.org", nil)
	if login.Code != http.StatusOK {
		t.Fatalf("login: %d %s", login.Code, login.Body.String())
	}
	cookies := login.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("cookies=%d", len(cookies))
	}
	cookie := cookies[0]
	if cookie.Name != "__Host-arcade_admin" || !cookie.Secure || !cookie.HttpOnly || cookie.SameSite != http.SameSiteLaxMode || cookie.MaxAge <= 0 {
		t.Fatalf("cookie flags: %+v", cookie)
	}
	if got := call("GET", "/api/v1/admin/overview", "", "", cookie).Code; got != http.StatusOK {
		t.Fatalf("admin overview: %d", got)
	}
	if got := call("POST", "/api/v1/admin/actions", `{"action":"entry.update","verifiedPorts":[28000]}`, "https://example.org", cookie).Code; got != http.StatusBadRequest {
		t.Fatalf("obsolete verifiedPorts API field accepted: %d", got)
	}
	action := `{"action":"global.update","accepting":false}`
	if got := call("POST", "/api/v1/admin/actions", action, "https://other.example", cookie).Code; got != http.StatusForbidden {
		t.Fatalf("cross origin action: %d", got)
	}
	if got := call("POST", "/api/v1/admin/actions", action, "https://example.org", nil).Code; got != http.StatusUnauthorized {
		t.Fatalf("unauthenticated action: %d", got)
	}
	if got := call("POST", "/api/v1/admin/actions", action, "https://example.org", cookie).Code; got != http.StatusOK {
		t.Fatalf("authorized action: %d", got)
	}
	var actor string
	if err := pool.QueryRow(ctx, `SELECT actor_admin_user_id FROM audit_events WHERE action='global.update'`).Scan(&actor); err != nil || actor != adminID {
		t.Fatalf("audit actor=%s: %v", actor, err)
	}
	// A.4: revocation failure must preserve the cookie/session and roll back
	// the replacement session, rather than reporting a false logout/rotation.
	_, userToken, err := s.CreateUserSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `CREATE FUNCTION reject_revoke() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'injected revoke failure'; END $$;
		CREATE TRIGGER reject_admin_revoke BEFORE UPDATE ON admin_sessions FOR EACH ROW EXECUTE FUNCTION reject_revoke();
		CREATE TRIGGER reject_user_revoke BEFORE UPDATE ON user_sessions FOR EACH ROW EXECUTE FUNCTION reject_revoke();`); err != nil {
		t.Fatal(err)
	}
	for _, target := range []struct {
		path   string
		cookie *http.Cookie
	}{
		{"/api/v1/admin/logout", cookie}, {"/api/v1/session/logout", &http.Cookie{Name: "__Host-arcade_session", Value: userToken}},
	} {
		response := call("POST", target.path, "", "https://example.org", target.cookie)
		if response.Code != http.StatusServiceUnavailable || len(response.Result().Cookies()) != 0 {
			t.Fatalf("failed logout: %d %v", response.Code, response.Result().Cookies())
		}
	}
	failedRotation := call("POST", "/api/v1/admin/login", `{"username":"p4d-http","password":"a long test password"}`, "https://example.org", cookie)
	var sessions int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM admin_sessions`).Scan(&sessions); err != nil {
		t.Fatal(err)
	}
	if failedRotation.Code != http.StatusServiceUnavailable || len(failedRotation.Result().Cookies()) != 0 || sessions != 1 {
		t.Fatalf("rotation failed nonatomically: %d sessions=%d", failedRotation.Code, sessions)
	}
	if call("GET", "/api/v1/admin/me", "", "", cookie).Code != http.StatusOK {
		t.Fatal("failed revoke invalidated old session")
	}
	if _, err := pool.Exec(ctx, `DROP TRIGGER reject_admin_revoke ON admin_sessions; DROP TRIGGER reject_user_revoke ON user_sessions; DROP FUNCTION reject_revoke()`); err != nil {
		t.Fatal(err)
	}
	rotation := call("POST", "/api/v1/admin/login", `{"username":"p4d-http","password":"a long test password"}`, "https://example.org", cookie)
	if rotation.Code != http.StatusOK || len(rotation.Result().Cookies()) != 1 {
		t.Fatalf("session rotation: %d", rotation.Code)
	}
	newCookie := rotation.Result().Cookies()[0]
	if cookie.Value == newCookie.Value || call("GET", "/api/v1/admin/me", "", "", cookie).Code != http.StatusUnauthorized {
		t.Fatal("old admin session survived rotation")
	}
	if got := call("POST", "/api/v1/admin/logout", "", "https://example.org", newCookie).Code; got != http.StatusNoContent {
		t.Fatalf("logout: %d", got)
	}
	if got := call("GET", "/api/v1/admin/me", "", "", newCookie).Code; got != http.StatusUnauthorized {
		t.Fatalf("logged out session: %d", got)
	}
	if _, err := pool.Exec(ctx, `UPDATE admin_users SET enabled=false WHERE id=$1`, adminID); err != nil {
		t.Fatal(err)
	}
	if got := call("POST", "/api/v1/admin/login", `{"username":"p4d-http","password":"a long test password"}`, "https://example.org", nil).Code; got != http.StatusUnauthorized {
		t.Fatalf("disabled admin login: %d", got)
	}
	for i := 0; i < 3; i++ {
		if got := call("POST", "/api/v1/admin/login", `{"username":"missing-p4d","password":"incorrect long password"}`, "https://example.org", nil).Code; got != http.StatusUnauthorized {
			t.Fatalf("failed login %d: %d", i, got)
		}
	}
	if got := call("POST", "/api/v1/admin/login", `{"username":"missing-p4d","password":"incorrect long password"}`, "https://example.org", nil).Code; got != http.StatusTooManyRequests {
		t.Fatalf("IP+username throttle: %d", got)
	}
}
