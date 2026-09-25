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

func TestP3BPlayerNodeSelectionAndSafeCancelHTTP(t *testing.T) {
	dsn := os.Getenv("PLATFORM_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("PLATFORM_TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	base, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	id, err := store.NewID()
	if err != nil {
		t.Fatal(err)
	}
	schema := "p3bhttp_" + strings.ReplaceAll(id, "-", "")
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
	if _, err := pool.Exec(ctx, `INSERT INTO game_presets(id,arcade_game_id,display_name,max_players,template_revision_id)
		VALUES($1,$2,'N6',10,'test-template')`, presetID, gameID); err != nil {
		t.Fatal(err)
	}
	nodeID, _, err := s.RegisterNode(ctx, "Manual node", "linux")
	if err != nil {
		t.Fatal(err)
	}
	handler, err := httpapi.NewHandler(s, httpapi.Config{PublicOrigin: testOrigin, Development: true})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	a, _ := newPartyBrowser(t, server.URL)
	b, _ := newPartyBrowser(t, server.URL)
	var nodes []store.NodeChoice
	a.call(t, http.MethodGet, "/api/v1/nodes?arcadeGameId="+gameID+"&gamePresetId="+presetID,
		nil, http.StatusOK, &nodes)
	if len(nodes) != 1 || nodes[0].ID != nodeID || nodes[0].DisplayName != "Manual node" || !nodes[0].Selectable || nodes[0].Status != "unavailable" {
		t.Fatalf("player-safe node choices: %+v", nodes)
	}
	body := map[string]string{"arcadeGameId": gameID, "gamePresetId": presetID,
		"nodeSelectionMode": "manual", "manualNodeId": nodeID}
	var request, duplicate store.ServerRequest
	a.call(t, http.MethodPost, "/api/v1/server-requests", body, http.StatusCreated, &request)
	a.call(t, http.MethodPost, "/api/v1/server-requests", body, http.StatusOK, &duplicate)
	if duplicate.ID != request.ID || request.NodeSelectionMode != "manual" || request.ManualNodeID == nil || *request.ManualNodeID != nodeID {
		t.Fatalf("manual intent not durable: %+v %+v", request, duplicate)
	}
	b.call(t, http.MethodPost, "/api/v1/server-requests/"+request.ID+"/cancel", nil, http.StatusNotFound, nil)
	var cancelled store.ServerRequest
	a.call(t, http.MethodPost, "/api/v1/server-requests/"+request.ID+"/cancel", nil, http.StatusOK, &cancelled)
	if cancelled.State != "cancelled" {
		t.Fatalf("cancel state: %+v", cancelled)
	}
	a.call(t, http.MethodPost, "/api/v1/server-requests/"+request.ID+"/cancel", nil, http.StatusConflict, nil)
	var replacement store.ServerRequest
	a.call(t, http.MethodPost, "/api/v1/server-requests", map[string]string{
		"arcadeGameId": gameID, "gamePresetId": presetID}, http.StatusCreated, &replacement)
	if replacement.ID == request.ID || replacement.NodeSelectionMode != "auto" || !replacement.RequestedAt.After(request.RequestedAt) {
		t.Fatalf("replacement kept old queue position: %+v", replacement)
	}
}
