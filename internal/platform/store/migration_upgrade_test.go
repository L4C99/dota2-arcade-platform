package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestUpgradeFromP0B(t *testing.T) {
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
	schema := "upgrade_" + strings.ReplaceAll(id, "-", "")
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
	s := &Store{Pool: pool}
	defer s.Close()
	initial, err := migrationFiles.ReadFile("migrations/0001_initial.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, string(initial), pgx.QueryExecModeSimpleProtocol); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `CREATE TABLE schema_migrations(version integer PRIMARY KEY,checksum text NOT NULL,applied_at timestamptz NOT NULL DEFAULT now())`); err != nil {
		t.Fatal(err)
	}
	checksum := sha256.Sum256(initial)
	if _, err := pool.Exec(ctx, "INSERT INTO schema_migrations(version,checksum) VALUES(1,$1)", hex.EncodeToString(checksum[:])); err != nil {
		t.Fatal(err)
	}
	nodeID, err := NewID()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "INSERT INTO nodes(id,display_name) VALUES($1,'P0B existing node')", nodeID); err != nil {
		t.Fatal(err)
	}
	if err := s.ApplyMigrations(ctx); err != nil {
		t.Fatal(err)
	}
	var displayName string
	if err := pool.QueryRow(ctx, "SELECT display_name FROM nodes WHERE id=$1", nodeID).Scan(&displayName); err != nil || displayName != "P0B existing node" {
		t.Fatalf("existing node lost during upgrade: %q %v", displayName, err)
	}
	if _, _, err := s.RegisterNode(ctx, "new node", "linux"); err != nil {
		t.Fatalf("new node registration after upgrade: %v", err)
	}
}
