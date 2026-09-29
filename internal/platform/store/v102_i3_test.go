package store

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/L4C99/dota2-arcade-platform/internal/contracts/nodev1"
)

func i3PassRun(t *testing.T, s *Store, c ValidationCandidate) ValidationRun {
	t.Helper()
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
	beginV102TestOperation(t, s, c.NodeID, run.CreateJobID)
	if _, err := s.ReportJob(ctx, c.NodeID, run.CreateJobID, nodev1.ReportRequest{State: "accepted", InstanceID: "i_validation", OperationID: "o_create"}); err != nil {
		t.Fatal(err)
	}
	heartbeat := p1TestHeartbeat("test-v1")
	heartbeat.HardMaxInstances = 2
	heartbeat.Network.LocalPortMax = heartbeat.Network.LocalPortMin + 1
	join := &nodev1.JoinInfo{LocalPort: 28000, PublicPort: 28000, ConnectHost: "127.0.0.1",
		EntryConfigRevision: nodev1.EntryConfigRevision(heartbeat.Network)}
	if _, err := s.ReportJob(ctx, c.NodeID, run.CreateJobID, nodev1.ReportRequest{State: "succeeded", InstanceID: "i_validation", OperationID: "o_create", JoinInfo: join}); err != nil {
		t.Fatal(err)
	}
	run, err = s.ConfirmValidationHuman(ctx, run.ID, c.AdminID, "pass", "awaiting_human")
	if err != nil {
		t.Fatal(err)
	}
	if run.State == "passed" {
		t.Fatal("human claim became final PASS")
	}
	if _, err := s.Pool.Exec(ctx, `UPDATE node_jobs SET state='claimed' WHERE id=$1`, run.StopJobID); err != nil {
		t.Fatal(err)
	}
	beginV102TestOperation(t, s, c.NodeID, run.StopJobID)
	if _, err := s.ReportJob(ctx, c.NodeID, run.StopJobID, nodev1.ReportRequest{State: "accepted", InstanceID: "i_validation", OperationID: "o_stop"}); err != nil {
		t.Fatal(err)
	}
	if err := s.ReportInstanceFact(ctx, c.NodeID, run.AllocationID, v102ReclaimedFact()); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReportJob(ctx, c.NodeID, run.StopJobID, nodev1.ReportRequest{State: "succeeded", InstanceID: "i_validation", OperationID: "o_stop"}); err != nil {
		t.Fatal(err)
	}
	if err := s.FinalizeReadyValidations(ctx); err != nil {
		t.Fatal(err)
	}
	run, err = s.ValidationRun(ctx, run.ID)
	if err != nil || run.State != "passed" {
		t.Fatalf("automatic finalizer %+v %v", run, err)
	}
	return run
}

func i3Publish(t *testing.T, s *Store, c ValidationCandidate, run ValidationRun, more ...ReleasePresetPlan) string {
	t.Helper()
	ctx := context.Background()
	var oldAccept bool
	if err := s.Pool.QueryRow(ctx, `SELECT accepting_new_requests FROM game_presets WHERE id=$1`, c.PresetID).Scan(&oldAccept); err != nil {
		t.Fatal(err)
	}
	plan := []ReleasePresetPlan{{PresetID: c.PresetID, ExpectedOldTemplateRevisionID: c.TemplateRevisionID,
		NewTemplateRevisionID: c.TemplateRevisionID, ExpectedOldAccepting: oldAccept, NewAccepting: true, ValidationRunID: run.ID}}
	plan = append(plan, more...)
	id, err := s.PublishRelease(ctx, c.AdminID, ReleasePublishRequest{GameID: c.GameID,
		ExpectedOldContentVersionID: "test-v1", NewContentVersionID: "test-v1", Presets: plan, RequestID: "release-i3"})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestI3NoPassBlocksPlayerAndBindingReopen(t *testing.T) {
	s, c := v102ValidationFixture(t, 2)
	ctx := context.Background()
	if _, err := s.Pool.Exec(ctx, `UPDATE game_presets SET validation_contract='v1_0_2' WHERE id=$1`, c.PresetID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Pool.Exec(ctx, `UPDATE node_content_bindings SET accepting_new_allocations=true WHERE node_id=$1 AND arcade_game_id=$2`, c.NodeID, c.GameID); err != nil {
		t.Fatal(err)
	}
	user, _, err := s.CreateUserSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.CreateUserRequest(ctx, user, c.GameID, c.PresetID); err != nil {
		t.Fatal(err)
	}
	if changed, err := s.TryAllocateOne(ctx); err != nil || changed {
		t.Fatalf("no PASS allocated: %t %v", changed, err)
	}
	if _, err := s.Pool.Exec(ctx, `UPDATE node_content_bindings SET accepting_new_allocations=false WHERE node_id=$1 AND arcade_game_id=$2`, c.NodeID, c.GameID); err != nil {
		t.Fatal(err)
	}
	if err := s.ApplyAdminAction(ctx, c.AdminID, AdminAction{Action: "binding.update", TargetID: c.NodeID, GameID: c.GameID, Accepting: boolPtr(true)}); !errors.Is(err, ErrValidationIncomplete) {
		t.Fatalf("binding bypass: %v", err)
	}
}

func TestI3FirstReleasePerPresetAndPlayerCapability(t *testing.T) {
	s, c := v102ValidationFixture(t, 2)
	ctx := context.Background()
	p2, _ := NewID()
	if _, err := s.Pool.Exec(ctx, `INSERT INTO game_presets(id,arcade_game_id,display_name,max_players,template_revision_id,validation_contract,enabled,accepting_new_requests)
		VALUES($1,$2,'P2',10,'test-template','legacy_v1',true,false)`, p2, c.GameID); err != nil {
		t.Fatal(err)
	}
	run := i3PassRun(t, s, c)
	var oldAccept bool
	if err := s.Pool.QueryRow(ctx, `SELECT accepting_new_requests FROM game_presets WHERE id=$1`, c.PresetID).Scan(&oldAccept); err != nil {
		t.Fatal(err)
	}
	first := ReleasePublishRequest{GameID: c.GameID, ExpectedOldContentVersionID: "test-v1", NewContentVersionID: "test-v1",
		Presets: []ReleasePresetPlan{{PresetID: c.PresetID, ExpectedOldTemplateRevisionID: "test-template", NewTemplateRevisionID: "test-template",
			ExpectedOldAccepting: oldAccept, NewAccepting: true, ValidationRunID: run.ID}}, RequestID: "first-release"}
	if _, err := s.PublishRelease(ctx, c.AdminID, first); !errors.Is(err, ErrCASConflict) {
		t.Fatalf("omitted P2 accepted: %v", err)
	}
	first.Presets = append(first.Presets, ReleasePresetPlan{PresetID: p2, ExpectedOldTemplateRevisionID: "test-template", NewTemplateRevisionID: "test-template", ExpectedOldAccepting: false, NewAccepting: false})
	id, err := s.PublishRelease(ctx, c.AdminID, first)
	if err != nil {
		t.Fatal(err)
	}
	if duplicate, err := s.PublishRelease(ctx, c.AdminID, first); err != nil || duplicate != id {
		t.Fatalf("release retry %q %v", duplicate, err)
	}
	first.NewContentVersionID = "other"
	if _, err := s.PublishRelease(ctx, c.AdminID, first); !errors.Is(err, ErrCASConflict) {
		t.Fatalf("request ID reused with other payload: %v", err)
	}
	var count int
	if err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM game_presets WHERE arcade_game_id=$1 AND validation_contract='v1_0_2'`, c.GameID).Scan(&count); err != nil || count != 2 {
		t.Fatalf("contracts %d %v", count, err)
	}
	if err := s.ApplyAdminAction(ctx, c.AdminID, AdminAction{Action: "binding.update", TargetID: c.NodeID, GameID: c.GameID, Accepting: boolPtr(true)}); err != nil {
		t.Fatal(err)
	}
	user, _, err := s.CreateUserSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.CreateUserRequest(ctx, user, c.GameID, c.PresetID); err != nil {
		t.Fatal(err)
	}
	if changed, err := s.TryAllocateOne(ctx); err != nil || !changed {
		t.Fatalf("valid PASS refused: %t %v", changed, err)
	}
	var capability, fingerprint, workshop, content, sha string
	var generation int64
	if err := s.Pool.QueryRow(ctx, `SELECT required_capability,expected_template_fingerprint_sha256,template_binding_generation,
		expected_workshop_id,expected_content_version_id,expected_vpk_sha256 FROM node_jobs WHERE allocation_id=(
		SELECT id FROM allocations WHERE server_request_id IS NOT NULL ORDER BY assigned_at DESC LIMIT 1)`).
		Scan(&capability, &fingerprint, &generation, &workshop, &content, &sha); err != nil {
		t.Fatal(err)
	}
	if capability != "content_validation_v102" || fingerprint != c.ExpectedTemplateFingerprintSHA256 || generation == 0 || workshop != "3564393242" || content != "test-v1" || sha != strings.Repeat("a", 64) {
		t.Fatalf("player frozen identity %s %s %d %s %s %s", capability, fingerprint, generation, workshop, content, sha)
	}
}

func TestI3ValidationStartRequestDurabilityAndProofDrift(t *testing.T) {
	s, c := v102ValidationFixture(t, 2)
	ctx := context.Background()
	c.RequestID = "validation-i3"
	run := i3PassRun(t, s, c)
	if duplicate, err := s.ReserveValidation(ctx, c); err != nil || duplicate.ID != run.ID {
		t.Fatalf("start retry %+v %v", duplicate, err)
	}
	changed := c
	changed.ExpectedMaintenanceEpoch++
	if _, err := s.ReserveValidation(ctx, changed); !errors.Is(err, ErrCASConflict) {
		t.Fatalf("start request mutation: %v", err)
	}
	id := i3Publish(t, s, c, run)
	if id == "" {
		t.Fatal("missing release")
	}
	if err := s.ApplyAdminAction(ctx, c.AdminID, AdminAction{Action: "binding.update", TargetID: c.NodeID, GameID: c.GameID, Accepting: boolPtr(true)}); err != nil {
		t.Fatal(err)
	}
	check := func(want bool) {
		t.Helper()
		tx, err := s.Pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)
		id, err := effectiveValidationProof(ctx, tx, c.NodeID, c.GameID, c.PresetID, c.ContentVersionID, c.TemplateRevisionID, run.ID)
		if err != nil || (id != "") != want {
			t.Fatalf("effective proof %q want %t: %v", id, want, err)
		}
	}
	check(true)
	if _, err := s.Pool.Exec(ctx, `UPDATE node_content_bindings SET content_fact_revision=content_fact_revision+1 WHERE node_id=$1 AND arcade_game_id=$2`, c.NodeID, c.GameID); err != nil {
		t.Fatal(err)
	}
	check(false)
	if err := s.ApplyAdminAction(ctx, c.AdminID, AdminAction{Action: "binding.update", TargetID: c.NodeID, GameID: c.GameID, Accepting: boolPtr(true)}); !errors.Is(err, ErrValidationIncomplete) {
		t.Fatalf("stale binding reopen: %v", err)
	}
	if err := s.ApplyAdminAction(ctx, c.AdminID, AdminAction{Action: "binding.update", TargetID: c.NodeID, GameID: c.GameID, Accepting: boolPtr(false)}); err != nil {
		t.Fatal(err)
	}
	if err := s.ApplyAdminAction(ctx, c.AdminID, AdminAction{Action: "preset.update", TargetID: c.PresetID, Accepting: boolPtr(false)}); err != nil {
		t.Fatal(err)
	}
	if err := s.ApplyAdminAction(ctx, c.AdminID, AdminAction{Action: "preset.update", TargetID: c.PresetID, Accepting: boolPtr(true)}); !errors.Is(err, ErrValidationIncomplete) {
		t.Fatalf("stale preset reopen: %v", err)
	}
	if _, err := s.PublishRelease(ctx, c.AdminID, ReleasePublishRequest{GameID: c.GameID, ExpectedOldContentVersionID: "test-v1", NewContentVersionID: "test-v1",
		Presets: []ReleasePresetPlan{{PresetID: c.PresetID, ExpectedOldTemplateRevisionID: "test-template", NewTemplateRevisionID: "test-template", ExpectedOldAccepting: false, NewAccepting: true, ValidationRunID: run.ID}}, RequestID: "stale-release"}); !errors.Is(err, ErrValidationIncomplete) {
		t.Fatalf("stale release: %v", err)
	}
}
