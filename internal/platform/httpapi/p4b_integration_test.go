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

func TestP4BAbandonHTTPConfirmationAndOwner(t *testing.T) {
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
	schema := "p4bhttp_" + strings.ReplaceAll(id, "-", "")
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
	gameID, _ := store.NewID()
	presetID, _ := store.NewID()
	if _, err := pool.Exec(ctx, `INSERT INTO arcade_games(id,workshop_id,display_name) VALUES($1,'3564393242','Test game')`, gameID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO template_revisions(id,arcade_game_id) VALUES('test-template',$1)`, gameID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO content_versions(id,arcade_game_id,content_sha256)
		VALUES('test-v1',$1,$2)`, gameID, strings.Repeat("a", 64)); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE arcade_games SET current_content_version_id='test-v1' WHERE id=$1`, gameID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO game_presets(id,arcade_game_id,display_name,max_players,template_revision_id)
		VALUES($1,$2,'N6',10,'test-template')`, presetID, gameID); err != nil {
		t.Fatal(err)
	}
	nodeID, _, err := s.RegisterNode(ctx, "Test node", "linux")
	if err != nil {
		t.Fatal(err)
	}
	handler, err := httpapi.NewHandler(s, httpapi.Config{PublicOrigin: testOrigin, Development: true})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()
	owner, ownerID := newPartyBrowser(t, server.URL)
	other, _ := newPartyBrowser(t, server.URL)
	r, _, err := s.CreateUserRequest(ctx, ownerID, gameID, presetID)
	if err != nil {
		t.Fatal(err)
	}
	allocationID, _ := store.NewID()
	if _, err := pool.Exec(ctx, `INSERT INTO allocations(id,server_request_id,arcade_game_id,attempt_sequence,node_id,
		content_version_id,template_revision_id,state,assigned_at)
		VALUES($1,$2,$3,1,$4,'test-v1','test-template','quarantined',now())`, allocationID, r.ID, gameID, nodeID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE server_requests SET state='quarantined' WHERE id=$1`, r.ID); err != nil {
		t.Fatal(err)
	}
	path := "/api/v1/server-requests/" + r.ID + "/abandon"
	owner.call(t, http.MethodPost, path, map[string]bool{"confirm": false}, http.StatusBadRequest, nil)
	other.call(t, http.MethodPost, path, map[string]bool{"confirm": true}, http.StatusNotFound, nil)
	var abandoned store.ServerRequest
	owner.call(t, http.MethodPost, path, map[string]bool{"confirm": true}, http.StatusOK, &abandoned)
	if abandoned.State != "abandoned" {
		t.Fatalf("abandon result: %+v", abandoned)
	}
	owner.call(t, http.MethodPost, path, map[string]bool{"confirm": true}, http.StatusOK, &abandoned)
	var occupied int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM allocations WHERE state='quarantined'`).Scan(&occupied); err != nil || occupied != 1 {
		t.Fatalf("abandon released capacity: %d %v", occupied, err)
	}
}
