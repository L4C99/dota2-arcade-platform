package store

import (
	"context"
	"regexp"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

var chineseDisplayName = regexp.MustCompile(`^[\p{Han}]{4,6}$`)

func TestDisplayNameVocabulary(t *testing.T) {
	if len(nameModifiers) < 40 || len(nameNouns) < 40 {
		t.Fatalf("small vocabulary: %d x %d", len(nameModifiers), len(nameNouns))
	}
	for i := 0; i < 100; i++ {
		name, err := newDisplayName()
		if err != nil || !chineseDisplayName.MatchString(name) {
			t.Fatalf("invalid generated name %q: %v", name, err)
		}
	}
}

func TestDisplayNameSessionPersistenceAndIdentity(t *testing.T) {
	s := playerTestStore(t)
	ctx := context.Background()
	if err := s.ConfigurePartySize(ctx, 3); err != nil {
		t.Fatal(err)
	}
	leader, token, err := s.CreateUserSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	name, err := s.UserDisplayName(ctx, leader)
	if err != nil || !chineseDisplayName.MatchString(name) {
		t.Fatalf("new User name %q: %v", name, err)
	}
	if got, err := s.UserForToken(ctx, token); err != nil || got != leader {
		t.Fatalf("session restore: %q %v", got, err)
	}
	reopenedPool, err := pgxpool.NewWithConfig(ctx, s.Pool.Config())
	if err != nil {
		t.Fatal(err)
	}
	defer reopenedPool.Close()
	reopened := &Store{Pool: reopenedPool}
	if err := reopened.ApplyMigrations(ctx); err != nil {
		t.Fatal(err)
	}
	if got, err := reopened.UserDisplayName(ctx, leader); err != nil || got != name {
		t.Fatalf("name changed after reopen: %q %v", got, err)
	}
	if _, err := s.CreateParty(ctx, leader); err != nil {
		t.Fatal(err)
	}
	member, _, err := s.CreateUserSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	// Duplicate presentation names must never merge identities or change auth.
	if _, err := s.Pool.Exec(ctx, `UPDATE users SET display_name=$2 WHERE id=$1`, member, name); err != nil {
		t.Fatal(err)
	}
	invite, err := s.CurrentInvite(ctx, leader)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.JoinParty(ctx, member, invite.Token); err != nil {
		t.Fatal(err)
	}
	party, err := s.CurrentParty(ctx, member)
	if err != nil || party == nil || len(party.Members) != 2 {
		t.Fatalf("Party with duplicate names: %+v %v", party, err)
	}
	if party.Members[0].DisplayName != name || party.Members[1].DisplayName != name || party.Members[0].UserID == party.Members[1].UserID {
		t.Fatalf("names/IDs: %+v", party.Members)
	}
	if err := s.RemovePartyMember(ctx, member, leader); err != ErrPartyForbidden {
		t.Fatalf("same-name member removed leader: %v", err)
	}
	if err := s.RemovePartyMember(ctx, leader, member); err != nil {
		t.Fatal(err)
	}
}
