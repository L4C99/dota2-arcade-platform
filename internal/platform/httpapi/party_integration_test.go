package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/L4C99/dota2-arcade-platform/internal/platform/httpapi"
	"github.com/L4C99/dota2-arcade-platform/internal/platform/store"
	"github.com/jackc/pgx/v5/pgxpool"
)

const testOrigin = "http://127.0.0.1:8080"

type partyBrowser struct {
	client *http.Client
	base   string
}

func (b partyBrowser) call(t *testing.T, method, path string, input any, want int, out any) {
	t.Helper()
	var body io.Reader
	if input != nil {
		data, err := json.Marshal(input)
		if err != nil {
			t.Fatal(err)
		}
		body = bytes.NewReader(data)
	}
	req, err := http.NewRequest(method, b.base+path, body)
	if err != nil {
		t.Fatal(err)
	}
	if method != http.MethodGet {
		req.Header.Set("Origin", testOrigin)
	}
	if input != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := b.client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != want {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 128))
		t.Fatalf("%s %s status=%d want=%d body=%q", method, path, resp.StatusCode, want, data)
	}
	if out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			t.Fatal(err)
		}
	}
}

func newPartyBrowser(t *testing.T, base string) (partyBrowser, string) {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	b := partyBrowser{client: &http.Client{Jar: jar}, base: base}
	var session struct {
		UserID string `json:"userId"`
	}
	b.call(t, http.MethodPost, "/api/v1/session", nil, http.StatusCreated, &session)
	if session.UserID == "" {
		t.Fatal("session did not create User")
	}
	return b, session.UserID
}

func TestP2DPartyHTTPAuthorizationWithIndependentSessions(t *testing.T) {
	dsn := os.Getenv("PLATFORM_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("PLATFORM_TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	base, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer base.Close()
	id, err := store.NewID()
	if err != nil {
		t.Fatal(err)
	}
	schema := "p2http_" + strings.ReplaceAll(id, "-", "")
	if _, err := base.Exec(ctx, `CREATE SCHEMA "`+schema+`"`); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if _, err := base.Exec(ctx, `DROP SCHEMA "`+schema+`" CASCADE`); err != nil {
			t.Errorf("cleanup: %v", err)
		}
	}()
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
	defer s.Close()
	if err := s.ApplyMigrations(ctx); err != nil {
		t.Fatal(err)
	}
	if err := s.ConfigurePartySize(ctx, 3); err != nil {
		t.Fatal(err)
	}
	gameID, err := store.NewID()
	if err != nil {
		t.Fatal(err)
	}
	presetID, err := store.NewID()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Pool.Exec(ctx, `INSERT INTO arcade_games(id,workshop_id,display_name) VALUES($1,'3564393242','Test game')`, gameID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Pool.Exec(ctx, `INSERT INTO template_revisions(id,arcade_game_id) VALUES('test-template',$1)`, gameID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Pool.Exec(ctx, `INSERT INTO game_presets(id,arcade_game_id,display_name,max_players,template_revision_id)
		VALUES($1,$2,'N6',10,'test-template')`, presetID, gameID); err != nil {
		t.Fatal(err)
	}
	handler, err := httpapi.NewHandler(s, httpapi.Config{PublicOrigin: testOrigin, Development: true})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()
	a, aID := newPartyBrowser(t, server.URL)
	b, bID := newPartyBrowser(t, server.URL)
	c, _ := newPartyBrowser(t, server.URL)
	var partyA, partyB store.Party
	a.call(t, http.MethodPost, "/api/v1/party", nil, http.StatusCreated, &partyA)
	c.call(t, http.MethodPost, "/api/v1/party", nil, http.StatusCreated, &partyB)
	if partyA.LeaderUserID != aID || partyA.CurrentRole != "leader" || partyB.ID == partyA.ID {
		t.Fatal("session-derived leaders missing")
	}
	var invite store.PartyInvite
	a.call(t, http.MethodGet, "/api/v1/party/invite", nil, http.StatusOK, &invite)
	b.call(t, http.MethodPost, "/api/v1/party/join", map[string]string{"token": invite.Token}, http.StatusOK, &store.Party{})
	b.call(t, http.MethodGet, "/api/v1/party/invite", nil, http.StatusForbidden, nil)
	requestBody := map[string]string{"arcadeGameId": gameID, "gamePresetId": presetID}
	b.call(t, http.MethodPost, "/api/v1/server-requests", requestBody, http.StatusForbidden, nil)
	forged := map[string]string{"arcadeGameId": gameID, "gamePresetId": presetID, "partyId": partyA.ID, "role": "leader", "userId": aID}
	b.call(t, http.MethodPost, "/api/v1/server-requests", forged, http.StatusBadRequest, nil)
	b.call(t, http.MethodPost, "/api/v1/party/invite/reset", nil, http.StatusForbidden, nil)
	b.call(t, http.MethodPost, "/api/v1/party/members/"+aID+"/remove", nil, http.StatusForbidden, nil)
	b.call(t, http.MethodPost, "/api/v1/party/disband", nil, http.StatusForbidden, nil)
	c.call(t, http.MethodPost, "/api/v1/party/members/"+bID+"/remove", nil, http.StatusForbidden, nil)
	var request store.ServerRequest
	a.call(t, http.MethodPost, "/api/v1/server-requests", requestBody, http.StatusCreated, &request)
	var shared store.ServerRequest
	b.call(t, http.MethodGet, "/api/v1/server-requests/current", nil, http.StatusOK, &shared)
	if shared.ID != request.ID {
		t.Fatalf("member request differs: %s %s", shared.ID, request.ID)
	}
	c.call(t, http.MethodGet, "/api/v1/server-requests/"+request.ID, nil, http.StatusNotFound, nil)
	c.call(t, http.MethodGet, "/api/v1/server-requests/"+request.ID+"/allocation", nil, http.StatusNotFound, nil)
	b.call(t, http.MethodPost, "/api/v1/server-requests/"+request.ID+"/stop", nil, http.StatusForbidden, nil)
	c.call(t, http.MethodPost, "/api/v1/server-requests/"+request.ID+"/stop", nil, http.StatusNotFound, nil)
	a.call(t, http.MethodPost, "/api/v1/party/disband", nil, http.StatusConflict, nil)
	a.call(t, http.MethodPost, "/api/v1/party/leave", nil, http.StatusForbidden, nil)
	b.call(t, http.MethodPost, "/api/v1/party/leave", nil, http.StatusNoContent, nil)
	b.call(t, http.MethodGet, "/api/v1/server-requests/"+request.ID, nil, http.StatusNotFound, nil)
	b.call(t, http.MethodPost, "/api/v1/server-requests/"+request.ID+"/stop", nil, http.StatusNotFound, nil)
	b.call(t, http.MethodPost, "/api/v1/party/invite/reset", nil, http.StatusForbidden, nil)
	var newInvite store.PartyInvite
	a.call(t, http.MethodPost, "/api/v1/party/invite/reset", nil, http.StatusOK, &newInvite)
	d, _ := newPartyBrowser(t, server.URL)
	d.call(t, http.MethodPost, "/api/v1/party/join", map[string]string{"token": invite.Token}, http.StatusNotFound, nil)
	if newInvite.Token == invite.Token {
		t.Fatal("reset retained old token")
	}
	var ownerID string
	if err := s.Pool.QueryRow(ctx, `SELECT owner_party_id FROM server_requests WHERE id=$1`, request.ID).Scan(&ownerID); err != nil || ownerID != partyA.ID {
		t.Fatalf("owner changed: %s %v", ownerID, err)
	}
}
