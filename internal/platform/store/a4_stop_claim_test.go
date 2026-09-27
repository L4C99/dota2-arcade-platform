package store

import (
	"context"
	"github.com/L4C99/dota2-arcade-platform/internal/contracts/nodev1"
	"testing"
)

func TestA4IndependentStopClaim(t *testing.T) {
	s, node, owner, request, allocation := p4cRunning(t)
	ctx := context.Background()
	// An unresolved integration create represents an independent old resource.
	old, err := NewID()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Pool.Exec(ctx, `INSERT INTO node_jobs(id,node_id,kind,integration_only,state,template_binding_key) VALUES($1,$2,'create',true,'unknown','test-binding')`, old, node); err != nil {
		t.Fatal(err)
	}
	if _, err := s.StopUserRequest(ctx, owner, request); err != nil {
		t.Fatal(err)
	}
	// A dependency on the same allocation must prevent the restricted claim.
	if _, err := s.Pool.Exec(ctx, `UPDATE node_jobs SET instance_id=$2 WHERE id=$1`, old, "i_p4c"); err != nil {
		t.Fatal(err)
	}
	if job, err := s.ClaimIndependentStop(ctx, node); err != nil || job != nil {
		t.Fatalf("dependent claim: %+v %v", job, err)
	}
	if _, err := s.Pool.Exec(ctx, `UPDATE node_jobs SET instance_id=NULL WHERE id=$1`, old); err != nil {
		t.Fatal(err)
	}
	job, err := s.ClaimIndependentStop(ctx, node)
	if err != nil || job == nil || job.Kind != "stop" {
		t.Fatalf("independent claim: %+v %v", job, err)
	}
	if _, err := s.ReportJob(ctx, node, job.ID, nodev1.ReportRequest{State: "accepted", InstanceID: "i_p4c", OperationID: "o_stop"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReportJob(ctx, node, job.ID, nodev1.ReportRequest{State: "succeeded", InstanceID: "i_p4c", OperationID: "o_stop"}); err != nil {
		t.Fatal(err)
	}
	p4aStates(t, s, request, allocation, "ended", "reclaimed", 0)
	if job, err := s.ClaimIndependentStop(ctx, node); err != nil || job != nil {
		t.Fatalf("claimed ordinary work: %+v %v", job, err)
	}
	var state string
	if err := s.Pool.QueryRow(ctx, `SELECT state FROM node_jobs WHERE id=$1`, old).Scan(&state); err != nil || state != "unknown" {
		t.Fatalf("old unknown: %s %v", state, err)
	}
}
