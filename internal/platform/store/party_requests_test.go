package store

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
)

func TestP2BPartyOwnerAndLeaderPermissions(t *testing.T) {
	s := playerTestStore(t)
	ctx := context.Background()
	if err := s.ConfigurePartySize(ctx, 3); err != nil {
		t.Fatal(err)
	}
	gameID, presetID := seedPlayerCatalog(t, s)
	solo := partyUser(t, s)
	soloRequest, created, err := s.CreateUserRequest(ctx, solo, gameID, presetID)
	if err != nil || !created {
		t.Fatalf("solo create: %+v %t %v", soloRequest, created, err)
	}
	var soloOwner, soloParty *string
	if err := s.Pool.QueryRow(ctx, `SELECT owner_user_id,owner_party_id FROM server_requests WHERE id=$1`, soloRequest.ID).Scan(&soloOwner, &soloParty); err != nil || soloOwner == nil || *soloOwner != solo || soloParty != nil {
		t.Fatalf("solo owner: %v %v %v", soloOwner, soloParty, err)
	}

	leader, member, outsider := partyUser(t, s), partyUser(t, s), partyUser(t, s)
	partyID, err := s.CreateParty(ctx, leader)
	if err != nil {
		t.Fatal(err)
	}
	invite, err := s.CurrentInvite(ctx, leader)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.JoinParty(ctx, member, invite.Token); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.CreateUserRequest(ctx, member, gameID, presetID); !errors.Is(err, ErrPartyForbidden) {
		t.Fatalf("member submitted: %v", err)
	}
	request, created, err := s.CreateUserRequest(ctx, leader, gameID, presetID)
	if err != nil || !created {
		t.Fatalf("leader create: %+v %t %v", request, created, err)
	}
	var partyOwner, userOwner *string
	if err := s.Pool.QueryRow(ctx, `SELECT owner_party_id,owner_user_id FROM server_requests WHERE id=$1`, request.ID).Scan(&partyOwner, &userOwner); err != nil || partyOwner == nil || *partyOwner != partyID || userOwner != nil {
		t.Fatalf("party owner: %v %v %v", partyOwner, userOwner, err)
	}
	duplicate, created, err := s.CreateUserRequest(ctx, leader, gameID, presetID)
	if err != nil || created || duplicate.ID != request.ID {
		t.Fatalf("duplicate: %+v %t %v", duplicate, created, err)
	}
	for _, user := range []string{leader, member} {
		current, err := s.CurrentUserRequest(ctx, user)
		if err != nil || current == nil || current.ID != request.ID {
			t.Fatalf("current %s: %+v %v", user, current, err)
		}
		got, err := s.UserRequest(ctx, user, request.ID)
		if err != nil || got.ID != request.ID {
			t.Fatalf("lookup %s: %+v %v", user, got, err)
		}
	}
	if _, err := s.UserRequest(ctx, outsider, request.ID); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("foreign lookup: %v", err)
	}
	if _, err := s.StopUserRequest(ctx, member, request.ID); !errors.Is(err, ErrPartyForbidden) {
		t.Fatalf("member stop: %v", err)
	}
	if _, err := s.StopUserRequest(ctx, leader, soloRequest.ID); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("foreign stop: %v", err)
	}
	if err := s.DisbandParty(ctx, leader); !errors.Is(err, ErrPartyBusy) {
		t.Fatalf("blocking disband: %v", err)
	}
	if err := s.LeaveParty(ctx, leader); !errors.Is(err, ErrPartyForbidden) {
		t.Fatalf("leader leave: %v", err)
	}
	var count int
	if err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM server_requests WHERE owner_party_id=$1`, partyID).Scan(&count); err != nil || count != 1 {
		t.Fatalf("request count=%d err=%v", count, err)
	}
}

func TestP2BPresetSizeBeforeRequest(t *testing.T) {
	s := playerTestStore(t)
	ctx := context.Background()
	if err := s.ConfigurePartySize(ctx, 3); err != nil {
		t.Fatal(err)
	}
	gameID, presetID := seedPlayerCatalog(t, s)
	if _, err := s.Pool.Exec(ctx, `UPDATE game_presets SET max_players=1 WHERE id=$1`, presetID); err != nil {
		t.Fatal(err)
	}
	leader, member := partyUser(t, s), partyUser(t, s)
	partyID, err := s.CreateParty(ctx, leader)
	if err != nil {
		t.Fatal(err)
	}
	invite, err := s.CurrentInvite(ctx, leader)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.JoinParty(ctx, member, invite.Token); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.CreateUserRequest(ctx, leader, gameID, presetID); !errors.Is(err, ErrPresetPartyTooLarge) {
		t.Fatalf("preset limit: %v", err)
	}
	var count int
	if err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM server_requests WHERE owner_party_id=$1`, partyID).Scan(&count); err != nil || count != 0 {
		t.Fatalf("request inserted before size check: %d %v", count, err)
	}
	if _, err := s.Pool.Exec(ctx, `UPDATE game_presets SET max_players=2 WHERE id=$1`, presetID); err != nil {
		t.Fatal(err)
	}
	if _, created, err := s.CreateUserRequest(ctx, leader, gameID, presetID); err != nil || !created {
		t.Fatalf("within limit: %t %v", created, err)
	}
}
