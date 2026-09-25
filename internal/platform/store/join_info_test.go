package store

import (
	"context"
	"testing"

	"github.com/L4C99/dota2-arcade-platform/internal/contracts/nodev1"
)

func TestP1CReadyWithoutMappingRetainsInstance(t *testing.T) {
	s := playerTestStore(t)
	ctx := context.Background()
	gameID, presetID := seedPlayerCatalog(t, s)
	nodeID, _, err := s.RegisterNode(ctx, "test node", "linux")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Pool.Exec(ctx, `INSERT INTO node_template_bindings(node_id,template_revision_id,binding_key)
		VALUES($1,'test-template','test-binding')`, nodeID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RecordHeartbeat(ctx, nodeID, p1TestHeartbeat("test-v1")); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Pool.Exec(ctx, `UPDATE nodes SET desired_max_instances=1 WHERE id=$1`, nodeID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Pool.Exec(ctx, `UPDATE node_content_bindings SET accepting_new_allocations=true WHERE node_id=$1`, nodeID); err != nil {
		t.Fatal(err)
	}
	userID, _, err := s.CreateUserSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	request, _, err := s.CreateUserRequest(ctx, userID, gameID, presetID)
	if err != nil {
		t.Fatal(err)
	}
	if changed, err := s.TryAllocateOne(ctx); err != nil || !changed {
		t.Fatalf("allocation %t: %v", changed, err)
	}
	job, err := s.ClaimNextJob(ctx, nodeID)
	if err != nil || job == nil {
		t.Fatalf("claim %+v: %v", job, err)
	}
	if _, err := s.PrepareCreate(ctx, nodeID, job.ID, "/tmp/trusted-template.json", 0); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReportJob(ctx, nodeID, job.ID, nodev1.ReportRequest{State: "accepted", InstanceID: "i_test", OperationID: "o_test"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReportJob(ctx, nodeID, job.ID, nodev1.ReportRequest{State: "succeeded", InstanceID: "i_test", OperationID: "o_test",
		JoinInfoErrorCode: "PORT_MAPPING_UNAVAILABLE"}); err != nil {
		t.Fatal(err)
	}
	allocation, err := s.UserRequestAllocation(ctx, userID, request.ID)
	if err != nil {
		t.Fatal(err)
	}
	if allocation.State != "running" || allocation.ReadyAt == nil || allocation.JoinInfoAvailableAt != nil ||
		allocation.JoinInfo != nil || allocation.JoinInfoErrorCode != "PORT_MAPPING_UNAVAILABLE" {
		t.Fatalf("Ready without mapping: %+v", allocation)
	}
	var occupied, stopJobs int
	if err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM allocations WHERE state NOT IN ('reclaimed','released_no_effect')`).Scan(&occupied); err != nil {
		t.Fatal(err)
	}
	if err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM node_jobs WHERE kind='stop'`).Scan(&stopJobs); err != nil {
		t.Fatal(err)
	}
	if occupied != 1 || stopJobs != 0 {
		t.Fatalf("Ready without mapping occupied=%d stopJobs=%d", occupied, stopJobs)
	}
}

func TestP1CEntryRevisionInvalidatesVerification(t *testing.T) {
	s := playerTestStore(t)
	ctx := context.Background()
	nodeID, _, err := s.RegisterNode(ctx, "test node", "linux")
	if err != nil {
		t.Fatal(err)
	}
	h := p1TestHeartbeat("test-v1")
	h.Content = []nodev1.ContentFact{}
	h.Network.ProtocolIP = "203.0.113.1"
	h.Network.A2SEnabled = true
	h.A2SQueryOK = true
	if _, err := s.RecordHeartbeat(ctx, nodeID, h); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Pool.Exec(ctx, `UPDATE node_entry_capabilities SET steam_entry_verified=true,
		steam_entry_enabled=true,steamchina_entry_verified=true,steamchina_entry_enabled=true WHERE node_id=$1`, nodeID); err != nil {
		t.Fatal(err)
	}
	h.Network.ProtocolIP = "203.0.113.2"
	if _, err := s.RecordHeartbeat(ctx, nodeID, h); err != nil {
		t.Fatal(err)
	}
	var steamVerified, steamEnabled, chinaVerified, chinaEnabled, a2sEnabled, a2sOK bool
	if err := s.Pool.QueryRow(ctx, `SELECT steam_entry_verified,steam_entry_enabled,
		steamchina_entry_verified,steamchina_entry_enabled,a2s_enabled,a2s_query_ok
		FROM node_entry_capabilities WHERE node_id=$1`, nodeID).
		Scan(&steamVerified, &steamEnabled, &chinaVerified, &chinaEnabled, &a2sEnabled, &a2sOK); err != nil {
		t.Fatal(err)
	}
	if steamVerified || steamEnabled || chinaVerified || chinaEnabled || !a2sEnabled || !a2sOK {
		t.Fatalf("entry revision did not invalidate verification")
	}
}
