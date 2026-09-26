package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/L4C99/dota2-arcade-platform/internal/contracts/nodev1"
)

func TestP5CEntryVerificationAmendment001(t *testing.T) {
	s := playerTestStore(t)
	ctx := context.Background()
	_, _ = seedPlayerCatalog(t, s)
	nodeID, _, err := s.RegisterNode(ctx, "entry node", "linux")
	if err != nil {
		t.Fatal(err)
	}
	h := p1TestHeartbeat("test-v1")
	h.HardMaxInstances = 2
	h.Network.LocalPortMax = 28001
	h.Network.ProtocolIP = "203.0.113.10"
	h.Network.A2SEnabled = true
	h.A2SQueryOK = false // Idle: no successful live query fact.
	if _, err := s.RecordHeartbeat(ctx, nodeID, h); err != nil {
		t.Fatal(err)
	}
	h.A2SQueryOK = true
	if _, err := s.RecordHeartbeat(ctx, nodeID, h); err != nil {
		t.Fatal(err)
	}
	var automaticallyVerified bool
	if err := s.Pool.QueryRow(ctx, `SELECT steam_entry_verified FROM node_entry_capabilities WHERE node_id=$1`, nodeID).Scan(&automaticallyVerified); err != nil || automaticallyVerified {
		t.Fatalf("A2S success automatically verified Steam: %t: %v", automaticallyVerified, err)
	}
	h.A2SQueryOK = false
	if _, err := s.RecordHeartbeat(ctx, nodeID, h); err != nil {
		t.Fatal(err)
	}
	adminID, err := s.CreateAdmin(ctx, "p5c-amendment-admin", "a long test password")
	if err != nil {
		t.Fatal(err)
	}
	apply := func(a AdminAction) error {
		a.Action, a.TargetID = "entry.update", nodeID
		return s.ApplyAdminAction(ctx, adminID, a)
	}
	if err := apply(AdminAction{Entry: "steam", Enabled: boolPtr(true)}); !errors.Is(err, ErrInvalidAdminAction) {
		t.Fatalf("unverified entry enabled: %v", err)
	}
	if err := apply(AdminAction{Entry: "steam", Verified: boolPtr(true)}); !errors.Is(err, ErrInvalidAdminAction) {
		t.Fatalf("unconfirmed verification: %v", err)
	}
	if err := apply(AdminAction{Entry: "steam", Verified: boolPtr(true), Enabled: boolPtr(true), Confirmed: true}); !errors.Is(err, ErrInvalidAdminAction) {
		t.Fatalf("combined verification/opening: %v", err)
	}
	if err := apply(AdminAction{Entry: "steam", Verified: boolPtr(true), Confirmed: true}); err != nil {
		t.Fatalf("confirmed verification without ports or note: %v", err)
	}
	var verified, enabled, chinaVerified, chinaEnabled bool
	var atIsSet, byIsSet bool
	var revision string
	read := func() {
		t.Helper()
		if err := s.Pool.QueryRow(ctx, `SELECT steam_entry_verified,steam_entry_enabled,
			steam_verified_at IS NOT NULL,steam_verified_by IS NOT NULL,
			steamchina_entry_verified,steamchina_entry_enabled,entry_config_revision
			FROM node_entry_capabilities WHERE node_id=$1`, nodeID).
			Scan(&verified, &enabled, &atIsSet, &byIsSet, &chinaVerified, &chinaEnabled, &revision); err != nil {
			t.Fatal(err)
		}
	}
	read()
	if !verified || enabled || !atIsSet || !byIsSet || chinaVerified || chinaEnabled || revision == "" {
		t.Fatal("initial verification metadata or scheme separation incorrect")
	}
	if err := apply(AdminAction{Entry: "steam", Enabled: boolPtr(true)}); err != nil {
		t.Fatal(err)
	}
	h.A2SQueryOK = true
	if _, err := s.RecordHeartbeat(ctx, nodeID, h); err != nil {
		t.Fatal(err)
	}
	h.A2SQueryOK = false
	if _, err := s.RecordHeartbeat(ctx, nodeID, h); err != nil {
		t.Fatal(err)
	}
	read()
	if !verified || !enabled {
		t.Fatal("A2S query result revoked verification or opening")
	}
	if err := apply(AdminAction{Entry: "steamchina", Verified: boolPtr(true), Confirmed: true}); err != nil {
		t.Fatal(err)
	}
	if err := apply(AdminAction{Entry: "steamchina", Enabled: boolPtr(true)}); err != nil {
		t.Fatal(err)
	}
	if err := apply(AdminAction{Entry: "steam", Verified: boolPtr(false)}); !errors.Is(err, ErrInvalidAdminAction) {
		t.Fatalf("same-revision verification was revoked: %v", err)
	}
	read()
	if !verified || !enabled || !atIsSet || !byIsSet || !chinaVerified || !chinaEnabled {
		t.Fatal("rejected revoke changed verification metadata or another scheme")
	}
	if err := apply(AdminAction{Entry: "steam", Enabled: boolPtr(false)}); err != nil {
		t.Fatal(err)
	}
	read()
	if !verified || enabled || !atIsSet || !byIsSet {
		t.Fatal("closing entry changed deployment attestation")
	}
	if err := apply(AdminAction{Entry: "steam", Enabled: boolPtr(true)}); err != nil {
		t.Fatal(err)
	}
	var audited int
	if err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM audit_events WHERE action='entry.update' AND target_type='node_entry' AND actor_admin_user_id=$1`, adminID).Scan(&audited); err != nil || audited != 6 {
		t.Fatalf("entry action audit count=%d: %v", audited, err)
	}
	h.Network.ProtocolIP = "203.0.113.11"
	if _, err := s.RecordHeartbeat(ctx, nodeID, h); err != nil {
		t.Fatal(err)
	}
	read()
	if verified || enabled || chinaVerified || chinaEnabled || atIsSet || byIsSet {
		t.Fatal("revision change did not invalidate both schemes")
	}
}

func TestP5CEntryVerificationRequiresProtocolConfiguration(t *testing.T) {
	s := playerTestStore(t)
	ctx := context.Background()
	nodeID, _, err := s.RegisterNode(ctx, "no protocol IP", "linux")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.RecordHeartbeat(ctx, nodeID, p1TestHeartbeat("test-v1")); err != nil {
		t.Fatal(err)
	}
	adminID, err := s.CreateAdmin(ctx, "p5c-no-protocol", "a long test password")
	if err != nil {
		t.Fatal(err)
	}
	err = s.ApplyAdminAction(ctx, adminID, AdminAction{Action: "entry.update", TargetID: nodeID, Entry: "steam", Verified: boolPtr(true), Confirmed: true})
	if !errors.Is(err, ErrJobConflict) {
		t.Fatalf("verification accepted without protocol IP: %v", err)
	}
}

func TestP5CInstanceA2SDiagnosticsAreIndependentAndIdleClears(t *testing.T) {
	s := playerTestStore(t)
	ctx := context.Background()
	nodeID, _, err := s.RegisterNode(ctx, "diagnostic node", "linux")
	if err != nil {
		t.Fatal(err)
	}
	h := p1TestHeartbeat("test-v1")
	h.HardMaxInstances = 2
	h.Network.LocalPortMax = 28001
	h.Network.A2SEnabled = true
	checkedAt := time.Now().UTC().Format(time.RFC3339Nano)
	h.A2SDiagnostics = []nodev1.A2SDiagnostic{
		{InstanceID: "ready-a", LocalPort: 28000, Status: "ok", CheckedAt: checkedAt},
		{InstanceID: "ready-b", LocalPort: 28001, Status: "failed", CheckedAt: checkedAt},
	}
	if _, err := s.RecordHeartbeat(ctx, nodeID, h); err != nil {
		t.Fatal(err)
	}
	rows, err := s.Pool.Query(ctx, `SELECT instance_id,local_port,status FROM node_instance_a2s_diagnostics WHERE node_id=$1 ORDER BY instance_id`, nodeID)
	if err != nil {
		t.Fatal(err)
	}
	var got []nodev1.A2SDiagnostic
	for rows.Next() {
		var item nodev1.A2SDiagnostic
		if err := rows.Scan(&item.InstanceID, &item.LocalPort, &item.Status); err != nil {
			rows.Close()
			t.Fatal(err)
		}
		got = append(got, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		t.Fatal(err)
	}
	rows.Close()
	if len(got) != 2 || got[0].Status != "ok" || got[1].Status != "failed" {
		t.Fatalf("lost independent facts: %+v", got)
	}
	h.A2SDiagnostics = nil
	if _, err := s.RecordHeartbeat(ctx, nodeID, h); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM node_instance_a2s_diagnostics WHERE node_id=$1`, nodeID).Scan(&count); err != nil || count != 0 {
		t.Fatalf("idle node retained live query facts: %d: %v", count, err)
	}
}
