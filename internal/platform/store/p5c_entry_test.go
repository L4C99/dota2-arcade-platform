package store

import (
	"context"
	"errors"
	"testing"
)

func TestP5CEntryVerificationRequiresCompletePortCoverage(t *testing.T) {
	s := playerTestStore(t)
	ctx := context.Background()
	_, _ = seedPlayerCatalog(t, s)
	nodeID, _, err := s.RegisterNode(ctx, "entry ports", "linux")
	if err != nil {
		t.Fatal(err)
	}
	h := p1TestHeartbeat("test-v1")
	h.HardMaxInstances = 2
	h.Network.LocalPortMax = 28001
	h.Network.ProtocolIP = "203.0.113.10"
	if _, err := s.RecordHeartbeat(ctx, nodeID, h); err != nil {
		t.Fatal(err)
	}
	adminID, err := s.CreateAdmin(ctx, "p5c-entry-admin", "a long test password")
	if err != nil {
		t.Fatal(err)
	}
	base := AdminAction{Action: "entry.update", TargetID: nodeID, Entry: "steam", Verified: boolPtr(true), Confirmed: true, VerificationNote: "real client entered every listed public port"}
	partial := base
	partial.VerifiedPorts = []int{28000}
	if err := s.ApplyAdminAction(ctx, adminID, partial); !errors.Is(err, ErrJobConflict) {
		t.Fatalf("one-port verification accepted: %v", err)
	}
	complete := base
	complete.VerifiedPorts = []int{28001, 28000}
	if err := s.ApplyAdminAction(ctx, adminID, complete); err != nil {
		t.Fatal(err)
	}
	if err := s.ApplyAdminAction(ctx, adminID, AdminAction{Action: "entry.update", TargetID: nodeID, Entry: "steam", Enabled: boolPtr(true)}); err != nil {
		t.Fatal(err)
	}
	o, err := s.AdminOverview(ctx)
	if err != nil || len(o.Entries) != 1 || len(o.Entries[0].PublicPorts) != 2 || len(o.Entries[0].SteamVerifiedPorts) != 2 || !o.Entries[0].SteamEnabled || o.Entries[0].A2SEnabled || o.Entries[0].A2SQueryOK {
		t.Fatalf("entry overview: %+v %v", o.Entries, err)
	}
	h.Network.ProtocolIP = "203.0.113.11"
	if _, err := s.RecordHeartbeat(ctx, nodeID, h); err != nil {
		t.Fatal(err)
	}
	o, err = s.AdminOverview(ctx)
	if err != nil || o.Entries[0].SteamVerified || o.Entries[0].SteamEnabled || len(o.Entries[0].SteamVerifiedPorts) != 0 {
		t.Fatalf("revision did not clear coverage: %+v %v", o.Entries, err)
	}
}
