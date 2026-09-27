package store

import (
	"context"
	"errors"
	"testing"
)

func currentEntryRevision(t *testing.T, s *Store, nodeID string) string {
	t.Helper()
	var revision string
	if err := s.Pool.QueryRow(context.Background(), `SELECT entry_config_revision FROM node_entry_capabilities WHERE node_id=$1`, nodeID).Scan(&revision); err != nil {
		t.Fatal(err)
	}
	return revision
}

func TestA4EntryRejectsStaleAttestationAndEnable(t *testing.T) {
	s := playerTestStore(t)
	ctx := context.Background()
	nodeID, _, err := s.RegisterNode(ctx, "revision race", "linux")
	if err != nil {
		t.Fatal(err)
	}
	h := p1TestHeartbeat("test-v1")
	h.Network.ProtocolIP = "203.0.113.10"
	if _, err = s.RecordHeartbeat(ctx, nodeID, h); err != nil {
		t.Fatal(err)
	}
	old := currentEntryRevision(t, s, nodeID)
	adminID, err := s.CreateAdmin(ctx, "revision-admin", "a long test password")
	if err != nil {
		t.Fatal(err)
	}
	h.Network.ProtocolIP = "203.0.113.11"
	if _, err = s.RecordHeartbeat(ctx, nodeID, h); err != nil {
		t.Fatal(err)
	}
	for _, revision := range []string{"", old} {
		for _, action := range []AdminAction{
			{Verified: boolPtr(true), Confirmed: true}, {Enabled: boolPtr(true)},
		} {
			action.Action, action.TargetID, action.Entry = "entry.update", nodeID, "steam"
			action.ExpectedEntryConfigRevision = revision
			if err := s.ApplyAdminAction(ctx, adminID, action); !errors.Is(err, ErrJobConflict) {
				t.Fatalf("stale action: %v", err)
			}
		}
	}
	var count int
	if err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM audit_events WHERE action='entry.update'`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("stale action audited: %d %v", count, err)
	}
	for _, action := range []AdminAction{{Verified: boolPtr(true), Confirmed: true}, {Enabled: boolPtr(true)}} {
		action.Action, action.TargetID, action.Entry = "entry.update", nodeID, "steam"
		action.ExpectedEntryConfigRevision = currentEntryRevision(t, s, nodeID)
		if err := s.ApplyAdminAction(ctx, adminID, action); err != nil {
			t.Fatal(err)
		}
	}
}
