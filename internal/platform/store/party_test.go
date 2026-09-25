package store

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"
)

func partyUser(t *testing.T, s *Store) string {
	t.Helper()
	id, _, err := s.CreateUserSession(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestP2APartyMembershipAndInvite(t *testing.T) {
	s := playerTestStore(t)
	ctx := context.Background()
	if _, err := s.CreateParty(ctx, partyUser(t, s)); !errors.Is(err, ErrPartySizeUnset) {
		t.Fatalf("unset size: %v", err)
	}
	if err := s.ConfigurePartySize(ctx, 2); err != nil {
		t.Fatal(err)
	}
	leader := partyUser(t, s)
	partyID, err := s.CreateParty(ctx, leader)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateParty(ctx, leader); !errors.Is(err, ErrAlreadyInParty) {
		t.Fatalf("second party: %v", err)
	}
	party, err := s.CurrentParty(ctx, leader)
	if err != nil || party == nil || party.ID != partyID || party.CurrentRole != "leader" || len(party.Members) != 1 {
		t.Fatalf("party: %+v %v", party, err)
	}
	invite, err := s.CurrentInvite(ctx, leader)
	if err != nil || len(invite.Token) != 43 {
		t.Fatalf("invite unavailable: %v", err)
	}
	member := partyUser(t, s)
	if _, err := s.JoinParty(ctx, member, invite.Token); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CurrentInvite(ctx, member); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("member invite access: %v", err)
	}
	if _, err := s.JoinParty(ctx, partyUser(t, s), invite.Token); !errors.Is(err, ErrPartyFull) {
		t.Fatalf("full party: %v", err)
	}
	if _, err := s.ResetInvite(ctx, member); !errors.Is(err, ErrPartyForbidden) {
		t.Fatalf("member reset: %v", err)
	}
	newInvite, err := s.ResetInvite(ctx, leader)
	if err != nil {
		t.Fatal(err)
	}
	if newInvite.Token == invite.Token {
		t.Fatal("reset reused invite")
	}
	if err := s.LeaveParty(ctx, member); err != nil {
		t.Fatal(err)
	}
	if _, err := s.JoinParty(ctx, member, invite.Token); !errors.Is(err, ErrInvalidInvite) {
		t.Fatalf("old invite accepted: %v", err)
	}
	if _, err := s.JoinParty(ctx, member, newInvite.Token); err != nil {
		t.Fatal(err)
	}
	if err := s.RemovePartyMember(ctx, member, leader); !errors.Is(err, ErrPartyForbidden) {
		t.Fatalf("member removed leader: %v", err)
	}
	if err := s.RemovePartyMember(ctx, leader, leader); !errors.Is(err, ErrCannotRemoveLeader) {
		t.Fatalf("removed leader: %v", err)
	}
	if err := s.RemovePartyMember(ctx, leader, member); err != nil {
		t.Fatal(err)
	}
	if err := s.DisbandParty(ctx, leader); err != nil {
		t.Fatal(err)
	}
	if p, err := s.CurrentParty(ctx, leader); err != nil || p != nil {
		t.Fatalf("disband: %+v %v", p, err)
	}
	if _, err := s.JoinParty(ctx, member, newInvite.Token); !errors.Is(err, ErrInvalidInvite) {
		t.Fatalf("disband invite accepted: %v", err)
	}
}

func TestP2AConcurrentLastSlotAndMultiParty(t *testing.T) {
	s := playerTestStore(t)
	ctx := context.Background()
	if err := s.ConfigurePartySize(ctx, 2); err != nil {
		t.Fatal(err)
	}
	leaderA, leaderB := partyUser(t, s), partyUser(t, s)
	if _, err := s.CreateParty(ctx, leaderA); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateParty(ctx, leaderB); err != nil {
		t.Fatal(err)
	}
	inviteA, _ := s.CurrentInvite(ctx, leaderA)
	inviteB, _ := s.CurrentInvite(ctx, leaderB)
	userA, userB := partyUser(t, s), partyUser(t, s)
	var wg sync.WaitGroup
	results := make([]error, 2)
	for i, user := range []string{userA, userB} {
		wg.Add(1)
		go func(i int, user string) { defer wg.Done(); _, results[i] = s.JoinParty(ctx, user, inviteA.Token) }(i, user)
	}
	wg.Wait()
	success := 0
	for _, err := range results {
		if err == nil {
			success++
		} else if !errors.Is(err, ErrPartyFull) {
			t.Fatalf("last slot: %v", err)
		}
	}
	if success != 1 {
		t.Fatalf("last slot successes=%d", success)
	}
	userC := partyUser(t, s)
	for i, token := range []string{inviteA.Token, inviteB.Token} {
		wg.Add(1)
		go func(i int, token string) { defer wg.Done(); _, results[i] = s.JoinParty(ctx, userC, token) }(i, token)
	}
	wg.Wait()
	// A is full, so B can accept the shared user exactly once.
	if !(errors.Is(results[0], ErrPartyFull) || errors.Is(results[0], ErrAlreadyInParty)) || results[1] != nil {
		t.Fatalf("multi-party results: %v", results)
	}
	if p, err := s.CurrentParty(ctx, userC); err != nil || p == nil || p.LeaderUserID != leaderB {
		t.Fatalf("membership: %+v %v", p, err)
	}
}
