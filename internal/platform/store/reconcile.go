package store

import (
	"context"
	"fmt"

	"github.com/L4C99/dota2-arcade-platform/internal/contracts/nodev1"
)

func (s *Store) ActiveAllocationsForNode(ctx context.Context, nodeID string) ([]nodev1.ActiveAllocation, error) {
	return s.ActiveAllocationsForNodeWithCapability(ctx, nodeID, nodev1.RequiredContentValidationV102)
}

func (s *Store) ActiveAllocationsForNodeWithCapability(ctx context.Context, nodeID, capability string) ([]nodev1.ActiveAllocation, error) {
	rows, err := s.Pool.Query(ctx, `SELECT a.id,j.instance_id,a.state,
		EXISTS(SELECT 1 FROM node_jobs active WHERE active.allocation_id=a.id AND active.state IN ('pending','claimed','accepted','unknown'))
		FROM allocations a JOIN node_jobs j ON j.allocation_id=a.id AND j.kind='create' AND j.instance_id IS NOT NULL
		WHERE a.node_id=$1 AND j.required_capability IN ('legacy_v1',$2)
		AND a.state NOT IN ('reclaimed','released_no_effect') ORDER BY a.assigned_at,a.id`, nodeID, capability)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []nodev1.ActiveAllocation{}
	for rows.Next() {
		var x nodev1.ActiveAllocation
		if err := rows.Scan(&x.ID, &x.InstanceID, &x.State, &x.HasOpenJob); err != nil {
			return nil, err
		}
		result = append(result, x)
	}
	return result, rows.Err()
}

// ReportInstanceFact accepts only status observed by the authenticated
// Controller for the immutable create-job instance identity. It never treats
// a missing response as reclaim, and it never rewrites an Allocation attempt.
func (s *Store) ReportInstanceFact(ctx context.Context, nodeID, allocationID string, f nodev1.InstanceFact) error {
	if f.InstanceID == "" || f.Port < 0 || f.Port > 65535 || (f.Outcome != "active" && f.Outcome != "reclaimed" && f.Outcome != "identity_unverified" && f.Outcome != "uncertain") {
		return ErrJobConflict
	}
	if f.Outcome == "reclaimed" && (f.Lifecycle != "reclaimed" || f.Process != "stopped" || f.Cleanup != "complete") {
		return ErrJobConflict
	}
	if f.Outcome == "active" && (f.Lifecycle != "active" || f.Process != "running") {
		return ErrJobConflict
	}
	if f.JoinInfo != nil || f.JoinInfoErrorCode != "" {
		if f.Outcome != "active" || f.Room != "ready" {
			return ErrJobConflict
		}
		if err := (nodev1.ReportRequest{State: "succeeded", JoinInfo: f.JoinInfo, JoinInfoErrorCode: f.JoinInfoErrorCode}).Validate(); err != nil {
			return ErrJobConflict
		}
		if f.JoinInfo != nil && f.JoinInfo.LocalPort != f.Port {
			return ErrJobConflict
		}
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var discoveredRequest *string
	if err := tx.QueryRow(ctx, `SELECT server_request_id FROM allocations WHERE id=$1 AND node_id=$2`, allocationID, nodeID).Scan(&discoveredRequest); err != nil {
		return err
	}
	if discoveredRequest != nil {
		var lockedRequest string
		if err := tx.QueryRow(ctx, `SELECT id FROM server_requests WHERE id=$1 FOR UPDATE`, *discoveredRequest).Scan(&lockedRequest); err != nil {
			return err
		}
	}
	var lockedNode string
	if err := tx.QueryRow(ctx, `SELECT id FROM nodes WHERE id=$1 FOR SHARE`, nodeID).Scan(&lockedNode); err != nil {
		return err
	}
	var entryRevision string
	if f.JoinInfo != nil {
		if err := tx.QueryRow(ctx, `SELECT entry_config_revision FROM node_entry_capabilities WHERE node_id=$1 FOR SHARE`, nodeID).Scan(&entryRevision); err != nil {
			return err
		}
	}
	if _, _, err := lockBusinessAllocation(ctx, tx, allocationID, nodeID); err != nil {
		return err
	}
	var requestID, runID *string
	var state, expectedID string
	var joinPort int
	if err := tx.QueryRow(ctx, `SELECT a.server_request_id,a.validation_run_id,a.state,j.instance_id,COALESCE(a.join_local_port,0)
		FROM allocations a JOIN node_jobs j ON j.allocation_id=a.id AND j.kind='create'
		WHERE a.id=$1 AND a.node_id=$2`, allocationID, nodeID).Scan(&requestID, &runID, &state, &expectedID, &joinPort); err != nil {
		return err
	}
	if expectedID != f.InstanceID {
		return fmt.Errorf("%w: instance identity changed", ErrJobConflict)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO allocation_reconcile_facts(allocation_id,instance_id,outcome,lifecycle,process,cleanup,port)
		VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT(allocation_id) DO UPDATE SET
		instance_id=EXCLUDED.instance_id,outcome=EXCLUDED.outcome,lifecycle=EXCLUDED.lifecycle,
		process=EXCLUDED.process,cleanup=EXCLUDED.cleanup,port=EXCLUDED.port,reported_at=now()`, allocationID, f.InstanceID, f.Outcome, f.Lifecycle, f.Process, f.Cleanup, f.Port); err != nil {
		return err
	}
	if state == "reclaimed" || state == "released_no_effect" {
		return tx.Commit(ctx)
	}
	if f.JoinInfo != nil || f.JoinInfoErrorCode != "" {
		if state != "running" {
			return ErrJobConflict
		}
		if f.JoinInfo != nil {
			if f.JoinInfo.EntryConfigRevision != entryRevision {
				return ErrJobConflict
			}
			if joinPort == 0 || joinPort == f.Port {
				j := f.JoinInfo
				if _, err := tx.Exec(ctx, `UPDATE allocations SET join_local_port=$2,join_public_port=$3,join_connect_host=$4,join_protocol_ip=NULLIF($5,''),join_entry_config_revision=$6,join_info_error_code=NULL,join_info_available_at=COALESCE(join_info_available_at,now()) WHERE id=$1`, allocationID, j.LocalPort, j.PublicPort, j.ConnectHost, j.ProtocolIP, j.EntryConfigRevision); err != nil {
					return err
				}
			}
		} else {
			if _, err := tx.Exec(ctx, `UPDATE allocations SET join_info_error_code=$2 WHERE id=$1`, allocationID, f.JoinInfoErrorCode); err != nil {
				return err
			}
		}
	}
	if f.Outcome == "reclaimed" {
		if _, err := tx.Exec(ctx, `UPDATE allocations SET state='reclaimed',reclaimed_at=COALESCE(reclaimed_at,now()) WHERE id=$1`, allocationID); err != nil {
			return err
		}
		var paused bool
		if requestID != nil {
			if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM next_game_intents WHERE source_request_id=$1 AND state='paused')`, requestID).Scan(&paused); err != nil {
				return err
			}
		}
		if requestID != nil && !paused {
			if _, err := tx.Exec(ctx, `UPDATE server_requests SET state='ended',updated_at=now() WHERE id=$1 AND state<>'abandoned'`, requestID); err != nil {
				return err
			}
			if err := consumeNextGameIntent(ctx, tx, *requestID, false); err != nil {
				return err
			}
		}
		if runID != nil {
			if err := advanceValidationFromFact(ctx, tx, *runID, "reclaimed", f.JoinInfo != nil); err != nil {
				return err
			}
		}
	} else if f.Outcome == "uncertain" && f.Lifecycle == "failed" && (f.Process == "stopped" || f.Process == "running") && state != "quarantined" && state != "stopping" {
		if err := applyAllocationJobReport(ctx, tx, allocationID, nodeID, "create", "failed_with_effect", expectedID, "INSTANCE_FAILED", nil, ""); err != nil {
			return err
		}
	} else if f.Outcome == "identity_unverified" || f.Outcome == "active" && joinPort != 0 && f.Port != joinPort {
		code := "IDENTITY_UNVERIFIED"
		if f.Outcome == "active" {
			code = "PORT_IDENTITY_CHANGED"
		}
		if _, err := tx.Exec(ctx, `UPDATE allocations SET state='quarantined',error_code=$2,quarantined_at=COALESCE(quarantined_at,now()) WHERE id=$1`, allocationID, code); err != nil {
			return err
		}
		if requestID != nil {
			if _, err := tx.Exec(ctx, `UPDATE server_requests SET state='quarantined',updated_at=now() WHERE id=$1 AND state<>'abandoned'`, requestID); err != nil {
				return err
			}
			if err := pauseNextGameIntent(ctx, tx, *requestID); err != nil {
				return err
			}
		}
		if runID != nil {
			if err := advanceValidationFromFact(ctx, tx, *runID, "quarantined", false); err != nil {
				return err
			}
		}
	}
	if runID != nil && f.Outcome == "active" && f.Room == "ready" {
		if err := advanceValidationFromFact(ctx, tx, *runID, "ready", f.JoinInfo != nil); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
