package store

import (
	"context"
	"errors"
	"testing"

	"github.com/L4C99/dota2-arcade-platform/internal/contracts/nodev1"
)

func boolPtr(x bool) *bool       { return &x }
func intPtr(x int) *int          { return &x }
func stringPtr(x string) *string { return &x }

func TestP4DAdminControlsAuditAndControllerFacts(t *testing.T) {
	s := playerTestStore(t)
	ctx := context.Background()
	gameID, presetID := seedPlayerCatalog(t, s)
	nodeID, _, err := s.RegisterNode(ctx, "P4D node", "linux")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.RecordHeartbeat(ctx, nodeID, p1TestHeartbeat("test-v1")); err != nil {
		t.Fatal(err)
	}
	adminID, err := s.CreateAdmin(ctx, "p4d-admin", "a long test password")
	if err != nil {
		t.Fatal(err)
	}
	actions := []AdminAction{
		{Action: "global.update", Accepting: boolPtr(false), Message: stringPtr("maintenance")},
		{Action: "announcement.update", Message: stringPtr("planned work")},
		{Action: "game.update", TargetID: gameID, Accepting: boolPtr(false), Enabled: boolPtr(false), Message: stringPtr("paused")},
		{Action: "preset.update", TargetID: presetID, Accepting: boolPtr(false)},
		{Action: "node.update", TargetID: nodeID, Draining: boolPtr(true), Priority: intPtr(5), Desired: intPtr(1)},
		{Action: "binding.update", TargetID: nodeID, GameID: gameID, Accepting: boolPtr(true)},
	}
	for _, a := range actions {
		if err := s.ApplyAdminAction(ctx, adminID, a); err != nil {
			t.Fatalf("%s: %v", a.Action, err)
		}
	}
	o, err := s.AdminOverview(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if o.Settings.AcceptingNewRequests || o.Settings.MaintenanceMessage != "maintenance" || o.Settings.SiteAnnouncement != "planned work" {
		t.Fatalf("settings: %+v", o.Settings)
	}
	if catalog, err := s.PlayerCatalog(ctx); err != nil || catalog.SiteAnnouncement != "planned work" {
		t.Fatalf("player announcement: %+v %v", catalog, err)
	}
	if len(o.Games) != 1 || o.Games[0].Enabled || o.Games[0].CurrentContentVersionID != "test-v1" || o.Presets[0].AcceptingNewRequests {
		t.Fatal("game/preset controls or content target")
	}
	if len(o.Nodes) != 1 || !o.Nodes[0].Draining || o.Nodes[0].Priority != 5 || o.Nodes[0].Desired != 1 || o.Nodes[0].Hard != 1 {
		t.Fatalf("node: %+v", o.Nodes)
	}
	if len(o.Bindings) != 1 || !o.Bindings[0].AcceptingNewAllocations || o.Bindings[0].ReportedContentVersionID != "test-v1" || o.Bindings[0].ReportedState != "confirmed" {
		t.Fatalf("Controller facts changed: %+v", o.Bindings)
	}
	if len(o.Audit) != len(actions)+1 || o.Audit[0].ActorUsername != "p4d-admin" || o.Audit[0].Result != "succeeded" {
		t.Fatalf("audit=%+v", o.Audit)
	}
	if err := s.ApplyAdminAction(ctx, adminID, AdminAction{Action: "node.update", TargetID: nodeID, Desired: intPtr(2)}); !errors.Is(err, ErrInvalidDesiredCapacity) {
		t.Fatalf("oversize capacity: %v", err)
	}
	if err := s.ApplyAdminAction(ctx, adminID, AdminAction{Action: "entry.update", TargetID: nodeID, Entry: "steam", Enabled: boolPtr(true)}); !errors.Is(err, ErrInvalidAdminAction) {
		t.Fatalf("unverified entry: %v", err)
	}
	if err := s.ApplyAdminAction(ctx, adminID, AdminAction{Action: "entry.update", TargetID: nodeID, Entry: "steam", Verified: boolPtr(true), Confirmed: true}); !errors.Is(err, ErrInvalidAdminAction) {
		t.Fatalf("no A2S fact verification: %v", err)
	}
	var audits int
	if err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM audit_events WHERE actor_kind='admin'`).Scan(&audits); err != nil || audits != len(actions) {
		t.Fatalf("failed action audited as success: %d %v", audits, err)
	}
	if _, err := s.Pool.Exec(ctx, `UPDATE audit_events SET result='rejected' WHERE id=$1`, o.Audit[0].ID); err == nil {
		t.Fatal("AuditEvent history was mutable")
	}
}

func TestP4DAdminSafeCancelStopAndQuarantine(t *testing.T) {
	s, nodeID, ownerID, requestID, allocationID := p4cRunning(t)
	ctx := context.Background()
	adminID, err := s.CreateAdmin(ctx, "p4d-operator", "a long test password")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.ApplyAdminAction(ctx, adminID, AdminAction{Action: "request.cancel", TargetID: requestID}); !errors.Is(err, ErrJobConflict) {
		t.Fatalf("unsafe cancel: %v", err)
	}
	if err := s.ApplyAdminAction(ctx, adminID, AdminAction{Action: "request.stop", TargetID: requestID}); err != nil {
		t.Fatal(err)
	}
	p4aStates(t, s, requestID, allocationID, "stopping", "stopping", 1)
	if err := s.ApplyAdminAction(ctx, adminID, AdminAction{Action: "request.stop", TargetID: requestID}); !errors.Is(err, ErrJobConflict) {
		t.Fatalf("duplicate stop: %v", err)
	}
	if err := s.ApplyAdminAction(ctx, adminID, AdminAction{Action: "allocation.quarantine", TargetID: allocationID}); err != nil {
		t.Fatal(err)
	}
	p4aStates(t, s, requestID, allocationID, "quarantined", "quarantined", 1)
	if _, err := s.AbandonQuarantinedUserRequest(ctx, ownerID, requestID); err != nil {
		t.Fatal(err)
	}
	if changed, err := s.TryAllocateOne(ctx); err != nil || changed {
		t.Fatalf("quarantined capacity reused: %t %v", changed, err)
	}
	var audits int
	if err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM audit_events WHERE actor_admin_user_id=$1`, adminID).Scan(&audits); err != nil || audits != 2 {
		t.Fatalf("audit=%d: %v", audits, err)
	}
	// A Controller's later accepted stop never makes an admin quarantine release capacity.
	stopJob, err := s.ClaimNextJob(ctx, nodeID)
	if err != nil || stopJob == nil {
		t.Fatalf("stop claim: %+v %v", stopJob, err)
	}
	if _, err := s.ReportJob(ctx, nodeID, stopJob.ID, nodev1.ReportRequest{State: "accepted", InstanceID: "i_p4c", OperationID: "o_stop"}); err != nil {
		t.Fatal(err)
	}
	p4aStates(t, s, requestID, allocationID, "abandoned", "quarantined", 1)
}

func TestP4DAdminCanReclaimAbandonedQuarantine(t *testing.T) {
	s, nodeID, ownerID, requestID, allocationID := p4cRunning(t)
	ctx := context.Background()
	adminID, err := s.CreateAdmin(ctx, "p4d-late-reclaim", "a long test password")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.ApplyAdminAction(ctx, adminID, AdminAction{Action: "allocation.quarantine", TargetID: allocationID}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AbandonQuarantinedUserRequest(ctx, ownerID, requestID); err != nil {
		t.Fatal(err)
	}
	if err := s.ApplyAdminAction(ctx, adminID, AdminAction{Action: "request.stop", TargetID: requestID}); err != nil {
		t.Fatalf("stop abandoned quarantined resource: %v", err)
	}
	p4aStates(t, s, requestID, allocationID, "abandoned", "quarantined", 1)
	if err := s.ApplyAdminAction(ctx, adminID, AdminAction{Action: "request.stop", TargetID: requestID}); !errors.Is(err, ErrJobConflict) {
		t.Fatalf("duplicate stop: %v", err)
	}
	job, err := s.ClaimNextJob(ctx, nodeID)
	if err != nil || job == nil || job.Kind != "stop" {
		t.Fatalf("stop job: %+v %v", job, err)
	}
	if _, err := s.ReportJob(ctx, nodeID, job.ID, nodev1.ReportRequest{State: "accepted", InstanceID: "i_p4c", OperationID: "o_late_stop"}); err != nil {
		t.Fatal(err)
	}
	p4aStates(t, s, requestID, allocationID, "abandoned", "quarantined", 1)
	if err := s.ReportInstanceFact(ctx, nodeID, allocationID, nodev1.InstanceFact{
		InstanceID: "i_p4c", Outcome: "reclaimed", Lifecycle: "reclaimed", Process: "stopped", Cleanup: "complete", Port: 28000,
	}); err != nil {
		t.Fatal(err)
	}
	p4aStates(t, s, requestID, allocationID, "abandoned", "reclaimed", 0)
	if err := s.ApplyAdminAction(ctx, adminID, AdminAction{Action: "request.stop", TargetID: requestID}); !errors.Is(err, ErrJobConflict) {
		t.Fatalf("reclaimed resource stop: %v", err)
	}
}

func TestP4DEntryVerificationRequiresCurrentControllerFacts(t *testing.T) {
	s := playerTestStore(t)
	ctx := context.Background()
	_, _ = seedPlayerCatalog(t, s)
	nodeID, _, err := s.RegisterNode(ctx, "entry fixture", "linux")
	if err != nil {
		t.Fatal(err)
	}
	h := p1TestHeartbeat("test-v1")
	h.Network.A2SEnabled = true
	h.A2SQueryOK = true
	if _, err := s.RecordHeartbeat(ctx, nodeID, h); err != nil {
		t.Fatal(err)
	}
	adminID, err := s.CreateAdmin(ctx, "entry-admin", "a long test password")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.ApplyAdminAction(ctx, adminID, AdminAction{Action: "entry.update", TargetID: nodeID, Entry: "steam", Verified: boolPtr(true)}); !errors.Is(err, ErrInvalidAdminAction) {
		t.Fatalf("unconfirmed human verification: %v", err)
	}
	if err := s.ApplyAdminAction(ctx, adminID, AdminAction{Action: "entry.update", TargetID: nodeID, Entry: "steam", Verified: boolPtr(true), Confirmed: true}); err != nil {
		t.Fatal(err)
	}
	if err := s.ApplyAdminAction(ctx, adminID, AdminAction{Action: "entry.update", TargetID: nodeID, Entry: "steam", Enabled: boolPtr(true)}); err != nil {
		t.Fatal(err)
	}
	var verified, enabled bool
	if err := s.Pool.QueryRow(ctx, `SELECT steam_entry_verified,steam_entry_enabled FROM node_entry_capabilities WHERE node_id=$1`, nodeID).Scan(&verified, &enabled); err != nil || !verified || !enabled {
		t.Fatalf("fixture entry %t/%t: %v", verified, enabled, err)
	}
	h.Network.ConnectHost = "127.0.0.2"
	if _, err := s.RecordHeartbeat(ctx, nodeID, h); err != nil {
		t.Fatal(err)
	}
	if err := s.Pool.QueryRow(ctx, `SELECT steam_entry_verified,steam_entry_enabled FROM node_entry_capabilities WHERE node_id=$1`, nodeID).Scan(&verified, &enabled); err != nil || verified || enabled {
		t.Fatalf("stale revision entry %t/%t: %v", verified, enabled, err)
	}
}
