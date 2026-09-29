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

func TestI3TemplateOnlyPublicationUsesNewPresetProof(t *testing.T) {
	s, c := v102ValidationFixture(t, 2)
	ctx := context.Background()
	firstRun := i3PassRun(t, s, c)
	_ = i3Publish(t, s, c, firstRun)
	if _, err := s.Pool.Exec(ctx, `INSERT INTO template_revisions(id,arcade_game_id) VALUES('test-template-2',$1)`, c.GameID); err != nil {
		t.Fatal(err)
	}
	if generation, err := s.UpsertTemplateBinding(ctx, c.NodeID, "test-template-2", "binding-2", c.ExpectedTemplateFingerprintSHA256, 0); err != nil || generation != 1 {
		t.Fatalf("new template binding %d %v", generation, err)
	}
	if _, err := s.RecordTemplateFact(ctx, c.NodeID, "test-template-2", "binding-2", "confirmed", "template-manifest-sha256-v1", c.ExpectedTemplateFingerprintSHA256); err != nil {
		t.Fatal(err)
	}
	if epoch, err := s.BeginScopedMaintenance(ctx, c.NodeID, c.GameID, c.AdminID, "template-maintenance", "template-only", 1); err != nil || epoch != 2 {
		t.Fatalf("maintenance epoch %d %v", epoch, err)
	}
	newCandidate := c
	newCandidate.TemplateRevisionID = "test-template-2"
	newCandidate.ExpectedMaintenanceEpoch = 2
	newRun := i3PassRun(t, s, newCandidate)
	request := ReleasePublishRequest{GameID: c.GameID, ExpectedOldContentVersionID: "test-v1", NewContentVersionID: "test-v1",
		Presets: []ReleasePresetPlan{{PresetID: c.PresetID, ExpectedOldTemplateRevisionID: "test-template", NewTemplateRevisionID: "test-template-2",
			ExpectedOldAccepting: true, NewAccepting: true, ValidationRunID: firstRun.ID}}, RequestID: "template-only"}
	if _, err := s.PublishRelease(ctx, c.AdminID, request); !errors.Is(err, ErrValidationIncomplete) {
		t.Fatalf("old combo proof accepted: %v", err)
	}
	request.Presets[0].ValidationRunID = newRun.ID
	if _, err := s.PublishRelease(ctx, c.AdminID, request); err != nil {
		t.Fatal(err)
	}
	var revision string
	if err := s.Pool.QueryRow(ctx, `SELECT template_revision_id FROM game_presets WHERE id=$1`, c.PresetID).Scan(&revision); err != nil || revision != "test-template-2" {
		t.Fatalf("published template %q %v", revision, err)
	}
}

func TestI3ContentChangeAndRollbackNeedAllPresetsAndNewEpochProof(t *testing.T) {
	s, c := v102ValidationFixture(t, 2)
	ctx := context.Background()
	p2, _ := NewID()
	if _, err := s.Pool.Exec(ctx, `INSERT INTO game_presets(id,arcade_game_id,display_name,max_players,template_revision_id,validation_contract,enabled,accepting_new_requests)
		VALUES($1,$2,'P2',10,'test-template','legacy_v1',true,false)`, p2, c.GameID); err != nil {
		t.Fatal(err)
	}
	firstRun := i3PassRun(t, s, c)
	firstID := i3Publish(t, s, c, firstRun, ReleasePresetPlan{PresetID: p2, ExpectedOldTemplateRevisionID: "test-template",
		NewTemplateRevisionID: "test-template", ExpectedOldAccepting: false, NewAccepting: false})
	if _, err := s.Pool.Exec(ctx, `INSERT INTO content_versions(id,arcade_game_id,content_sha256) VALUES('test-v2',$1,$2)`, c.GameID, strings.Repeat("b", 64)); err != nil {
		t.Fatal(err)
	}
	advance := func(requestID, version, sha string, oldEpoch int64) ValidationRun {
		t.Helper()
		if epoch, err := s.BeginScopedMaintenance(ctx, c.NodeID, c.GameID, c.AdminID, requestID, "content switch", oldEpoch); err != nil || epoch != oldEpoch+1 {
			t.Fatalf("maintenance epoch %d %v", epoch, err)
		}
		h := p1TestHeartbeat(version)
		h.HardMaxInstances = 2
		h.Network.LocalPortMax = h.Network.LocalPortMin + 1
		h.Content[0].VPKSHA256 = sha
		h.InventoryScanID, h.InventoryState = "second-scan", "confirmed"
		h.TemplateFacts = []nodev1.TemplateFact{{BindingKey: "test-binding", State: "confirmed", ManifestAlgorithm: nodev1.TemplateManifestAlgorithmV1,
			FingerprintSHA256: c.ExpectedTemplateFingerprintSHA256}}
		h.Capabilities = []string{nodev1.CapabilityContentValidationV102, nodev1.CapabilityTemplateManifestV1, nodev1.CapabilityCoreInventoryV1}
		if _, err := s.RecordHeartbeat(ctx, c.NodeID, h); err != nil {
			t.Fatal(err)
		}
		candidate := c
		candidate.ContentVersionID = version
		candidate.ExpectedMaintenanceEpoch = oldEpoch + 1
		return i3PassRun(t, s, candidate)
	}
	v2Run := advance("v2-maintenance", "test-v2", strings.Repeat("b", 64), 1)
	change := ReleasePublishRequest{GameID: c.GameID, ExpectedOldContentVersionID: "test-v1", NewContentVersionID: "test-v2",
		Presets: []ReleasePresetPlan{{PresetID: c.PresetID, ExpectedOldTemplateRevisionID: "test-template", NewTemplateRevisionID: "test-template",
			ExpectedOldAccepting: true, NewAccepting: true, ValidationRunID: v2Run.ID}}, RequestID: "content-change"}
	if _, err := s.PublishRelease(ctx, c.AdminID, change); !errors.Is(err, ErrCASConflict) {
		t.Fatalf("omitted P2 on content change: %v", err)
	}
	change.Presets = append(change.Presets, ReleasePresetPlan{PresetID: p2, ExpectedOldTemplateRevisionID: "test-template",
		NewTemplateRevisionID: "test-template", ExpectedOldAccepting: false, NewAccepting: false})
	v2Release, err := s.PublishRelease(ctx, c.AdminID, change)
	if err != nil {
		t.Fatal(err)
	}
	rollback := change
	rollback.ExpectedOldContentVersionID, rollback.NewContentVersionID = "test-v2", "test-v1"
	rollback.RollbackOfReleaseID, rollback.RequestID = v2Release, "rollback"
	rollback.Presets[0].ValidationRunID = firstRun.ID
	if _, err := s.PublishRelease(ctx, c.AdminID, rollback); !errors.Is(err, ErrValidationIncomplete) {
		t.Fatalf("old epoch rollback proof accepted: %v", err)
	}
	v1Run := advance("v1-maintenance", "test-v1", strings.Repeat("a", 64), 2)
	rollback.Presets[0].ValidationRunID = v1Run.ID
	rollbackID, err := s.PublishRelease(ctx, c.AdminID, rollback)
	if err != nil {
		t.Fatal(err)
	}
	row, err := s.ReleaseDetail(ctx, rollbackID)
	if err != nil || row.RollbackOfReleaseID != v2Release || row.NewContentVersionID != "test-v1" {
		t.Fatalf("rollback record %+v %v", row, err)
	}
	first, err := s.ReleaseDetail(ctx, firstID)
	if err != nil || first.NewContentVersionID != "test-v1" || first.RollbackOfReleaseID != "" {
		t.Fatalf("original release was changed %+v %v", first, err)
	}
}

func TestI3ProofDriftClosesEveryAdmissionPath(t *testing.T) {
	for _, drift := range []struct {
		name string
		make func(*testing.T, *Store, ValidationCandidate)
	}{
		{"maintenance epoch", func(t *testing.T, s *Store, c ValidationCandidate) {
			_, err := s.Pool.Exec(context.Background(), `UPDATE node_content_bindings SET maintenance_epoch=maintenance_epoch+1 WHERE node_id=$1 AND arcade_game_id=$2`, c.NodeID, c.GameID)
			if err != nil {
				t.Fatal(err)
			}
		}},
		{"content fact revision", func(t *testing.T, s *Store, c ValidationCandidate) {
			_, err := s.Pool.Exec(context.Background(), `UPDATE node_content_bindings SET content_fact_revision=content_fact_revision+1 WHERE node_id=$1 AND arcade_game_id=$2`, c.NodeID, c.GameID)
			if err != nil {
				t.Fatal(err)
			}
		}},
		{"template binding generation", func(t *testing.T, s *Store, c ValidationCandidate) {
			if _, err := s.UpsertTemplateBinding(context.Background(), c.NodeID, c.TemplateRevisionID, "new-binding-key", c.ExpectedTemplateFingerprintSHA256, 2); err != nil {
				t.Fatal(err)
			}
		}},
		{"template fact revision", func(t *testing.T, s *Store, c ValidationCandidate) {
			if _, err := s.RecordTemplateFact(context.Background(), c.NodeID, c.TemplateRevisionID, "test-binding", "confirmed", "template-manifest-sha256-v1", strings.Repeat("e", 64)); err != nil {
				t.Fatal(err)
			}
		}},
		{"unaccounted inventory", func(t *testing.T, s *Store, c ValidationCandidate) {
			ctx := context.Background()
			if _, err := s.RecordInventory(ctx, c.NodeID, "unaccounted-scan", true, "", []InventoryInstance{{InstanceID: "stray", Lifecycle: "active", Process: "running", Cleanup: "incomplete"}}); err != nil {
				t.Fatal(err)
			}
			if _, err := s.Pool.Exec(ctx, `UPDATE node_reports SET inventory_scan_id='unaccounted-scan',inventory_state='confirmed' WHERE node_id=$1`, c.NodeID); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run(drift.name, func(t *testing.T) {
			s, c := v102ValidationFixture(t, 2)
			ctx := context.Background()
			run := i3PassRun(t, s, c)
			_ = i3Publish(t, s, c, run)
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
			drift.make(t, s, c)
			if changed, err := s.TryAllocateOne(ctx); err != nil || changed {
				t.Fatalf("scheduler used stale proof: %t %v", changed, err)
			}
			if err := s.ApplyAdminAction(ctx, c.AdminID, AdminAction{Action: "binding.update", TargetID: c.NodeID, GameID: c.GameID, Accepting: boolPtr(true)}); !errors.Is(err, ErrValidationIncomplete) {
				t.Fatalf("binding reopened on stale proof: %v", err)
			}
			if err := s.ApplyAdminAction(ctx, c.AdminID, AdminAction{Action: "preset.update", TargetID: c.PresetID, Accepting: boolPtr(true)}); !errors.Is(err, ErrValidationIncomplete) {
				t.Fatalf("preset reopened on stale proof: %v", err)
			}
			if err := s.ApplyAdminAction(ctx, c.AdminID, AdminAction{Action: "binding.update", TargetID: c.NodeID, GameID: c.GameID, Accepting: boolPtr(false)}); err != nil {
				t.Fatal(err)
			}
			_, err = s.PublishRelease(ctx, c.AdminID, ReleasePublishRequest{GameID: c.GameID, ExpectedOldContentVersionID: "test-v1", NewContentVersionID: "test-v1",
				Presets: []ReleasePresetPlan{{PresetID: c.PresetID, ExpectedOldTemplateRevisionID: "test-template", NewTemplateRevisionID: "test-template", ExpectedOldAccepting: true, NewAccepting: true, ValidationRunID: run.ID}}, RequestID: "stale-proof"})
			if !errors.Is(err, ErrValidationIncomplete) {
				t.Fatalf("release used stale proof: %v", err)
			}
		})
	}
}

func TestI3ValidationRejectsCrossGameCandidateAndPrematureHumanActions(t *testing.T) {
	s, c := v102ValidationFixture(t, 2)
	ctx := context.Background()
	otherGame, _ := NewID()
	otherPreset, _ := NewID()
	if _, err := s.Pool.Exec(ctx, `INSERT INTO arcade_games(id,workshop_id,display_name) VALUES($1,'99999999','Other')`, otherGame); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Pool.Exec(ctx, `INSERT INTO content_versions(id,arcade_game_id,content_sha256) VALUES('other-content',$1,$2)`, otherGame, strings.Repeat("b", 64)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Pool.Exec(ctx, `INSERT INTO template_revisions(id,arcade_game_id) VALUES('other-template',$1)`, otherGame); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Pool.Exec(ctx, `INSERT INTO game_presets(id,arcade_game_id,display_name,max_players,template_revision_id) VALUES($1,$2,'Other preset',10,'other-template')`, otherPreset, otherGame); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*ValidationCandidate){
		func(x *ValidationCandidate) { x.PresetID = otherPreset },
		func(x *ValidationCandidate) { x.ContentVersionID = "other-content" },
		func(x *ValidationCandidate) { x.TemplateRevisionID = "other-template" },
	} {
		wrong := c
		change(&wrong)
		if run, err := s.ReserveValidation(ctx, wrong); err == nil {
			t.Fatalf("cross-game candidate created Run %+v", run)
		}
	}
	run, err := s.ReserveValidation(ctx, c)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.ConfirmValidationHuman(ctx, run.ID, c.AdminID, "pass", "awaiting_human"); !errors.Is(err, ErrCASConflict) {
		t.Fatalf("premature human pass: %v", err)
	}
	if _, err := s.StopValidation(ctx, run.ID, c.AdminID, "create_pending"); !errors.Is(err, ErrCASConflict) {
		t.Fatalf("stop guessed instance: %v", err)
	}
}
