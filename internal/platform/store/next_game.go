package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

type NextGameIntent struct {
	SourceRequestID string     `json:"sourceRequestId"`
	State           string     `json:"state"`
	NewRequestID    *string    `json:"newRequestId,omitempty"`
	CreatedAt       time.Time  `json:"createdAt"`
	ConsumedAt      *time.Time `json:"consumedAt,omitempty"`
}

func (s *Store) UserNextGameIntent(ctx context.Context, userID, requestID string) (*NextGameIntent, error) {
	if _, err := s.UserRequest(ctx, userID, requestID); err != nil {
		return nil, err
	}
	var intent NextGameIntent
	err := s.Pool.QueryRow(ctx, `SELECT source_request_id,state,new_request_id,created_at,consumed_at
		FROM next_game_intents WHERE source_request_id=$1`, requestID).
		Scan(&intent.SourceRequestID, &intent.State, &intent.NewRequestID, &intent.CreatedAt, &intent.ConsumedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &intent, nil
}

// consumeNextGameIntent runs inside the same transaction that proves full
// reclaim (or the explicit owner abandon transaction after quarantine). The
// unique source key and row lock make repeated reports and clicks idempotent.
func consumeNextGameIntent(ctx context.Context, tx pgx.Tx, sourceRequestID string, afterAbandon bool) error {
	var state string
	err := tx.QueryRow(ctx, `SELECT state FROM next_game_intents WHERE source_request_id=$1 FOR UPDATE`, sourceRequestID).Scan(&state)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if state == "consumed" || (state == "paused" && !afterAbandon) {
		return nil
	}
	var ownerUserID, ownerPartyID, manualNodeID *string
	var gameID, presetID, mode string
	if err := tx.QueryRow(ctx, `SELECT owner_user_id,owner_party_id,arcade_game_id,game_preset_id,
		node_selection_mode,manual_node_id FROM server_requests WHERE id=$1`, sourceRequestID).
		Scan(&ownerUserID, &ownerPartyID, &gameID, &presetID, &mode, &manualNodeID); err != nil {
		return err
	}
	newID, err := NewID()
	if err != nil {
		return err
	}
	createdAt := time.Now().UTC()
	if _, err := tx.Exec(ctx, `INSERT INTO server_requests(id,owner_user_id,owner_party_id,
		arcade_game_id,game_preset_id,state,requested_at,updated_at,node_selection_mode,manual_node_id)
		VALUES($1,$2,$3,$4,$5,'waiting',$6,$6,$7,$8)`, newID, ownerUserID, ownerPartyID,
		gameID, presetID, createdAt, mode, manualNodeID); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE next_game_intents SET state='consumed',new_request_id=$2,
		consumed_at=$3,updated_at=$3 WHERE source_request_id=$1`, sourceRequestID, newID, createdAt)
	return err
}

func pauseNextGameIntent(ctx context.Context, tx pgx.Tx, sourceRequestID string) error {
	_, err := tx.Exec(ctx, `UPDATE next_game_intents SET state='paused',updated_at=now()
		WHERE source_request_id=$1 AND state='pending'`, sourceRequestID)
	return err
}
