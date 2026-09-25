package store

import (
	"context"
	"errors"
	"testing"

	"github.com/L4C99/dota2-arcade-platform/internal/contracts/nodev1"
	"github.com/jackc/pgx/v5"
)

func TestP2CActiveJoinLeavePreservesInstanceAndParty(t *testing.T) {
	s := playerTestStore(t)
	ctx := context.Background()
	if err := s.ConfigurePartySize(ctx, 3); err != nil {
		t.Fatal(err)
	}
	gameID, presetID := seedPlayerCatalog(t, s)
	if _, err := s.Pool.Exec(ctx, `UPDATE game_presets SET max_players=1 WHERE id=$1`, presetID); err != nil {
		t.Fatal(err)
	}
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
	leader, member, newMember := partyUser(t, s), partyUser(t, s), partyUser(t, s)
	partyID, err := s.CreateParty(ctx, leader)
	if err != nil {
		t.Fatal(err)
	}
	invite, err := s.CurrentInvite(ctx, leader)
	if err != nil {
		t.Fatal(err)
	}
	request, created, err := s.CreateUserRequest(ctx, leader, gameID, presetID)
	if err != nil || !created {
		t.Fatalf("create: %t %v", created, err)
	}
	if changed, err := s.TryAllocateOne(ctx); err != nil || !changed {
		t.Fatalf("allocate: %t %v", changed, err)
	}
	job, err := s.ClaimNextJob(ctx, nodeID)
	if err != nil || job == nil {
		t.Fatalf("create job: %+v %v", job, err)
	}
	if _, err := s.PrepareCreate(ctx, nodeID, job.ID, "/tmp/trusted-template.json", 0); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReportJob(ctx, nodeID, job.ID, nodev1.ReportRequest{State: "accepted", InstanceID: "i_party", OperationID: "o_create"}); err != nil {
		t.Fatal(err)
	}
	join := &nodev1.JoinInfo{LocalPort: 28000, PublicPort: 28000, ConnectHost: "127.0.0.1",
		EntryConfigRevision: nodev1.EntryConfigRevision(p1TestHeartbeat("test-v1").Network)}
	if _, err := s.ReportJob(ctx, nodeID, job.ID, nodev1.ReportRequest{State: "succeeded", InstanceID: "i_party", OperationID: "o_create", JoinInfo: join}); err != nil {
		t.Fatal(err)
	}
	allocation, err := s.UserRequestAllocation(ctx, leader, request.ID)
	if err != nil || allocation == nil || allocation.JoinInfo == nil {
		t.Fatalf("leader join info: %+v %v", allocation, err)
	}
	allocationID := allocation.ID
	for _, user := range []string{member, newMember} {
		if _, err := s.JoinParty(ctx, user, invite.Token); err != nil {
			t.Fatalf("active join: %v", err)
		}
		current, err := s.CurrentUserRequest(ctx, user)
		if err != nil || current == nil || current.ID != request.ID {
			t.Fatalf("shared current: %+v %v", current, err)
		}
		shared, err := s.UserRequestAllocation(ctx, user, request.ID)
		if err != nil || shared == nil || shared.ID != allocationID || shared.JoinInfo == nil ||
			shared.JoinInfo.ConnectCommand != allocation.JoinInfo.ConnectCommand {
			t.Fatalf("shared join info: %+v %v", shared, err)
		}
	}
	if _, _, err := s.CreateUserRequest(ctx, leader, gameID, presetID); err != nil {
		t.Fatalf("duplicate running request: %v", err)
	}
	if err := s.LeaveParty(ctx, member); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UserRequest(ctx, member, request.ID); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("former member read: %v", err)
	}
	if err := s.RemovePartyMember(ctx, leader, newMember); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UserRequest(ctx, newMember, request.ID); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("removed member read: %v", err)
	}
	var requestOwner string
	var allocationCount, jobCount int
	if err := s.Pool.QueryRow(ctx, `SELECT owner_party_id FROM server_requests WHERE id=$1`, request.ID).Scan(&requestOwner); err != nil || requestOwner != partyID {
		t.Fatalf("owner rewritten: %s %v", requestOwner, err)
	}
	if err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM allocations WHERE server_request_id=$1 AND id=$2 AND state='running'`, request.ID, allocationID).Scan(&allocationCount); err != nil || allocationCount != 1 {
		t.Fatalf("allocation changed: %d %v", allocationCount, err)
	}
	if err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM node_jobs WHERE allocation_id=$1`, allocationID).Scan(&jobCount); err != nil || jobCount != 1 {
		t.Fatalf("job changed: %d %v", jobCount, err)
	}
	if err := s.DisbandParty(ctx, leader); !errors.Is(err, ErrPartyBusy) {
		t.Fatalf("active disband: %v", err)
	}
	if _, err := s.StopUserRequest(ctx, leader, request.ID); err != nil {
		t.Fatal(err)
	}
	stopJob, err := s.ClaimNextJob(ctx, nodeID)
	if err != nil || stopJob == nil || stopJob.Kind != "stop" {
		t.Fatalf("stop job: %+v %v", stopJob, err)
	}
	if _, err := s.ReportJob(ctx, nodeID, stopJob.ID, nodev1.ReportRequest{State: "accepted", InstanceID: "i_party", OperationID: "o_stop"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReportJob(ctx, nodeID, stopJob.ID, nodev1.ReportRequest{State: "succeeded", InstanceID: "i_party", OperationID: "o_stop"}); err != nil {
		t.Fatal(err)
	}
	party, err := s.CurrentParty(ctx, leader)
	if err != nil || party == nil || party.ID != partyID || party.CurrentRole != "leader" || len(party.Members) != 1 {
		t.Fatalf("party after reclaim: %+v %v", party, err)
	}
	retained, err := s.CurrentInvite(ctx, leader)
	if err != nil || retained.Token != invite.Token {
		t.Fatalf("invite after reclaim: %v", err)
	}
	if _, err := s.JoinParty(ctx, member, invite.Token); err != nil {
		t.Fatalf("join after reclaim: %v", err)
	}
}
