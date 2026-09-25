package store

import (
	"context"
	"errors"
	"sync"
	"testing"
)

func TestP2DSameUserTwoPartyAndInviteResetRaces(t *testing.T) {
	s := playerTestStore(t)
	ctx := context.Background()
	if err := s.ConfigurePartySize(ctx, 3); err != nil {
		t.Fatal(err)
	}
	leaderA, leaderB, user := partyUser(t, s), partyUser(t, s), partyUser(t, s)
	if _, err := s.CreateParty(ctx, leaderA); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateParty(ctx, leaderB); err != nil {
		t.Fatal(err)
	}
	inviteA, _ := s.CurrentInvite(ctx, leaderA)
	inviteB, _ := s.CurrentInvite(ctx, leaderB)
	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i, token := range []string{inviteA.Token, inviteB.Token} {
		wg.Add(1)
		go func(i int, token string) { defer wg.Done(); _, errs[i] = s.JoinParty(ctx, user, token) }(i, token)
	}
	wg.Wait()
	success := 0
	for _, err := range errs {
		if err == nil {
			success++
		} else if !errors.Is(err, ErrAlreadyInParty) {
			t.Fatalf("same user: %v", err)
		}
	}
	if success != 1 {
		t.Fatalf("same user joined %d parties", success)
	}
	var count int
	if err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM party_members WHERE user_id=$1`, user).Scan(&count); err != nil || count != 1 {
		t.Fatalf("membership=%d %v", count, err)
	}
	for iteration := 0; iteration < 8; iteration++ {
		old, err := s.CurrentInvite(ctx, leaderA)
		if err != nil {
			t.Fatal(err)
		}
		candidate := partyUser(t, s)
		var joinErr, resetErr error
		wg.Add(2)
		go func() { defer wg.Done(); _, joinErr = s.JoinParty(ctx, candidate, old.Token) }()
		go func() { defer wg.Done(); _, resetErr = s.ResetInvite(ctx, leaderA) }()
		wg.Wait()
		if resetErr != nil {
			t.Fatal(resetErr)
		}
		if joinErr != nil && !errors.Is(joinErr, ErrInvalidInvite) && !errors.Is(joinErr, ErrPartyFull) {
			t.Fatalf("reset race join: %v", joinErr)
		}
		if _, err := s.JoinParty(ctx, partyUser(t, s), old.Token); !errors.Is(err, ErrInvalidInvite) {
			t.Fatalf("old invite valid after reset: %v", err)
		}
		if joinErr == nil {
			if err := s.LeaveParty(ctx, candidate); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func TestP2DPartyDuplicateAndSingleNodeLastSlot(t *testing.T) {
	s := playerTestStore(t)
	ctx := context.Background()
	if err := s.ConfigurePartySize(ctx, 3); err != nil {
		t.Fatal(err)
	}
	gameID, presetID := seedPlayerCatalog(t, s)
	nodeID, _, err := s.RegisterNode(ctx, "single node", "linux")
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
	leaders := []string{partyUser(t, s), partyUser(t, s)}
	for _, leader := range leaders {
		if _, err := s.CreateParty(ctx, leader); err != nil {
			t.Fatal(err)
		}
	}
	const posts = 8
	ids := make([][]string, 2)
	errLists := make([][]error, 2)
	var wg sync.WaitGroup
	for owner, leader := range leaders {
		ids[owner] = make([]string, posts)
		errLists[owner] = make([]error, posts)
		for i := 0; i < posts; i++ {
			wg.Add(1)
			go func(owner, i int, leader string) {
				defer wg.Done()
				r, _, err := s.CreateUserRequest(ctx, leader, gameID, presetID)
				ids[owner][i], errLists[owner][i] = r.ID, err
			}(owner, i, leader)
		}
	}
	wg.Wait()
	for owner := range ids {
		for i, id := range ids[owner] {
			if errLists[owner][i] != nil || id == "" || id != ids[owner][0] {
				t.Fatalf("owner %d post %d: %s %v", owner, i, id, errLists[owner][i])
			}
		}
	}
	if ids[0][0] == ids[1][0] {
		t.Fatal("distinct Party owners shared request")
	}
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := s.TryAllocateOne(ctx); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	var allocations, jobs int
	if err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM allocations`).Scan(&allocations); err != nil {
		t.Fatal(err)
	}
	if err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM node_jobs WHERE integration_only=false`).Scan(&jobs); err != nil {
		t.Fatal(err)
	}
	if allocations != 1 || jobs != 1 {
		t.Fatalf("last slot oversold: allocations=%d jobs=%d", allocations, jobs)
	}
}
