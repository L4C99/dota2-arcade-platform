package store

import (
	"context"
	"errors"
	"sync"
	"testing"
)

func p3CapacityNode(t *testing.T, s *Store, name string, hard int) string {
	t.Helper()
	ctx := context.Background()
	id, _, err := s.RegisterNode(ctx, name, "linux")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Pool.Exec(ctx, `INSERT INTO node_template_bindings(node_id,template_revision_id,binding_key)
		VALUES($1,'test-template','test-binding')`, id); err != nil {
		t.Fatal(err)
	}
	heartbeat := p1TestHeartbeat("test-v1")
	heartbeat.HardMaxInstances = hard
	heartbeat.Network.LocalPortMax = heartbeat.Network.LocalPortMin + hard - 1
	if _, err := s.RecordHeartbeat(ctx, id, heartbeat); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Pool.Exec(ctx, `UPDATE node_content_bindings SET accepting_new_allocations=true WHERE node_id=$1`, id); err != nil {
		t.Fatal(err)
	}
	if err := s.SetDesiredCapacity(ctx, id, hard); err != nil {
		t.Fatal(err)
	}
	return id
}

func p3CapacityRequest(t *testing.T, s *Store, gameID, presetID string) {
	t.Helper()
	ctx := context.Background()
	userID, _, err := s.CreateUserSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.CreateUserRequest(ctx, userID, gameID, presetID); err != nil {
		t.Fatal(err)
	}
}

func TestP3APerNodeConcurrentReservation(t *testing.T) {
	s := playerTestStore(t)
	ctx := context.Background()
	gameID, presetID := seedPlayerCatalog(t, s)
	first := p3CapacityNode(t, s, "first", 1)
	second := p3CapacityNode(t, s, "second", 1)
	for i := 0; i < 3; i++ {
		p3CapacityRequest(t, s, gameID, presetID)
	}
	var wg sync.WaitGroup
	errs := make([]error, 12)
	for i := range errs {
		wg.Add(1)
		go func(i int) { defer wg.Done(); _, errs[i] = s.TryAllocateOne(ctx) }(i)
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, id := range []string{first, second} {
		capacity, err := s.Capacity(ctx, id)
		if err != nil || capacity.Occupied != 1 || capacity.Hard != 1 || capacity.Desired != 1 {
			t.Fatalf("node %s capacity %+v: %v", id, capacity, err)
		}
	}
	var waiting int
	if err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM server_requests WHERE state='waiting'`).Scan(&waiting); err != nil || waiting != 1 {
		t.Fatalf("waiting=%d: %v", waiting, err)
	}
	// Both nonterminal states keep their own node occupied.
	if _, err := s.Pool.Exec(ctx, `UPDATE allocations SET state='create_unknown' WHERE node_id=$1`, first); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Pool.Exec(ctx, `UPDATE allocations SET state='failed_unreclaimed' WHERE node_id=$1`, second); err != nil {
		t.Fatal(err)
	}
	if changed, err := s.TryAllocateOne(ctx); err != nil || changed {
		t.Fatalf("unsafe capacity release: changed=%t err=%v", changed, err)
	}
}

func TestP3ADesiredAndHardShrink(t *testing.T) {
	s := playerTestStore(t)
	ctx := context.Background()
	gameID, presetID := seedPlayerCatalog(t, s)
	id := p3CapacityNode(t, s, "shrinking", 2)
	if err := s.SetDesiredCapacity(ctx, id, 3); !errors.Is(err, ErrInvalidDesiredCapacity) {
		t.Fatalf("desired above hard accepted: %v", err)
	}
	p3CapacityRequest(t, s, gameID, presetID)
	p3CapacityRequest(t, s, gameID, presetID)
	if changed, err := s.TryAllocateOne(ctx); err != nil || !changed {
		t.Fatalf("first reservation: %t %v", changed, err)
	}
	if err := s.SetDesiredCapacity(ctx, id, 1); err != nil {
		t.Fatal(err)
	}
	if changed, err := s.TryAllocateOne(ctx); err != nil || changed {
		t.Fatalf("desired reduction oversold: %t %v", changed, err)
	}
	if err := s.SetDesiredCapacity(ctx, id, 2); err != nil {
		t.Fatal(err)
	}
	heartbeat := p1TestHeartbeat("test-v1")
	if _, err := s.RecordHeartbeat(ctx, id, heartbeat); err != nil {
		t.Fatal(err)
	}
	capacity, err := s.Capacity(ctx, id)
	if err != nil || capacity.Hard != 1 || capacity.Desired != 2 || capacity.Occupied != 1 {
		t.Fatalf("hard shrink capacity %+v: %v", capacity, err)
	}
	if changed, err := s.TryAllocateOne(ctx); err != nil || changed {
		t.Fatalf("hard shrink oversold: %t %v", changed, err)
	}
	if err := s.SetDesiredCapacity(ctx, id, 1); err != nil {
		t.Fatal(err)
	}
	if err := s.SetDesiredCapacity(ctx, id, -1); !errors.Is(err, ErrInvalidDesiredCapacity) {
		t.Fatalf("negative desired accepted: %v", err)
	}
}
