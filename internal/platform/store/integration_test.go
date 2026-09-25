package store

import (
	"context"
	"encoding/hex"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Set PLATFORM_TEST_DATABASE_URL to an isolated development PostgreSQL
// instance. Each run creates a disposable schema and never touches public.
func TestPostgresDurability(t *testing.T) {
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
	schema := "p0b_" + strings.ReplaceAll(id, "-", "")
	if _, err := base.Exec(ctx, `CREATE SCHEMA "`+schema+`"`); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if _, err := base.Exec(ctx, `DROP SCHEMA "`+schema+`" CASCADE`); err != nil {
			t.Errorf("cleanup test schema: %v", err)
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
	if err := s.ApplyMigrations(ctx); err != nil {
		t.Fatal(err)
	}
	if err := s.ApplyMigrations(ctx); err != nil {
		t.Fatalf("idempotent migration: %v", err)
	}

	userID, userToken, err := s.CreateUserSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := s.UserForToken(ctx, userToken); err != nil || got != userID {
		t.Fatalf("user session: %s %v", got, err)
	}
	adminID, err := s.CreateAdmin(ctx, "TestAdmin", "a long test password")
	if err != nil {
		t.Fatal(err)
	}
	gotID, adminToken, err := s.LoginAdmin(ctx, "testadmin", "a long test password")
	if err != nil || gotID != adminID {
		t.Fatalf("admin login: %s %v", gotID, err)
	}
	if got, err := s.AdminForToken(ctx, adminToken); err != nil || got != adminID {
		t.Fatalf("admin session: %s %v", got, err)
	}
	reopenedPool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	reopened := &Store{Pool: reopenedPool}
	defer reopened.Close()
	if err := reopened.ApplyMigrations(ctx); err != nil {
		t.Fatalf("migration after reopen: %v", err)
	}
	if got, err := reopened.UserForToken(ctx, userToken); err != nil || got != userID {
		t.Fatalf("user session after reopen: %s %v", got, err)
	}
	if got, err := reopened.AdminForToken(ctx, adminToken); err != nil || got != adminID {
		t.Fatalf("admin session after reopen: %s %v", got, err)
	}
	if err := s.ResetAdminPassword(ctx, "testadmin", "another long test password"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AdminForToken(ctx, adminToken); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("reset did not revoke session: %v", err)
	}
	if _, _, err := s.LoginAdmin(ctx, "testadmin", "a long test password"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("old password accepted: %v", err)
	}
	_, freshAdminToken, err := s.LoginAdmin(ctx, "testadmin", "another long test password")
	if err != nil {
		t.Fatalf("new password rejected: %v", err)
	}
	if _, err := s.Pool.Exec(ctx, "UPDATE admin_users SET enabled=false WHERE id=$1", adminID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AdminForToken(ctx, freshAdminToken); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("disabled admin session accepted: %v", err)
	}

	nodeID, err := NewID()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Pool.Exec(ctx, "INSERT INTO nodes(id,display_name) VALUES($1,'test node')", nodeID); err != nil {
		t.Fatal(err)
	}
	targetJobID, err := s.CreateIntegrationCreateJob(ctx, nodeID, "test-template", 28000)
	if err != nil {
		t.Fatal(err)
	}
	targetJob, err := s.JobForNode(ctx, nodeID, targetJobID)
	if err != nil || targetJob.TemplateBindingKey != "test-template" || targetJob.RequestedPort != 28000 {
		t.Fatalf("integration target: %+v %v", targetJob, err)
	}
	jobID, err := s.CreateIntegrationJob(ctx, nodeID, "create", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.PrepareCreate(ctx, nodeID, jobID, "C:/test/template.json", 0); err == nil {
		t.Fatal("unclaimed job prepared")
	}
	if _, err := s.Pool.Exec(ctx, "UPDATE node_jobs SET state='claimed' WHERE id=$1", jobID); err != nil {
		t.Fatal(err)
	}
	frozen, err := s.PrepareCreate(ctx, nodeID, jobID, "C:/test/template.json", 0)
	if err != nil {
		t.Fatal(err)
	}
	if frozen.IdempotencyKey != CoreIdempotencyKey(jobID) || len(frozen.FingerprintSHA) != 32 {
		t.Fatal("incomplete frozen execution")
	}
	if preparedJob, err := reopened.JobForNode(ctx, nodeID, jobID); err != nil || preparedJob.FrozenCreate == nil || preparedJob.PreparedAtUnix <= 0 {
		t.Fatalf("reopened job lacks durable prepared time: %+v %v", preparedJob, err)
	}
	if _, err := s.PrepareCreate(ctx, nodeID, jobID, "C:/test/template.json", 0); err != nil {
		t.Fatalf("same request retry rejected: %v", err)
	}
	if _, err := s.PrepareCreate(ctx, nodeID, jobID, "C:/other/template.json", 0); err == nil {
		t.Fatal("changed frozen request accepted")
	}
	if _, err := s.Pool.Exec(ctx, "UPDATE node_job_executions SET requested_port=1 WHERE node_job_id=$1", jobID); err == nil {
		t.Fatal("frozen execution changed in SQL")
	}
	if _, err := s.Pool.Exec(ctx, "DELETE FROM node_job_executions WHERE node_job_id=$1", jobID); err == nil {
		t.Fatal("frozen execution deleted in SQL")
	}
	if got, err := s.FrozenCreateForJob(ctx, nodeID, jobID); err != nil || hex.EncodeToString(got.FingerprintSHA) != hex.EncodeToString(frozen.FingerprintSHA) {
		t.Fatalf("frozen record changed: %v", err)
	}
	if got, err := reopened.FrozenCreateForJob(ctx, nodeID, jobID); err != nil || hex.EncodeToString(got.FingerprintSHA) != hex.EncodeToString(frozen.FingerprintSHA) {
		t.Fatalf("frozen record after reopen: %v", err)
	}
	if err := s.RevokeUserToken(ctx, userToken); err != nil {
		t.Fatal(err)
	}
	if _, err := reopened.UserForToken(ctx, userToken); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("revoked user session accepted: %v", err)
	}
	if _, err := s.Pool.Exec(ctx, "UPDATE schema_migrations SET checksum='changed' WHERE version=1"); err != nil {
		t.Fatal(err)
	}
	if err := s.ApplyMigrations(ctx); err == nil {
		t.Fatal("changed migration checksum accepted")
	}
}
