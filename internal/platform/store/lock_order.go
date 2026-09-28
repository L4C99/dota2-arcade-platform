package store

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// lockBusinessAllocation uses unlocked IDs only to find the lock set. It
// rechecks ownership after taking Request -> all Jobs by ID -> Allocation.
// Callers may already hold the same Request lock, but must not hold a Job or
// Allocation lock before this function.
func lockBusinessAllocation(ctx context.Context, tx pgx.Tx, allocationID, nodeID string) (requestID *string, purpose string, err error) {
	var discoveredRequest *string
	var discoveredPurpose string
	err = tx.QueryRow(ctx, `SELECT server_request_id,purpose FROM allocations
		WHERE id=$1 AND node_id=$2`, allocationID, nodeID).Scan(&discoveredRequest, &discoveredPurpose)
	if err != nil {
		return nil, "", err
	}
	if discoveredRequest != nil {
		var locked string
		if err = tx.QueryRow(ctx, `SELECT id FROM server_requests WHERE id=$1 FOR UPDATE`, *discoveredRequest).Scan(&locked); err != nil {
			return nil, "", err
		}
	}
	rows, err := tx.Query(ctx, `SELECT id FROM node_jobs WHERE allocation_id=$1 ORDER BY id`, allocationID)
	if err != nil {
		return nil, "", err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return nil, "", err
		}
		ids = append(ids, id)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return nil, "", err
	}
	rows.Close()
	for _, id := range ids {
		var locked string
		if err = tx.QueryRow(ctx, `SELECT id FROM node_jobs WHERE id=$1 FOR UPDATE`, id).Scan(&locked); err != nil {
			return nil, "", err
		}
	}
	var actualRequest *string
	var actualPurpose, actualNode string
	err = tx.QueryRow(ctx, `SELECT server_request_id,purpose,node_id FROM allocations WHERE id=$1 FOR UPDATE`, allocationID).
		Scan(&actualRequest, &actualPurpose, &actualNode)
	if err != nil {
		return nil, "", err
	}
	if actualNode != nodeID || actualPurpose != discoveredPurpose || (actualRequest == nil) != (discoveredRequest == nil) ||
		(actualRequest != nil && *actualRequest != *discoveredRequest) {
		return nil, "", fmt.Errorf("%w: Allocation owner changed", ErrJobConflict)
	}
	return actualRequest, actualPurpose, nil
}

// lockJobBusinessRows handles an integration-only Job or resolves its
// Allocation before taking any row lock.
func lockJobBusinessRows(ctx context.Context, tx pgx.Tx, nodeID, jobID string) (allocationID string, requestID *string, purpose string, err error) {
	err = tx.QueryRow(ctx, `SELECT COALESCE(allocation_id::text,'') FROM node_jobs WHERE id=$1 AND node_id=$2`, jobID, nodeID).Scan(&allocationID)
	if err != nil {
		return "", nil, "", err
	}
	if allocationID != "" {
		requestID, purpose, err = lockBusinessAllocation(ctx, tx, allocationID, nodeID)
		if err != nil {
			return "", nil, "", err
		}
		var actual string
		if err = tx.QueryRow(ctx, `SELECT COALESCE(allocation_id::text,'') FROM node_jobs WHERE id=$1 AND node_id=$2`, jobID, nodeID).Scan(&actual); err != nil {
			return "", nil, "", err
		}
		if actual != allocationID {
			return "", nil, "", fmt.Errorf("%w: Job Allocation changed", ErrJobConflict)
		}
		return allocationID, requestID, purpose, nil
	}
	var locked string
	if err = tx.QueryRow(ctx, `SELECT id FROM node_jobs WHERE id=$1 AND node_id=$2 FOR UPDATE`, jobID, nodeID).Scan(&locked); err != nil {
		return "", nil, "", err
	}
	return "", nil, "", nil
}

// Recheck authority after the Request row is locked, without holding User or
// Party rows ahead of Request. Holding User first deadlocks with report paths
// that update a Request and perform PostgreSQL owner FK checks.
func requireCurrentRequestOwner(ctx context.Context, tx pgx.Tx, requestID, userID string) error {
	var authorized bool
	if err := tx.QueryRow(ctx, `SELECT
		(COALESCE(r.owner_user_id=$2,false) AND NOT EXISTS(SELECT 1 FROM party_members WHERE user_id=$2)) OR
		EXISTS(SELECT 1 FROM party_members m JOIN parties p ON p.id=m.party_id
		 WHERE m.user_id=$2 AND m.party_id=r.owner_party_id AND p.leader_user_id=$2 AND p.dissolved_at IS NULL)
		FROM server_requests r WHERE r.id=$1`, requestID, userID).Scan(&authorized); err != nil {
		return err
	}
	if !authorized {
		return ErrPartyForbidden
	}
	return nil
}
