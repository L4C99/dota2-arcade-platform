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

func TestAmendment001UpgradePreservesVerifiedState(t *testing.T) {
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
	schema := "amendment001_" + strings.ReplaceAll(id, "-", "")
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
	if _, err := pool.Exec(ctx, `CREATE TABLE schema_migrations(version integer PRIMARY KEY,checksum text NOT NULL,applied_at timestamptz NOT NULL DEFAULT now())`); err != nil {
		t.Fatal(err)
	}
	entries, err := migrationFiles.ReadDir("migrations")
	if err != nil {
		t.Fatal(err)
	}
	for version := 1; version <= 16; version++ {
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
			t.Fatalf("apply migration %d: %v", version, err)
		}
		checksum := sha256.Sum256(raw)
		if _, err := pool.Exec(ctx, `INSERT INTO schema_migrations(version,checksum) VALUES($1,$2)`, version, hex.EncodeToString(checksum[:])); err != nil {
			t.Fatal(err)
		}
	}
	adminID, err := (&Store{Pool: pool}).CreateAdmin(ctx, "amendment-admin", "a long test password")
	if err != nil {
		t.Fatal(err)
	}
	nodeID, _, err := (&Store{Pool: pool}).RegisterNode(ctx, "amendment node", "linux")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO node_entry_capabilities(node_id,entry_config_revision,reported_at,
		steam_entry_verified,steam_entry_enabled,steam_verified_at,steam_verified_by,steam_verified_ports,steam_verification_note)
		VALUES($1,$2,now(),true,true,now(),$3,ARRAY[28000,28001],'old evidence')`, nodeID, strings.Repeat("a", 64), adminID); err != nil {
		t.Fatal(err)
	}
	if err := (&Store{Pool: pool}).ApplyMigrations(ctx); err != nil {
		t.Fatal(err)
	}
	var verified, enabled, hasTime, hasActor bool
	if err := pool.QueryRow(ctx, `SELECT steam_entry_verified,steam_entry_enabled,steam_verified_at IS NOT NULL,steam_verified_by IS NOT NULL
		FROM node_entry_capabilities WHERE node_id=$1`, nodeID).Scan(&verified, &enabled, &hasTime, &hasActor); err != nil {
		t.Fatal(err)
	}
	if !verified || !enabled || !hasTime || !hasActor {
		t.Fatal("migration 17 lost existing human verification")
	}
	var obsolete int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM information_schema.columns WHERE table_schema=$1 AND table_name='node_entry_capabilities'
		AND column_name IN ('steam_verified_ports','steamchina_verified_ports','steam_verification_note','steamchina_verification_note')`, schema).Scan(&obsolete); err != nil || obsolete != 0 {
		t.Fatalf("obsolete columns remain: %d: %v", obsolete, err)
	}
}
