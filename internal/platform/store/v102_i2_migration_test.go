package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestI2Migration20To21AndRerun(t *testing.T) {
	dsn := os.Getenv("PLATFORM_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("disposable PostgreSQL unavailable")
	}
	ctx := context.Background()
	base, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := NewID()
	schema := "i2upgrade_" + strings.ReplaceAll(id, "-", "")
	if _, err := base.Exec(ctx, `CREATE SCHEMA "`+schema+`"`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = base.Exec(ctx, `DROP SCHEMA "`+schema+`" CASCADE`); base.Close() })
	conf, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	conf.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, conf)
	if err != nil {
		t.Fatal(err)
	}
	s := &Store{Pool: pool}
	t.Cleanup(s.Close)
	if _, err := pool.Exec(ctx, `CREATE TABLE schema_migrations(version integer PRIMARY KEY,checksum text NOT NULL,applied_at timestamptz NOT NULL DEFAULT now())`); err != nil {
		t.Fatal(err)
	}
	names, err := fs.Glob(migrationFiles, "migrations/*.sql")
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(names)
	for version, name := range names[:20] {
		data, err := migrationFiles.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, string(data), pgx.QueryExecModeSimpleProtocol); err != nil {
			t.Fatalf("migration %d: %v", version+1, err)
		}
		sum := sha256.Sum256(data)
		if _, err := pool.Exec(ctx, `INSERT INTO schema_migrations(version,checksum) VALUES($1,$2)`, version+1, hex.EncodeToString(sum[:])); err != nil {
			t.Fatal(err)
		}
	}
	nodeID, _, err := s.RegisterNode(ctx, "upgrade node", "linux")
	if err != nil {
		t.Fatal(err)
	}
	adminID, err := s.CreateAdmin(ctx, "upgrade-admin", "test-only-long-password")
	if err != nil {
		t.Fatal(err)
	}
	gameID, presetID, runID, allocationID, jobID := mustUpgradeID(t), mustUpgradeID(t), mustUpgradeID(t), mustUpgradeID(t), mustUpgradeID(t)
	if _, err := pool.Exec(ctx, `INSERT INTO arcade_games(id,workshop_id,display_name) VALUES($1,'12345','Upgrade game')`, gameID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO content_versions(id,arcade_game_id,content_sha256) VALUES('upgrade-v1',$1,$2)`, gameID, strings.Repeat("a", 64)); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO template_revisions(id,arcade_game_id) VALUES('upgrade-template',$1)`, gameID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO game_presets(id,arcade_game_id,display_name,max_players,template_revision_id) VALUES($1,$2,'Upgrade',1,'upgrade-template')`, presetID, gameID); err != nil {
		t.Fatal(err)
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `INSERT INTO validation_runs(id,started_by_admin_user_id,node_id,arcade_game_id,game_preset_id,
		content_version_id,content_sha256,template_revision_id,template_binding_key,template_fingerprint_sha256,
		template_binding_generation,maintenance_epoch,content_fact_revision,template_fact_revision)
		VALUES($1,$2,$3,$4,$5,'upgrade-v1',$6,'upgrade-template','upgrade-key',$7,2,1,1,1)`, runID, adminID, nodeID, gameID, presetID, strings.Repeat("a", 64), strings.Repeat("f", 64)); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO allocations(id,purpose,validation_run_id,arcade_game_id,attempt_sequence,node_id,content_version_id,template_revision_id,state,assigned_at)
		VALUES($1,'validation',$2,$3,1,$4,'upgrade-v1','upgrade-template','reserved',now())`, allocationID, runID, gameID, nodeID); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO node_jobs(id,node_id,kind,integration_only,allocation_id,template_binding_key,required_capability)
		VALUES($1,$2,'create',false,$3,'upgrade-key','content_validation_v102')`, jobID, nodeID, allocationID); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err := s.ApplyMigrations(ctx); err != nil {
		t.Fatal(err)
	}
	if err := s.ApplyMigrations(ctx); err != nil {
		t.Fatalf("rerun: %v", err)
	}
	var latest, count int
	if err := pool.QueryRow(ctx, `SELECT max(version),count(*) FROM schema_migrations`).Scan(&latest, &count); err != nil || latest != 22 || count != 22 {
		t.Fatalf("migration ledger %d/%d: %v", latest, count, err)
	}
	var columns int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM information_schema.columns WHERE table_schema=$1 AND table_name='node_reports' AND column_name IN ('inventory_scan_id','inventory_state')`, schema).Scan(&columns); err != nil || columns != 2 {
		t.Fatalf("inventory columns %d: %v", columns, err)
	}
	var fingerprint, workshop, version, vpk string
	var generation int64
	if err := pool.QueryRow(ctx, `SELECT expected_template_fingerprint_sha256,template_binding_generation,
		expected_workshop_id,expected_content_version_id,expected_vpk_sha256 FROM node_jobs WHERE id=$1`, jobID).
		Scan(&fingerprint, &generation, &workshop, &version, &vpk); err != nil || fingerprint != strings.Repeat("f", 64) || generation != 2 || workshop != "12345" || version != "upgrade-v1" || vpk != strings.Repeat("a", 64) {
		t.Fatalf("open I1 Job identity not backfilled: %s %d %s %s %s %v", fingerprint, generation, workshop, version, vpk, err)
	}
	if _, err := pool.Exec(ctx, `UPDATE node_jobs SET expected_vpk_sha256=$2 WHERE id=$1`, jobID, strings.Repeat("b", 64)); err == nil {
		t.Fatal("immutable Job expected identity changed")
	}
	if _, err := pool.Exec(ctx, `INSERT INTO node_jobs(id,node_id,kind,integration_only,required_capability)
		VALUES($1,$2,'create',true,'content_validation_v102')`, mustUpgradeID(t), nodeID); err == nil {
		t.Fatal("incomplete v102 Job accepted")
	}
}

func mustUpgradeID(t *testing.T) string {
	t.Helper()
	id, err := NewID()
	if err != nil {
		t.Fatal(err)
	}
	return id
}
