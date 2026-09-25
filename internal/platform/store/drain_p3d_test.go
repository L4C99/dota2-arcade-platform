package store

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
)

func TestP3DDrainResumeKeepsExistingAndManualWaiting(t *testing.T) {
	s := playerTestStore(t)
	ctx := context.Background()
	gameID, presetID := seedPlayerCatalog(t, s)
	a := p3CapacityNode(t, s, "A", 2)
	b := p3CapacityNode(t, s, "B", 1)
	if err := s.SetNodePriority(ctx, a, 10); err != nil {
		t.Fatal(err)
	}
	first := p3Request(t, s, gameID, presetID, "auto", "")
	if changed, err := s.TryAllocateOne(ctx); err != nil || !changed {
		t.Fatalf("first reserve: %t %v", changed, err)
	}
	if got := allocatedNode(t, s, first.ID); got != a {
		t.Fatalf("first node = %s", got)
	}
	if err := s.SetNodeDrain(ctx, a, true); err != nil {
		t.Fatal(err)
	}
	if draining, err := s.NodeDraining(ctx, a); err != nil || !draining {
		t.Fatalf("drain flag = %t: %v", draining, err)
	}
	manual := p3Request(t, s, gameID, presetID, "manual", a)
	lateAuto := p3Request(t, s, gameID, presetID, "auto", "")
	if changed, err := s.TryAllocateOne(ctx); err != nil || !changed {
		t.Fatalf("drain blocked unrelated auto: %t %v", changed, err)
	}
	if got := allocatedNode(t, s, lateAuto.ID); got != b {
		t.Fatalf("drained A received new auto: %s", got)
	}
	var state string
	if err := s.Pool.QueryRow(ctx, `SELECT state FROM server_requests WHERE id=$1`, manual.ID).Scan(&state); err != nil || state != "waiting" {
		t.Fatalf("manual on drained A = %s: %v", state, err)
	}
	firstCapacity, err := s.Capacity(ctx, a)
	if err != nil || firstCapacity.Occupied != 1 {
		t.Fatalf("drain changed existing occupancy: %+v %v", firstCapacity, err)
	}
	// Drain never stops the already reserved create job. The Controller can
	// continue this Allocation while admission remains closed.
	job, err := s.ClaimNextJob(ctx, a)
	if err != nil || job == nil || job.Kind != "create" {
		t.Fatalf("drain stopped existing job: %+v %v", job, err)
	}
	if err := s.SetNodeDrain(ctx, a, false); err != nil {
		t.Fatal(err)
	}
	if changed, err := s.TryAllocateOne(ctx); err != nil || !changed {
		t.Fatalf("resume did not unblock manual: %t %v", changed, err)
	}
	if got := allocatedNode(t, s, manual.ID); got != a {
		t.Fatalf("manual moved after resume: %s", got)
	}
	firstCapacity, err = s.Capacity(ctx, a)
	if err != nil || firstCapacity.Occupied != 2 {
		t.Fatalf("resumed A capacity %+v: %v", firstCapacity, err)
	}
	if err := s.SetNodeDrain(ctx, a, false); err != nil {
		t.Fatalf("idempotent resume: %v", err)
	}
	if err := s.SetNodeDrain(ctx, "00000000-0000-4000-8000-000000000000", true); err != pgx.ErrNoRows {
		t.Fatalf("unknown node drain: %v", err)
	}
}
