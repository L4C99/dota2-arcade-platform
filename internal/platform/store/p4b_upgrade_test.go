package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path"
	"strconv"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestP4BUpgradeFromP3ClosurePreservesHistory(t *testing.T) {
	dsn := os.Getenv("PLATFORM_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("PLATFORM_TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	base, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := NewID()
	schema := "p4bupgrade_" + strings.ReplaceAll(id, "-", "")
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
	if _, err := pool.Exec(ctx, `CREATE TABLE schema_migrations(version integer PRIMARY KEY,checksum text NOT NULL,
		applied_at timestamptz NOT NULL DEFAULT now())`); err != nil {
		t.Fatal(err)
	}
	names, err := fs.Glob(migrationFiles, "migrations/*.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range names {
		prefix, _, _ := strings.Cut(path.Base(name), "_")
		version, err := strconv.Atoi(prefix)
		if err != nil {
			t.Fatal(err)
		}
		if version > 10 {
			continue
		}
		content, err := migrationFiles.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := tx.Exec(ctx, string(content), pgx.QueryExecModeSimpleProtocol); err != nil {
			_ = tx.Rollback(ctx)
			t.Fatalf("P3 migration %d: %v", version, err)
		}
		checksum := sha256.Sum256(content)
		if _, err := tx.Exec(ctx, `INSERT INTO schema_migrations(version,checksum) VALUES($1,$2)`, version,
			hex.EncodeToString(checksum[:])); err != nil {
			_ = tx.Rollback(ctx)
			t.Fatal(err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
	}
	gameID, presetID := seedPlayerCatalog(t, s)
	nodeID, _, err := s.RegisterNode(ctx, "old P3 node", "linux")
	if err != nil {
		t.Fatal(err)
	}
	owner, _, err := s.CreateUserSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	r, _, err := s.CreateUserRequest(ctx, owner, gameID, presetID)
	if err != nil {
		t.Fatal(err)
	}
	allocationID, _ := NewID()
	jobID, _ := NewID()
	if _, err := pool.Exec(ctx, `INSERT INTO allocations(id,server_request_id,arcade_game_id,attempt_sequence,node_id,
		content_version_id,template_revision_id,state,assigned_at,reclaimed_at)
		VALUES($1,$2,$3,1,$4,'test-v1','test-template','reclaimed',now(),now())`, allocationID, r.ID, gameID, nodeID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO node_jobs(id,node_id,kind,state,integration_only,allocation_id,instance_id,operation_id)
		VALUES($1,$2,'create','succeeded',false,$3,'i_old','o_old')`, jobID, nodeID, allocationID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE server_requests SET state='ended' WHERE id=$1`, r.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.ApplyMigrations(ctx); err != nil {
		t.Fatalf("P3 to P4 migration: %v", err)
	}
	if err := s.ApplyMigrations(ctx); err != nil {
		t.Fatalf("repeat migration: %v", err)
	}
	var oldNode, oldVersion, oldJob, oldRequestState string
	if err := pool.QueryRow(ctx, `SELECT a.node_id,a.content_version_id,j.id,r.state FROM allocations a
		JOIN node_jobs j ON j.allocation_id=a.id JOIN server_requests r ON r.id=a.server_request_id
		WHERE a.id=$1`, allocationID).Scan(&oldNode, &oldVersion, &oldJob, &oldRequestState); err != nil {
		t.Fatal(err)
	}
	if oldNode != nodeID || oldVersion != "test-v1" || oldJob != jobID || oldRequestState != "ended" {
		t.Fatal(fmt.Sprintf("P3 history changed: %s %s %s %s", oldNode, oldVersion, oldJob, oldRequestState))
	}
	var version int
	if err := pool.QueryRow(ctx, `SELECT max(version) FROM schema_migrations`).Scan(&version); err != nil || version < 13 {
		t.Fatalf("migration version=%d: %v", version, err)
	}
}
