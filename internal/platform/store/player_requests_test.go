package store

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func playerTestStore(t *testing.T) *Store {
	t.Helper()
	dsn := os.Getenv("PLATFORM_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("PLATFORM_TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	base, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	id, err := NewID()
	if err != nil {
		t.Fatal(err)
	}
	schema := "p1a_" + strings.ReplaceAll(id, "-", "")
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
	s := &Store{Pool: pool}
	t.Cleanup(s.Close)
	if err := s.ApplyMigrations(ctx); err != nil {
		t.Fatal(err)
	}
	return s
}

func seedPlayerCatalog(t *testing.T, s *Store) (string, string) {
	t.Helper()
	ctx := context.Background()
	gameID, err := NewID()
	if err != nil {
		t.Fatal(err)
	}
	presetID, err := NewID()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Pool.Exec(ctx, `INSERT INTO arcade_games(id,workshop_id,display_name) VALUES($1,'3564393242','Test game')`, gameID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Pool.Exec(ctx, `INSERT INTO content_versions(id,arcade_game_id,content_sha256) VALUES('test-v1',$1,$2)`, gameID, strings.Repeat("a", 64)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Pool.Exec(ctx, `UPDATE arcade_games SET current_content_version_id='test-v1' WHERE id=$1`, gameID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Pool.Exec(ctx, `INSERT INTO template_revisions(id,arcade_game_id) VALUES('test-template',$1)`, gameID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Pool.Exec(ctx, `INSERT INTO game_presets(id,arcade_game_id,display_name,max_players,template_revision_id)
		VALUES($1,$2,'N6',10,'test-template')`, presetID, gameID); err != nil {
		t.Fatal(err)
	}
	return gameID, presetID
}

func TestP1ARequestConcurrencyAndMaintenance(t *testing.T) {
	s := playerTestStore(t)
	ctx := context.Background()
	gameID, presetID := seedPlayerCatalog(t, s)
	userID, _, err := s.CreateUserSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	const count = 12
	var wg sync.WaitGroup
	ids := make([]string, count)
	errs := make([]error, count)
	for i := 0; i < count; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			r, _, err := s.CreateUserRequest(ctx, userID, gameID, presetID)
			ids[i], errs[i] = r.ID, err
		}(i)
	}
	wg.Wait()
	for i := 0; i < count; i++ {
		if errs[i] != nil || ids[i] != ids[0] {
			t.Fatalf("submission %d got %q %v, first %q", i, ids[i], errs[i], ids[0])
		}
	}
	var n int
	if err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM server_requests WHERE owner_user_id=$1`, userID).Scan(&n); err != nil || n != 1 {
		t.Fatalf("request count %d: %v", n, err)
	}
	current, err := s.CurrentUserRequest(ctx, userID)
	if err != nil || current == nil || current.ID != ids[0] {
		t.Fatalf("current request %+v: %v", current, err)
	}
	otherID, _, err := s.CreateUserSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.UserRequest(ctx, otherID, ids[0]); err == nil {
		t.Fatal("other user read request")
	}
	if _, err := s.Pool.Exec(ctx, `UPDATE platform_settings SET accepting_new_requests=false,maintenance_message='global pause'`); err != nil {
		t.Fatal(err)
	}
	if r, created, err := s.CreateUserRequest(ctx, userID, gameID, presetID); err != nil || created || r.ID != ids[0] {
		t.Fatalf("duplicate during maintenance: %+v %t %v", r, created, err)
	}
	assertMaintenance := func(scope string) {
		t.Helper()
		_, _, err := s.CreateUserRequest(ctx, otherID, gameID, presetID)
		var maintenance *MaintenanceError
		if !errors.As(err, &maintenance) || maintenance.Scope != scope {
			t.Fatalf("want %s maintenance, got %v", scope, err)
		}
	}
	assertMaintenance("global")
	if _, err := s.Pool.Exec(ctx, `UPDATE platform_settings SET accepting_new_requests=true`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Pool.Exec(ctx, `UPDATE arcade_games SET accepting_new_requests=false,maintenance_message='game pause' WHERE id=$1`, gameID); err != nil {
		t.Fatal(err)
	}
	assertMaintenance("arcadeGame")
	if _, err := s.Pool.Exec(ctx, `UPDATE arcade_games SET accepting_new_requests=true WHERE id=$1`, gameID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Pool.Exec(ctx, `UPDATE game_presets SET accepting_new_requests=false,maintenance_message='preset pause' WHERE id=$1`, presetID); err != nil {
		t.Fatal(err)
	}
	assertMaintenance("gamePreset")
	if _, err := s.Pool.Exec(ctx, `UPDATE game_presets SET accepting_new_requests=true,enabled=false WHERE id=$1`, presetID); err != nil {
		t.Fatal(err)
	}
	assertMaintenance("gamePreset")
	if _, err := s.Pool.Exec(ctx, `UPDATE game_presets SET enabled=true WHERE id=$1`, presetID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Pool.Exec(ctx, `UPDATE arcade_games SET enabled=false WHERE id=$1`, gameID); err != nil {
		t.Fatal(err)
	}
	assertMaintenance("arcadeGame")
	if _, err := s.Pool.Exec(ctx, `UPDATE content_versions SET content_sha256=$1 WHERE id='test-v1'`, strings.Repeat("b", 64)); err == nil {
		t.Fatal("ContentVersion changed")
	}
}
