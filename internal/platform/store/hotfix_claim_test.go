package store

import (
	"context"
	"sync"
	"testing"

	"github.com/L4C99/dota2-arcade-platform/internal/contracts/nodev1"
)

func TestHotfixIndependentClaimPreservesCapacityAndIdentity(t *testing.T) {
	s := playerTestStore(t)
	ctx := context.Background()
	game, preset := seedPlayerCatalog(t, s)
	node := p3CapacityNode(t, s, "hotfix-two-slots", 2)
	p3CapacityRequest(t, s, game, preset)
	p3CapacityRequest(t, s, game, preset)
	for i := 0; i < 2; i++ {
		if changed, err := s.TryAllocateOne(ctx); err != nil || !changed {
			t.Fatalf("reservation %d: changed=%v err=%v", i, changed, err)
		}
	}
	a, err := s.ClaimNextJob(ctx, node)
	if err != nil || a == nil {
		t.Fatalf("claim A: %+v %v", a, err)
	}
	if _, err := s.PrepareCreate(ctx, node, a.ID, "/tmp/test-template.json", 0); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReportJob(ctx, node, a.ID, nodev1.ReportRequest{State: "accepted", InstanceID: "i_a", OperationID: "o_a"}); err != nil {
		t.Fatal(err)
	}
	b, err := s.ClaimNextJob(ctx, node)
	if err != nil || b == nil || b.ID == a.ID || b.Kind != "create" {
		t.Fatalf("independent B claim while A accepted: %+v %v", b, err)
	}
	if _, err := s.PrepareCreate(ctx, node, b.ID, "/tmp/test-template.json", 0); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReportJob(ctx, node, b.ID, nodev1.ReportRequest{State: "accepted", InstanceID: "i_b", OperationID: "o_b"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReportJob(ctx, node, a.ID, nodev1.ReportRequest{State: "unknown", InstanceID: "i_a", OperationID: "o_a"}); err != nil {
		t.Fatal(err)
	}
	if got := sMustJob(t, s, node, a.ID); got.State != "unknown" || got.InstanceID != "i_a" || got.OperationID != "o_a" {
		t.Fatalf("A identity changed: %+v", got)
	}
	if got := sMustJob(t, s, node, b.ID); got.State != "accepted" || got.InstanceID != "i_b" || got.OperationID != "o_b" {
		t.Fatalf("B identity changed: %+v", got)
	}
	p3CapacityRequest(t, s, game, preset)
	if changed, err := s.TryAllocateOne(ctx); err != nil || changed {
		t.Fatalf("capacity oversold: changed=%v err=%v", changed, err)
	}
	if cap, err := s.Capacity(ctx, node); err != nil || cap.Occupied != 2 || cap.Hard != 2 || cap.Desired != 2 {
		t.Fatalf("unknown capacity released: %+v %v", cap, err)
	}
}

func TestHotfixConcurrentClaimsRespectSameInstance(t *testing.T) {
	s := playerTestStore(t)
	ctx := context.Background()
	node, _, err := s.RegisterNode(ctx, "hotfix-racing-claims", "linux")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.RecordHeartbeat(ctx, node, p1TestHeartbeat("test-v1")); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		id, err := NewID()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.Pool.Exec(ctx, `INSERT INTO node_jobs(id,node_id,kind,integration_only,state,instance_id)
			VALUES($1,$2,'stop',true,'pending','i_race')`, id, node); err != nil {
			t.Fatal(err)
		}
	}
	var wg sync.WaitGroup
	results := make([]*nodev1.Job, 2)
	errs := make([]error, 2)
	for i := range results {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i], errs[i] = s.ClaimNextJob(ctx, node)
		}(i)
	}
	wg.Wait()
	claimed := 0
	for i := range results {
		if errs[i] != nil {
			t.Fatal(errs[i])
		}
		if results[i] != nil {
			claimed++
		}
	}
	if claimed != 1 {
		t.Fatalf("same instance had %d concurrent winners", claimed)
	}
}

func sMustJob(t *testing.T, s *Store, node, id string) nodev1.Job {
	t.Helper()
	job, err := s.JobForNode(context.Background(), node, id)
	if err != nil {
		t.Fatal(err)
	}
	return job
}

func TestHotfixOrdinaryClaimSkipsSameInstanceConflict(t *testing.T) {
	s := playerTestStore(t)
	ctx := context.Background()
	node, _, err := s.RegisterNode(ctx, "hotfix-conflict", "linux")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.RecordHeartbeat(ctx, node, p1TestHeartbeat("test-v1")); err != nil {
		t.Fatal(err)
	}
	old, err := NewID()
	if err != nil {
		t.Fatal(err)
	}
	stop, err := NewID()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Pool.Exec(ctx, `INSERT INTO node_jobs(id,node_id,kind,integration_only,state,instance_id)
		VALUES($1,$3,'stop',true,'accepted','i_same'),($2,$3,'stop',true,'pending','i_same')`, old, stop, node); err != nil {
		t.Fatal(err)
	}
	if job, err := s.ClaimNextJob(ctx, node); err != nil || job != nil {
		t.Fatalf("same-instance stop conflict claimed: %+v %v", job, err)
	}
	if got := sMustJob(t, s, node, stop); got.State != "pending" {
		t.Fatalf("conflicting job moved: %+v", got)
	}
}
