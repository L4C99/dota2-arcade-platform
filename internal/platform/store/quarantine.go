package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// ConfigureQuarantineThreshold installs the deployment setting on every
// Platform start. The frozen V1 default is supplied by the caller as 15m.
func (s *Store) ConfigureQuarantineThreshold(ctx context.Context, threshold time.Duration) error {
	if threshold <= 0 || threshold > 30*24*time.Hour {
		return fmt.Errorf("invalid quarantine threshold %s", threshold)
	}
	_, err := s.Pool.Exec(ctx, `UPDATE platform_settings
		SET quarantine_after_node_unreachable=make_interval(secs => $1::double precision)
		WHERE singleton=true`, threshold.Seconds())
	return err
}

// QuarantineOneUnreachable moves one potentially effectful Allocation into
// isolation after its Node's last heartbeat has aged past the configured
// threshold. It never releases capacity or changes an already abandoned
// ServerRequest. The short transaction is safely repeatable after restart.
func (s *Store) QuarantineOneUnreachable(ctx context.Context) (bool, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)
	var allocationID, requestID string
	err = tx.QueryRow(ctx, `SELECT a.id,a.server_request_id FROM allocations a
		JOIN nodes n ON n.id=a.node_id JOIN platform_settings s ON s.singleton=true
		WHERE n.last_heartbeat IS NOT NULL
		AND n.last_heartbeat <= now() - s.quarantine_after_node_unreachable
		AND (a.state IN ('create_unknown','creating','running','stopping','failed_unreclaimed')
			OR (a.state='reserved' AND EXISTS (
				SELECT 1 FROM node_jobs j WHERE j.allocation_id=a.id AND j.kind='create'
				AND j.state IN ('claimed','accepted','unknown','failed_with_effect'))))
		ORDER BY n.last_heartbeat,a.assigned_at,a.id FOR UPDATE OF a SKIP LOCKED LIMIT 1`).Scan(&allocationID, &requestID)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if _, err := tx.Exec(ctx, `UPDATE allocations
		SET state='quarantined',error_code='NODE_UNREACHABLE',quarantined_at=now()
		WHERE id=$1`, allocationID); err != nil {
		return false, err
	}
	if _, err := tx.Exec(ctx, `UPDATE server_requests SET state='quarantined',updated_at=now()
		WHERE id=$1 AND state <> 'abandoned'`, requestID); err != nil {
		return false, err
	}
	if err := pauseNextGameIntent(ctx, tx, requestID); err != nil {
		return false, err
	}
	return true, tx.Commit(ctx)
}

// MarkAllocationQuarantined is an operator action for a diagnosed, occupied
// attempt. It does not stop a process, free capacity, or change node facts.
func (s *Store) MarkAllocationQuarantined(ctx context.Context, allocationID string) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var requestID, state string
	if err := tx.QueryRow(ctx, `SELECT server_request_id,state FROM allocations WHERE id=$1 FOR UPDATE`, allocationID).
		Scan(&requestID, &state); err != nil {
		return err
	}
	if state == "quarantined" {
		return tx.Commit(ctx)
	}
	if state == "reclaimed" || state == "released_no_effect" {
		return fmt.Errorf("%w: resource is already terminal", ErrJobConflict)
	}
	if state == "reserved" {
		var effectPossible bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM node_jobs WHERE allocation_id=$1 AND kind='create'
			AND state <> 'pending')`, allocationID).Scan(&effectPossible); err != nil {
			return err
		}
		if !effectPossible {
			return fmt.Errorf("%w: create has not been claimed", ErrJobConflict)
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE allocations SET state='quarantined',error_code='MANUAL_QUARANTINE',
		quarantined_at=COALESCE(quarantined_at,now()) WHERE id=$1`, allocationID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE server_requests SET state='quarantined',updated_at=now()
		WHERE id=$1 AND state <> 'abandoned'`, requestID); err != nil {
		return err
	}
	if err := pauseNextGameIntent(ctx, tx, requestID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// AbandonQuarantinedUserRequest removes only the old owner's blocking
// business intent. Its Allocation and all core history remain untouched.
func (s *Store) AbandonQuarantinedUserRequest(ctx context.Context, userID, requestID string) (ServerRequest, error) {
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
	if r.State == "abandoned" {
		return r, tx.Commit(ctx)
	}
	if r.State != "quarantined" {
		return ServerRequest{}, fmt.Errorf("%w: request is not quarantined", ErrJobConflict)
	}
	var allocationState string
	if err := tx.QueryRow(ctx, `SELECT state FROM allocations WHERE server_request_id=$1
		ORDER BY attempt_sequence DESC LIMIT 1 FOR UPDATE`, requestID).Scan(&allocationState); err != nil {
		return ServerRequest{}, err
	}
	var pausedIntent bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM next_game_intents
		WHERE source_request_id=$1 AND state='paused')`, requestID).Scan(&pausedIntent); err != nil {
		return ServerRequest{}, err
	}
	if allocationState != "quarantined" && !(allocationState == "reclaimed" && pausedIntent) {
		return ServerRequest{}, fmt.Errorf("%w: Allocation is not quarantined", ErrJobConflict)
	}
	at := time.Now().UTC()
	if _, err := tx.Exec(ctx, `UPDATE server_requests SET state='abandoned',abandoned_at=$2,updated_at=$2
		WHERE id=$1`, requestID, at); err != nil {
		return ServerRequest{}, err
	}
	r.State, r.UpdatedAt = "abandoned", at
	if err := consumeNextGameIntent(ctx, tx, requestID, true); err != nil {
		return ServerRequest{}, err
	}
	return r, tx.Commit(ctx)
}
