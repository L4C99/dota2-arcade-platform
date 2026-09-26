package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestP5UpgradeFromMigration13PreservesCatalog(t *testing.T) {
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
	id, err := NewID()
	if err != nil {
		t.Fatal(err)
	}
	schema := "p5_upgrade_" + strings.ReplaceAll(id, "-", "")
	if _, err := base.Exec(ctx, `CREATE SCHEMA "`+schema+`"`); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if _, err := base.Exec(ctx, `DROP SCHEMA "`+schema+`" CASCADE`); err != nil {
			t.Errorf("cleanup schema: %v", err)
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
	defer pool.Close()
	for version := 1; version <= 13; version++ {
		entries, err := migrationFiles.ReadDir("migrations")
		if err != nil {
			t.Fatal(err)
		}
		var path string
		for _, entry := range entries {
			if strings.HasPrefix(entry.Name(), fmt.Sprintf("%04d_", version)) {
				path = "migrations/" + entry.Name()
				break
			}
		}
		if path == "" {
			t.Fatalf("migration %d missing", version)
		}
		raw, err := migrationFiles.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, string(raw), pgx.QueryExecModeSimpleProtocol); err != nil {
			t.Fatalf("apply %d: %v", version, err)
		}
		if version == 1 {
			if _, err := pool.Exec(ctx, `CREATE TABLE schema_migrations(version integer PRIMARY KEY,checksum text NOT NULL,applied_at timestamptz NOT NULL DEFAULT now())`); err != nil {
				t.Fatal(err)
			}
		}
		checksum := sha256.Sum256(raw)
		if _, err := pool.Exec(ctx, `INSERT INTO schema_migrations(version,checksum) VALUES($1,$2)`, version, hex.EncodeToString(checksum[:])); err != nil {
			t.Fatal(err)
		}
	}
	s := &Store{Pool: pool}
	gameID, _ := seedPlayerCatalog(t, s)
	if _, err := s.CreateAdmin(ctx, "p5-upgrade-admin", "a long test password"); err != nil {
		t.Fatal(err)
	}
	nodeID, _, err := s.RegisterNode(ctx, "previously verified", "linux")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO node_entry_capabilities(node_id,entry_config_revision,reported_at,
		steam_entry_verified,steam_entry_enabled,steamchina_entry_verified,steamchina_entry_enabled)
		VALUES($1,$2,now(),true,true,true,true)`, nodeID, strings.Repeat("a", 64)); err != nil {
		t.Fatal(err)
	}
	if err := s.ApplyMigrations(ctx); err != nil {
		t.Fatal(err)
	}
	if err := s.ApplyMigrations(ctx); err != nil {
		t.Fatalf("idempotent upgrade: %v", err)
	}
	var version int
	if err := pool.QueryRow(ctx, `SELECT max(version) FROM schema_migrations`).Scan(&version); err != nil || version != 17 {
		t.Fatalf("migration version %d: %v", version, err)
	}
	var current, workshop string
	if err := pool.QueryRow(ctx, `SELECT current_content_version_id,workshop_id FROM arcade_games WHERE id=$1`, gameID).Scan(&current, &workshop); err != nil || current != "test-v1" || workshop != "3564393242" {
		t.Fatalf("catalog lost: %s %s %v", current, workshop, err)
	}
	var steamVerified, steamEnabled, chinaVerified, chinaEnabled bool
	if err := pool.QueryRow(ctx, `SELECT steam_entry_verified,steam_entry_enabled,steamchina_entry_verified,steamchina_entry_enabled
		FROM node_entry_capabilities WHERE node_id=$1`, nodeID).Scan(&steamVerified, &steamEnabled, &chinaVerified, &chinaEnabled); err != nil || steamVerified || steamEnabled || chinaVerified || chinaEnabled {
		t.Fatalf("old partial verification not invalidated: %v %v %v %v %v", steamVerified, steamEnabled, chinaVerified, chinaEnabled, err)
	}
}
