package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// CancelUserRequest closes a request only while it is still pure waiting.
// Locking the request serializes this decision with allocation reservation.
func (s *Store) CancelUserRequest(ctx context.Context, userID, requestID string) (ServerRequest, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return ServerRequest{}, err
	}
	defer tx.Rollback(ctx)
	if err := lockUser(ctx, tx, userID); err != nil {
		return ServerRequest{}, err
	}
	var partyID, leaderID string
	err = tx.QueryRow(ctx, `SELECT party_id FROM party_members WHERE user_id=$1`, userID).Scan(&partyID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return ServerRequest{}, err
	}
	ownerField, ownerID := "owner_user_id", userID
	if err == nil {
		if err := tx.QueryRow(ctx, `SELECT leader_user_id FROM parties WHERE id=$1 AND dissolved_at IS NULL FOR UPDATE`, partyID).Scan(&leaderID); err != nil {
			return ServerRequest{}, err
		}
		if leaderID != userID {
			return ServerRequest{}, ErrPartyForbidden
		}
		ownerField, ownerID = "owner_party_id", partyID
	}
	r, err := scanServerRequest(tx.QueryRow(ctx, `SELECT id,arcade_game_id,game_preset_id,state,requested_at,updated_at,node_selection_mode,manual_node_id
		FROM server_requests WHERE id=$1 AND `+ownerField+`=$2 FOR UPDATE`, requestID, ownerID))
	if err != nil {
		return ServerRequest{}, err
	}
	var attempts int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM allocations WHERE server_request_id=$1`, requestID).Scan(&attempts); err != nil {
		return ServerRequest{}, err
	}
	if r.State != "waiting" || attempts != 0 {
		return ServerRequest{}, fmt.Errorf("%w: request is no longer safely waiting", ErrJobConflict)
	}
	r.State, r.UpdatedAt = "cancelled", time.Now().UTC()
	if _, err := tx.Exec(ctx, `UPDATE server_requests SET state='cancelled',updated_at=$2 WHERE id=$1`, requestID, r.UpdatedAt); err != nil {
		return ServerRequest{}, err
	}
	return r, tx.Commit(ctx)
}
