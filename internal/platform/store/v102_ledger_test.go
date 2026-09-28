package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io/fs"
	"os"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/L4C99/dota2-arcade-platform/internal/contracts/nodev1"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestV102MigrationFrom19(t *testing.T) {
	dsn := os.Getenv("PLATFORM_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("isolated PostgreSQL unavailable")
	}
	ctx := context.Background()
	base, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer base.Close()
	id, _ := NewID()
	schema := "i1_" + strings.ReplaceAll(id, "-", "")
	if _, err := base.Exec(ctx, `CREATE SCHEMA "`+schema+`"`); err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = base.Exec(ctx, `DROP SCHEMA "`+schema+`" CASCADE`) }()
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
	s := &Store{Pool: pool}
	if _, err := pool.Exec(ctx, `CREATE TABLE schema_migrations(version integer PRIMARY KEY,checksum text NOT NULL,applied_at timestamptz NOT NULL DEFAULT now())`); err != nil {
		t.Fatal(err)
	}
	names, err := fs.Glob(migrationFiles, "migrations/*.sql")
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(names)
	oldChecksums := map[int]string{}
	for version, name := range names {
		if version >= 19 {
			break
		}
		content, err := migrationFiles.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, string(content), pgx.QueryExecModeSimpleProtocol); err != nil {
			t.Fatalf("old migration %d: %v", version+1, err)
		}
		if version+1 == 9 {
			tx, err := pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if err := backfillUserDisplayNames(ctx, tx); err != nil {
				t.Fatal(err)
			}
			if err := tx.Commit(ctx); err != nil {
				t.Fatal(err)
			}
		}
		digest := sha256.Sum256(content)
		oldChecksums[version+1] = hex.EncodeToString(digest[:])
		if _, err := pool.Exec(ctx, `INSERT INTO schema_migrations(version,checksum) VALUES($1,$2)`, version+1, oldChecksums[version+1]); err != nil {
			t.Fatal(err)
		}
	}
	game, preset := seedPlayerCatalog(t, s)
	node, _, err := s.RegisterNode(ctx, "migration node", "linux")
	if err != nil {
		t.Fatal(err)
	}
	user, _, err := s.CreateUserSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	request, _, err := s.CreateUserRequest(ctx, user, game, preset)
	if err != nil {
		t.Fatal(err)
	}
	allocation, _ := NewID()
	if _, err := pool.Exec(ctx, `INSERT INTO allocations(id,server_request_id,arcade_game_id,attempt_sequence,node_id,content_version_id,template_revision_id,assigned_at)
		VALUES($1,$2,$3,1,$4,'test-v1','test-template',now())`, allocation, request.ID, game, node); err != nil {
		t.Fatal(err)
	}
	job, _ := NewID()
	if _, err := pool.Exec(ctx, `INSERT INTO node_jobs(id,node_id,kind,integration_only,allocation_id,state,template_binding_key)
		VALUES($1,$2,'create',false,$3,'pending','test-binding')`, job, node, allocation); err != nil {
		t.Fatal(err)
	}
	admin, err := s.CreateAdmin(ctx, "migration-admin", "a long test password")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO content_release_validations(node_id,arcade_game_id,content_version_id,verified_by)
		VALUES($1,$2,'test-v1',$3)`, node, game, admin); err != nil {
		t.Fatal(err)
	}
	if err := s.ApplyMigrations(ctx); err != nil {
		t.Fatal(err)
	}
	if err := s.ApplyMigrations(ctx); err != nil {
		t.Fatalf("rerun current: %v", err)
	}
	for version, want := range oldChecksums {
		var got string
		if err := pool.QueryRow(ctx, `SELECT checksum FROM schema_migrations WHERE version=$1`, version).Scan(&got); err != nil || got != want {
			t.Fatalf("old checksum %d changed: %s %v", version, got, err)
		}
	}
	var purpose, contract, owner string
	if err := pool.QueryRow(ctx, `SELECT purpose,server_request_id FROM allocations WHERE id=$1`, allocation).Scan(&purpose, &owner); err != nil || purpose != "player" || owner != request.ID {
		t.Fatalf("old allocation changed: %s %s %v", purpose, owner, err)
	}
	if err := pool.QueryRow(ctx, `SELECT validation_contract FROM game_presets WHERE id=$1`, preset).Scan(&contract); err != nil || contract != "legacy_v1" {
		t.Fatalf("legacy contract: %s %v", contract, err)
	}
	var jobState, capability string
	if err := pool.QueryRow(ctx, `SELECT state,required_capability FROM node_jobs WHERE id=$1`, job).Scan(&jobState, &capability); err != nil || jobState != "pending" || capability != "legacy_v1" {
		t.Fatalf("old Job changed: %s %s %v", jobState, capability, err)
	}
	var legacyRows, runs int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM content_release_validations WHERE node_id=$1 AND arcade_game_id=$2`, node, game).Scan(&legacyRows); err != nil || legacyRows != 1 {
		t.Fatalf("legacy validation lost: %d %v", legacyRows, err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM validation_runs`).Scan(&runs); err != nil || runs != 0 {
		t.Fatalf("legacy row promoted to Run: %d %v", runs, err)
	}
	invalidID, _ := NewID()
	if _, err := pool.Exec(ctx, `INSERT INTO allocations(id,purpose,server_request_id,arcade_game_id,attempt_sequence,node_id,content_version_id,template_revision_id,assigned_at)
		VALUES($1,'validation',$2,$3,1,$4,'test-v1','test-template',now())`, invalidID, request.ID, game, node); err == nil {
		t.Fatal("invalid owner XOR accepted")
	}
}

func v102ValidationFixture(t *testing.T, hard int) (*Store, ValidationCandidate) {
	t.Helper()
	s := playerTestStore(t)
	ctx := context.Background()
	game, preset := seedPlayerCatalog(t, s)
	admin, err := s.CreateAdmin(ctx, "i1-admin", "a long test password")
	if err != nil {
		t.Fatal(err)
	}
	node, _, err := s.RegisterNode(ctx, "i1-node", "linux")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Pool.Exec(ctx, `INSERT INTO node_template_bindings(node_id,template_revision_id,binding_key)
		VALUES($1,'test-template','test-binding')`, node); err != nil {
		t.Fatal(err)
	}
	h := p1TestHeartbeat("test-v1")
	h.HardMaxInstances = hard
	h.Network.LocalPortMax = h.Network.LocalPortMin + hard - 1
	h.Content[0].VPKSHA256 = strings.Repeat("a", 64)
	if _, err := s.RecordHeartbeat(ctx, node, h); err != nil {
		t.Fatal(err)
	}
	var revision int64
	if err := s.Pool.QueryRow(ctx, `SELECT content_fact_revision FROM node_content_bindings WHERE node_id=$1 AND arcade_game_id=$2`, node, game).Scan(&revision); err != nil || revision != 1 {
		t.Fatalf("first content revision %d %v", revision, err)
	}
	if _, err := s.RecordHeartbeat(ctx, node, h); err != nil {
		t.Fatal(err)
	}
	if err := s.Pool.QueryRow(ctx, `SELECT content_fact_revision FROM node_content_bindings WHERE node_id=$1 AND arcade_game_id=$2`, node, game).Scan(&revision); err != nil || revision != 1 {
		t.Fatalf("repeat content revision %d %v", revision, err)
	}
	empty := h
	empty.Content = nil
	if _, err := s.RecordHeartbeat(ctx, node, empty); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RecordHeartbeat(ctx, node, empty); err != nil {
		t.Fatal(err)
	}
	if err := s.Pool.QueryRow(ctx, `SELECT content_fact_revision FROM node_content_bindings WHERE node_id=$1 AND arcade_game_id=$2`, node, game).Scan(&revision); err != nil || revision != 2 {
		t.Fatalf("repeated unknown revision %d %v", revision, err)
	}
	if _, err := s.RecordHeartbeat(ctx, node, h); err != nil {
		t.Fatal(err)
	}
	if err := s.Pool.QueryRow(ctx, `SELECT content_fact_revision FROM node_content_bindings WHERE node_id=$1 AND arcade_game_id=$2`, node, game).Scan(&revision); err != nil || revision != 3 {
		t.Fatalf("restored fact revision %d %v", revision, err)
	}
	if _, err := s.Pool.Exec(ctx, `UPDATE node_reports SET capabilities=ARRAY['contentValidationV102'] WHERE node_id=$1`, node); err != nil {
		t.Fatal(err)
	}
	if err := s.SetDesiredCapacity(ctx, node, hard); err != nil {
		t.Fatal(err)
	}
	fingerprint := strings.Repeat("f", 64)
	generation, err := s.UpsertTemplateBinding(ctx, node, "test-template", "test-binding", fingerprint, 1)
	if err != nil || generation != 2 {
		t.Fatalf("binding generation %d %v", generation, err)
	}
	if again, err := s.UpsertTemplateBinding(ctx, node, "test-template", "test-binding", fingerprint, 2); err != nil || again != 2 {
		t.Fatalf("repeat binding generation %d %v", again, err)
	}
	if revision, err := s.RecordTemplateFact(ctx, node, "test-template", "test-binding", "confirmed", "template-manifest-sha256-v1", fingerprint); err != nil || revision != 1 {
		t.Fatalf("first template fact %d %v", revision, err)
	}
	if revision, err := s.RecordTemplateFact(ctx, node, "test-template", "test-binding", "confirmed", "template-manifest-sha256-v1", fingerprint); err != nil || revision != 1 {
		t.Fatalf("repeat template fact %d %v", revision, err)
	}
	if snapshot, err := s.RecordInventory(ctx, node, "first-scan", false, "LIST_FAILED", nil); err != nil || snapshot.State != "unknown" {
		t.Fatalf("partial inventory %+v %v", snapshot, err)
	}
	if snapshot, err := s.RecordInventory(ctx, node, "second-scan", true, "", nil); err != nil || snapshot.State != "confirmed" {
		t.Fatalf("complete inventory %+v %v", snapshot, err)
	}
	if _, err := s.RecordInventory(ctx, node, "second-scan", true, "", nil); !errors.Is(err, ErrJobConflict) {
		t.Fatalf("duplicate scan accepted: %v", err)
	}
	epoch, err := s.BeginScopedMaintenance(ctx, node, game, admin, "maintenance-1", "content validation", 0)
	if err != nil || epoch != 1 {
		t.Fatalf("maintenance epoch %d %v", epoch, err)
	}
	if again, err := s.BeginScopedMaintenance(ctx, node, game, admin, "maintenance-1", "content validation", 0); err != nil || again != 1 {
		t.Fatalf("maintenance retry %d %v", again, err)
	}
	return s, ValidationCandidate{NodeID: node, GameID: game, PresetID: preset, ContentVersionID: "test-v1", TemplateRevisionID: "test-template", AdminID: admin, ExpectedMaintenanceEpoch: 1, ExpectedTemplateFingerprintSHA256: fingerprint}
}

func TestV102ValidationReserveAndCapability(t *testing.T) {
	s, c := v102ValidationFixture(t, 1)
	ctx := context.Background()
	run, err := s.ReserveValidation(ctx, c)
	if err != nil {
		t.Fatal(err)
	}
	if run.State != "create_pending" || run.AllocationID == "" || run.CreateJobID == "" {
		t.Fatalf("incomplete reserve %+v", run)
	}
	var purpose, jobCapability string
	var occupied int
	if err := s.Pool.QueryRow(ctx, `SELECT purpose FROM allocations WHERE id=$1`, run.AllocationID).Scan(&purpose); err != nil || purpose != "validation" {
		t.Fatalf("purpose %s %v", purpose, err)
	}
	if err := s.Pool.QueryRow(ctx, `SELECT required_capability FROM node_jobs WHERE id=$1`, run.CreateJobID).Scan(&jobCapability); err != nil || jobCapability != "content_validation_v102" {
		t.Fatalf("capability %s %v", jobCapability, err)
	}
	if err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM allocations WHERE node_id=$1 AND state NOT IN ('reclaimed','released_no_effect')`, c.NodeID).Scan(&occupied); err != nil || occupied != 1 {
		t.Fatalf("occupied %d %v", occupied, err)
	}
	if _, err := s.ReserveValidation(ctx, c); err == nil {
		t.Fatal("second Run reserved before first reclaim")
	}
	if _, err := s.Pool.Exec(ctx, `UPDATE node_jobs SET required_capability='legacy_v1' WHERE id=$1`, run.CreateJobID); err == nil {
		t.Fatal("capability downgrade accepted")
	}
	if _, err := s.Pool.Exec(ctx, `UPDATE allocations SET purpose='player' WHERE id=$1`, run.AllocationID); err == nil {
		t.Fatal("Allocation identity changed")
	}
	if _, err := s.Pool.Exec(ctx, `UPDATE validation_runs SET content_version_id='other' WHERE id=$1`, run.ID); err == nil {
		t.Fatal("Run identity changed")
	}
	orphanID, _ := NewID()
	if _, err := s.Pool.Exec(ctx, `INSERT INTO validation_runs
		(id,started_by_admin_user_id,node_id,arcade_game_id,game_preset_id,content_version_id,content_sha256,
		template_revision_id,template_binding_key,template_fingerprint_sha256,template_binding_generation,
		maintenance_epoch,content_fact_revision,template_fact_revision)
		SELECT $1,started_by_admin_user_id,node_id,arcade_game_id,game_preset_id,content_version_id,content_sha256,
		template_revision_id,template_binding_key,template_fingerprint_sha256,template_binding_generation,
		maintenance_epoch,content_fact_revision,template_fact_revision FROM validation_runs WHERE id=$2`, orphanID, run.ID); err == nil {
		t.Fatal("orphan Run committed without Allocation/create Job")
	}
	if _, err := s.Pool.Exec(ctx, `UPDATE node_jobs SET state='claimed' WHERE id=$1`, run.CreateJobID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.PrepareCreate(ctx, c.NodeID, run.CreateJobID, "C:/versioned/template.json", 0); err == nil {
		t.Fatal("legacy prepare froze new Job")
	}
	if _, err := s.PrepareCreateWithManifest(ctx, c.NodeID, run.CreateJobID, "C:/versioned/template.json", 0, "template-manifest-sha256-v1", c.ExpectedTemplateFingerprintSHA256); err != nil {
		t.Fatal(err)
	}
	if frozen, err := s.FrozenCreateForJob(ctx, c.NodeID, run.CreateJobID); err != nil || frozen.RequiredCapability != "content_validation_v102" || frozen.TemplateFingerprintSHA256 != c.ExpectedTemplateFingerprintSHA256 {
		t.Fatalf("frozen capability %+v %v", frozen, err)
	}
}

func TestV102PlayerVsValidationLastSlot(t *testing.T) {
	s, c := v102ValidationFixture(t, 1)
	ctx := context.Background()
	otherGame, _ := NewID()
	otherPreset, _ := NewID()
	workshop := "9988776655"
	if _, err := s.Pool.Exec(ctx, `INSERT INTO arcade_games(id,workshop_id,display_name,current_content_version_id)
		VALUES($1,$2,'Other game',NULL)`, otherGame, workshop); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Pool.Exec(ctx, `INSERT INTO content_versions(id,arcade_game_id,content_sha256) VALUES('other-v1',$1,$2)`, otherGame, strings.Repeat("b", 64)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Pool.Exec(ctx, `UPDATE arcade_games SET current_content_version_id='other-v1' WHERE id=$1`, otherGame); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Pool.Exec(ctx, `INSERT INTO template_revisions(id,arcade_game_id) VALUES('other-template',$1)`, otherGame); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Pool.Exec(ctx, `INSERT INTO game_presets(id,arcade_game_id,display_name,max_players,template_revision_id)
		VALUES($1,$2,'Other preset',1,'other-template')`, otherPreset, otherGame); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Pool.Exec(ctx, `INSERT INTO node_template_bindings(node_id,template_revision_id,binding_key)
		VALUES($1,'other-template','other-binding')`, c.NodeID); err != nil {
		t.Fatal(err)
	}
	h := p1TestHeartbeat("test-v1")
	h.Content = []nodev1.ContentFact{{WorkshopID: workshop, ContentVersionID: "other-v1", VPKSHA256: strings.Repeat("b", 64), State: "confirmed"}, {WorkshopID: "3564393242", ContentVersionID: "test-v1", VPKSHA256: strings.Repeat("a", 64), State: "confirmed"}}
	if _, err := s.RecordHeartbeat(ctx, c.NodeID, h); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Pool.Exec(ctx, `UPDATE node_content_bindings SET accepting_new_allocations=true WHERE node_id=$1 AND arcade_game_id=$2`, c.NodeID, otherGame); err != nil {
		t.Fatal(err)
	}
	user, _, err := s.CreateUserSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.CreateUserRequest(ctx, user, otherGame, otherPreset); err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	var wg sync.WaitGroup
	var reserveErr, scheduleErr error
	wg.Add(2)
	go func() { defer wg.Done(); <-start; _, reserveErr = s.ReserveValidation(ctx, c) }()
	go func() { defer wg.Done(); <-start; _, scheduleErr = s.TryAllocateOne(ctx) }()
	close(start)
	wg.Wait()
	if scheduleErr != nil {
		t.Fatal(scheduleErr)
	}
	if reserveErr != nil && !errors.Is(reserveErr, ErrJobConflict) {
		t.Fatal(reserveErr)
	}
	var total int
	if err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM allocations WHERE node_id=$1 AND state NOT IN ('reclaimed','released_no_effect')`, c.NodeID).Scan(&total); err != nil || total != 1 {
		t.Fatalf("last-slot race occupied=%d reserve=%v schedule=%v err=%v", total, reserveErr, scheduleErr, err)
	}
	var jobs int
	if err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM node_jobs WHERE node_id=$1 AND kind='create'`, c.NodeID).Scan(&jobs); err != nil || jobs != 1 {
		t.Fatalf("create jobs=%d %v", jobs, err)
	}
}

func TestV102ValidationHumanPassRequiresFormalReclaim(t *testing.T) {
	s, c := v102ValidationFixture(t, 1)
	ctx := context.Background()
	run, err := s.ReserveValidation(ctx, c)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Pool.Exec(ctx, `UPDATE node_jobs SET state='claimed' WHERE id=$1`, run.CreateJobID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.PrepareCreateWithManifest(ctx, c.NodeID, run.CreateJobID, "C:/versioned/template.json", 0,
		"template-manifest-sha256-v1", c.ExpectedTemplateFingerprintSHA256); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReportJob(ctx, c.NodeID, run.CreateJobID, nodev1.ReportRequest{State: "accepted", InstanceID: "i_validation", OperationID: "o_create"}); err != nil {
		t.Fatal(err)
	}
	join := &nodev1.JoinInfo{LocalPort: 28000, PublicPort: 28000, ConnectHost: "127.0.0.1",
		EntryConfigRevision: nodev1.EntryConfigRevision(p1TestHeartbeat("test-v1").Network)}
	if _, err := s.ReportJob(ctx, c.NodeID, run.CreateJobID, nodev1.ReportRequest{State: "succeeded", InstanceID: "i_validation", OperationID: "o_create", JoinInfo: join}); err != nil {
		t.Fatal(err)
	}
	run, err = s.ValidationRun(ctx, run.ID)
	if err != nil || run.State != "awaiting_human" || run.ReadyAt == nil || run.JoinInfoAvailableAt == nil {
		t.Fatalf("ready evidence %+v %v", run, err)
	}
	run, err = s.ConfirmValidationHuman(ctx, run.ID, c.AdminID, "pass")
	if err != nil {
		t.Fatal(err)
	}
	if run.State != "stop_pending" || run.PassedAt != nil || run.StopJobID == "" {
		t.Fatalf("human claim became PASS %+v", run)
	}
	if _, err := s.FinalizeValidationRun(ctx, run.ID); err == nil {
		t.Fatal("PASS before reclaim")
	}
	if _, err := s.Pool.Exec(ctx, `UPDATE node_jobs SET state='claimed' WHERE id=$1`, run.StopJobID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReportJob(ctx, c.NodeID, run.StopJobID, nodev1.ReportRequest{State: "accepted", InstanceID: "i_validation", OperationID: "o_stop"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.FinalizeValidationRun(ctx, run.ID); err == nil {
		t.Fatal("PASS before stop completion")
	}
	if _, err := s.ReportJob(ctx, c.NodeID, run.StopJobID, nodev1.ReportRequest{State: "succeeded", InstanceID: "i_validation", OperationID: "o_stop"}); err != nil {
		t.Fatal(err)
	}
	run, err = s.FinalizeValidationRun(ctx, run.ID)
	if err != nil || run.State != "passed" || run.PassedAt == nil {
		t.Fatalf("final PASS %+v %v", run, err)
	}
	var occupied int
	if err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM allocations WHERE node_id=$1 AND state NOT IN ('reclaimed','released_no_effect')`, c.NodeID).Scan(&occupied); err != nil || occupied != 0 {
		t.Fatalf("capacity after reclaim %d %v", occupied, err)
	}
}

func TestV102UnknownCreateKeepsCapacity(t *testing.T) {
	s, c := v102ValidationFixture(t, 1)
	ctx := context.Background()
	run, err := s.ReserveValidation(ctx, c)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Pool.Exec(ctx, `UPDATE node_jobs SET state='claimed' WHERE id=$1`, run.CreateJobID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.PrepareCreateWithManifest(ctx, c.NodeID, run.CreateJobID, "C:/versioned/template.json", 0,
		"template-manifest-sha256-v1", c.ExpectedTemplateFingerprintSHA256); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReportJob(ctx, c.NodeID, run.CreateJobID, nodev1.ReportRequest{State: "unknown"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReportJob(ctx, c.NodeID, run.CreateJobID, nodev1.ReportRequest{State: "rejected_no_effect", ErrorCode: "LOCAL_REJECTION"}); err == nil {
		t.Fatal("unknown create released as no effect")
	}
	run, err = s.ValidationRun(ctx, run.ID)
	if err != nil || run.State != "create_unknown" {
		t.Fatalf("unknown state %+v %v", run, err)
	}
	if _, err := s.FinalizeValidationRun(ctx, run.ID); err == nil {
		t.Fatal("unknown Run passed")
	}
	var occupied int
	if err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM allocations WHERE node_id=$1 AND state NOT IN ('reclaimed','released_no_effect')`, c.NodeID).Scan(&occupied); err != nil || occupied != 1 {
		t.Fatalf("unknown capacity %d %v", occupied, err)
	}
}

func TestV102EffectfulFailureNeedsStopBeforeFail(t *testing.T) {
	s, c := v102ValidationFixture(t, 1)
	ctx := context.Background()
	run, err := s.ReserveValidation(ctx, c)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Pool.Exec(ctx, `UPDATE node_jobs SET state='claimed' WHERE id=$1`, run.CreateJobID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.PrepareCreateWithManifest(ctx, c.NodeID, run.CreateJobID, "C:/versioned/template.json", 0,
		"template-manifest-sha256-v1", c.ExpectedTemplateFingerprintSHA256); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReportJob(ctx, c.NodeID, run.CreateJobID, nodev1.ReportRequest{State: "accepted", InstanceID: "i_failed", OperationID: "o_create"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReportJob(ctx, c.NodeID, run.CreateJobID, nodev1.ReportRequest{State: "failed_with_effect", InstanceID: "i_failed", OperationID: "o_create", ErrorCode: "STARTUP_FAILED"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.FinalizeValidationFailure(ctx, run.ID, "STARTUP_FAILED"); err == nil {
		t.Fatal("effectful failure released before stop")
	}
	run, err = s.ValidationRun(ctx, run.ID)
	if err != nil || run.StopJobID == "" {
		t.Fatalf("automatic stop missing %+v %v", run, err)
	}
	if _, err := s.Pool.Exec(ctx, `UPDATE node_jobs SET state='claimed' WHERE id=$1`, run.StopJobID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReportJob(ctx, c.NodeID, run.StopJobID, nodev1.ReportRequest{State: "accepted", InstanceID: "i_failed", OperationID: "o_stop"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReportJob(ctx, c.NodeID, run.StopJobID, nodev1.ReportRequest{State: "succeeded", InstanceID: "i_failed", OperationID: "o_stop"}); err != nil {
		t.Fatal(err)
	}
	run, err = s.FinalizeValidationFailure(ctx, run.ID, "STARTUP_FAILED")
	if err != nil || run.State != "failed" || run.ResultCode != "FAIL" {
		t.Fatalf("failed Run %+v %v", run, err)
	}
	var occupied int
	if err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM allocations WHERE node_id=$1 AND state NOT IN ('reclaimed','released_no_effect')`, c.NodeID).Scan(&occupied); err != nil || occupied != 0 {
		t.Fatalf("capacity after failure reclaim %d %v", occupied, err)
	}
}

func TestV102LegacyContentActionGuard(t *testing.T) {
	s := playerTestStore(t)
	ctx := context.Background()
	admin, err := s.CreateAdmin(ctx, "guard-admin", "a long test password")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.ApplyAdminAction(ctx, admin, AdminAction{Action: "game.create", WorkshopID: "1122334455", DisplayName: "New game"}); err != nil {
		t.Fatal(err)
	}
	var game string
	if err := s.Pool.QueryRow(ctx, `SELECT id FROM arcade_games WHERE workshop_id='1122334455'`).Scan(&game); err != nil {
		t.Fatal(err)
	}
	if err := s.ApplyAdminAction(ctx, admin, AdminAction{Action: "template.create", TargetID: game, TemplateRevisionID: "new-template"}); err != nil {
		t.Fatal(err)
	}
	if err := s.ApplyAdminAction(ctx, admin, AdminAction{Action: "content.create", TargetID: game, ContentVersionID: "new-v1", ContentSHA256: strings.Repeat("a", 64)}); err != nil {
		t.Fatal(err)
	}
	if err := s.ApplyAdminAction(ctx, admin, AdminAction{Action: "preset.create", TargetID: game, DisplayName: "new preset", TemplateRevisionID: "new-template", MaxPlayers: 1}); err != nil {
		t.Fatal(err)
	}
	var preset, contract string
	if err := s.Pool.QueryRow(ctx, `SELECT id,validation_contract FROM game_presets WHERE arcade_game_id=$1`, game).Scan(&preset, &contract); err != nil || contract != "v1_0_2" {
		t.Fatalf("new contract=%s %v", contract, err)
	}
	if err := s.ApplyAdminAction(ctx, admin, AdminAction{Action: "content.validate", TargetID: game, GameID: game, ContentVersionID: "new-v1", Confirmed: true}); !errors.Is(err, ErrJobConflict) {
		t.Fatalf("old validate accepted: %v", err)
	}
	if err := s.ApplyAdminAction(ctx, admin, AdminAction{Action: "content.publish", TargetID: game, ContentVersionID: "new-v1", Confirmed: true}); !errors.Is(err, ErrJobConflict) {
		t.Fatalf("old publish accepted: %v", err)
	}
	if _, err := s.Pool.Exec(ctx, `UPDATE game_presets SET validation_contract='legacy_v1' WHERE id=$1`, preset); err == nil {
		t.Fatal("contract downgrade accepted")
	}
	var runs int
	if err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM validation_runs`).Scan(&runs); err != nil || runs != 0 {
		t.Fatalf("legacy proof promoted: %d %v", runs, err)
	}
}
