package store

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestA4CatalogIDValidationBeforeWrite(t *testing.T) {
	s := playerTestStore(t)
	ctx := context.Background()
	game, _ := seedPlayerCatalog(t, s)
	admin, err := s.CreateAdmin(ctx, "ids-admin", "a long test password")
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range []AdminAction{
		{Action: "game.create", WorkshopID: strings.Repeat("1", 21), DisplayName: "invalid"},
		{Action: "content.create", TargetID: game, ContentVersionID: "CURRENT", ContentSHA256: strings.Repeat("a", 64)},
		{Action: "content.create", TargetID: game, ContentVersionID: "a/b", ContentSHA256: strings.Repeat("a", 64)},
	} {
		if err := s.ApplyAdminAction(ctx, admin, a); !errors.Is(err, ErrInvalidAdminAction) {
			t.Fatal(err)
		}
	}
	var n int
	if err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM audit_events WHERE actor_admin_user_id=$1`, admin).Scan(&n); err != nil || n != 0 {
		t.Fatalf("invalid writes audited %d %v", n, err)
	}
}
