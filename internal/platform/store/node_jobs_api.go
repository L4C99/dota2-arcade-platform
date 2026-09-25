package store

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/L4C99/dota2-arcade-platform/internal/contracts/nodev1"
	"github.com/jackc/pgx/v5"
)

func (s *Store) ClaimNextJob(ctx context.Context, nodeID string) (*nodev1.Job, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	var jobID string
	err = tx.QueryRow(ctx, `SELECT j.id FROM node_jobs j
        JOIN nodes n ON n.id=j.node_id JOIN node_reports r ON r.node_id=n.id
        WHERE j.node_id=$1 AND j.state='pending' AND n.enabled AND r.compatibility_status='compatible'
        AND n.last_heartbeat > $2
        ORDER BY j.created_at,j.id FOR UPDATE OF j SKIP LOCKED LIMIT 1`, nodeID, time.Now().UTC().Add(-nodeOnlineWindow)).Scan(&jobID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, "UPDATE node_jobs SET state='claimed',claimed_at=now(),updated_at=now() WHERE id=$1", jobID); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	job, err := s.JobForNode(ctx, nodeID, jobID)
	if err != nil {
		return nil, err
	}
	return &job, nil
}

func (s *Store) OpenJobsForNode(ctx context.Context, nodeID string) ([]nodev1.Job, error) {
	rows, err := s.Pool.Query(ctx, `SELECT j.id,j.kind,j.state,j.integration_only,
		COALESCE(j.template_binding_key,''),j.requested_port,
        COALESCE(j.instance_id,''),COALESCE(j.operation_id,''),
        COALESCE(e.core_idempotency_key,''),COALESCE(e.resolved_template_path,''),
        COALESCE(e.requested_port,0),COALESCE(encode(e.request_fingerprint,'hex'),''),
        COALESCE(EXTRACT(EPOCH FROM e.prepared_at)::bigint,0)
        FROM node_jobs j LEFT JOIN node_job_executions e ON e.node_job_id=j.id
        WHERE j.node_id=$1 AND j.state IN ('pending','claimed','accepted','unknown')
        ORDER BY j.created_at,j.id`, nodeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	jobs := make([]nodev1.Job, 0)
	for rows.Next() {
		job, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, job)
	}
	return jobs, rows.Err()
}

func (s *Store) JobForNode(ctx context.Context, nodeID, jobID string) (nodev1.Job, error) {
	row := s.Pool.QueryRow(ctx, `SELECT j.id,j.kind,j.state,j.integration_only,
		COALESCE(j.template_binding_key,''),j.requested_port,
        COALESCE(j.instance_id,''),COALESCE(j.operation_id,''),
        COALESCE(e.core_idempotency_key,''),COALESCE(e.resolved_template_path,''),
        COALESCE(e.requested_port,0),COALESCE(encode(e.request_fingerprint,'hex'),''),
        COALESCE(EXTRACT(EPOCH FROM e.prepared_at)::bigint,0)
        FROM node_jobs j LEFT JOIN node_job_executions e ON e.node_job_id=j.id
        WHERE j.node_id=$1 AND j.id=$2`, nodeID, jobID)
	return scanJob(row)
}

type jobScanner interface{ Scan(dest ...any) error }

func scanJob(row jobScanner) (nodev1.Job, error) {
	var job nodev1.Job
	var key, template, fingerprint string
	var port int
	err := row.Scan(&job.ID, &job.Kind, &job.State, &job.IntegrationOnly,
		&job.TemplateBindingKey, &job.RequestedPort,
		&job.InstanceID, &job.OperationID, &key, &template, &port, &fingerprint, &job.PreparedAtUnix)
	if err != nil {
		return nodev1.Job{}, err
	}
	if key != "" {
		job.FrozenCreate = &nodev1.FrozenCreate{IdempotencyKey: key, Template: template, Port: port, FingerprintSHA256: fingerprint}
	}
	return job, nil
}

func permittedTransition(old, next string) bool {
	if old == next {
		return true
	}
	switch old {
	case "claimed":
		return next == "accepted" || next == "unknown" || next == "rejected_no_effect" || next == "failed_with_effect"
	case "accepted":
		return next == "unknown" || next == "succeeded" || next == "failed_with_effect"
	case "unknown":
		return next == "accepted" || next == "succeeded" || next == "rejected_no_effect" || next == "failed_with_effect"
	default:
		return false
	}
}

func (s *Store) ReportJob(ctx context.Context, nodeID, jobID string, report nodev1.ReportRequest) (nodev1.Job, error) {
	if err := report.Validate(); err != nil {
		return nodev1.Job{}, fmt.Errorf("%w: %v", ErrJobConflict, err)
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return nodev1.Job{}, err
	}
	defer tx.Rollback(ctx)
	var oldState, kind, instanceID, operationID, errorCode, errorStage, allocationID string
	err = tx.QueryRow(ctx, `SELECT state,kind,COALESCE(instance_id,''),COALESCE(operation_id,''),
		COALESCE(error_code,''),COALESCE(error_stage,''),COALESCE(allocation_id::text,'')
		FROM node_jobs WHERE id=$1 AND node_id=$2 FOR UPDATE`, jobID, nodeID).
		Scan(&oldState, &kind, &instanceID, &operationID, &errorCode, &errorStage, &allocationID)
	if err != nil {
		return nodev1.Job{}, err
	}
	if !permittedTransition(oldState, report.State) {
		return nodev1.Job{}, fmt.Errorf("%w: invalid transition %s to %s", ErrJobConflict, oldState, report.State)
	}
	oldInstanceID, oldOperationID := instanceID, operationID
	if report.InstanceID != "" && instanceID != "" && report.InstanceID != instanceID {
		return nodev1.Job{}, fmt.Errorf("%w: instance ID changed", ErrJobConflict)
	}
	if report.OperationID != "" && operationID != "" && report.OperationID != operationID {
		return nodev1.Job{}, fmt.Errorf("%w: operation ID changed", ErrJobConflict)
	}
	if report.InstanceID != "" {
		instanceID = report.InstanceID
	}
	if report.OperationID != "" {
		operationID = report.OperationID
	}
	if kind == "create" {
		var hasFrozen bool
		if err := tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM node_job_executions WHERE node_job_id=$1)", jobID).Scan(&hasFrozen); err != nil {
			return nodev1.Job{}, err
		}
		if !hasFrozen && report.State != "rejected_no_effect" {
			return nodev1.Job{}, fmt.Errorf("%w: create job has no frozen execution", ErrJobConflict)
		}
	}
	if report.State == "accepted" || report.State == "succeeded" {
		if instanceID == "" || operationID == "" {
			return nodev1.Job{}, fmt.Errorf("%w: accepted job lacks core IDs", ErrJobConflict)
		}
	}
	if report.State == "rejected_no_effect" {
		if kind != "create" || instanceID != "" || operationID != "" || report.ErrorCode == "" {
			return nodev1.Job{}, fmt.Errorf("%w: no-effect rejection lacks proof", ErrJobConflict)
		}
		// A later validation rejection cannot erase an earlier transport loss.
		// Reconciliation must explicitly assert that the original operation
		// produced no instance; the Controller currently leaves ambiguous
		// unknown calls open instead of making that assertion.
		if oldState == "unknown" && (report.ErrorCode != "RECONCILED_NO_EFFECT" || report.ErrorStage != "reconcile") {
			return nodev1.Job{}, fmt.Errorf("%w: ambiguous create cannot be released by rejection", ErrJobConflict)
		}
	}
	if report.State == "failed_with_effect" && instanceID == "" && operationID == "" {
		return nodev1.Job{}, fmt.Errorf("%w: effectful failure lacks core IDs", ErrJobConflict)
	}
	if allocationID != "" && kind == "create" && report.State == "succeeded" &&
		(report.JoinInfo == nil) == (report.JoinInfoErrorCode == "") {
		return nodev1.Job{}, fmt.Errorf("%w: business Ready requires JoinInfo or structured error", ErrJobConflict)
	}
	if oldState == report.State && instanceID == oldInstanceID && operationID == oldOperationID &&
		errorCode == report.ErrorCode && errorStage == report.ErrorStage {
		if err := tx.Commit(ctx); err != nil {
			return nodev1.Job{}, err
		}
		return s.JobForNode(ctx, nodeID, jobID)
	}
	if oldState == "succeeded" || oldState == "rejected_no_effect" || oldState == "failed_with_effect" {
		return nodev1.Job{}, fmt.Errorf("%w: terminal report cannot change", ErrJobConflict)
	}
	_, err = tx.Exec(ctx, `UPDATE node_jobs SET state=$3,instance_id=NULLIF($4,''),operation_id=NULLIF($5,''),
        error_code=NULLIF($6,''),error_stage=NULLIF($7,''),updated_at=now() WHERE id=$1 AND node_id=$2`,
		jobID, nodeID, report.State, instanceID, operationID, report.ErrorCode, report.ErrorStage)
	if err != nil {
		return nodev1.Job{}, err
	}
	if allocationID != "" {
		if err := applyAllocationJobReport(ctx, tx, allocationID, nodeID, kind, report.State, instanceID,
			report.ErrorCode, report.JoinInfo, report.JoinInfoErrorCode); err != nil {
			return nodev1.Job{}, err
		}
	}
	_, err = tx.Exec(ctx, `INSERT INTO node_job_reports(node_job_id,state,instance_id,operation_id,error_code,error_stage)
        VALUES($1,$2,NULLIF($3,''),NULLIF($4,''),NULLIF($5,''),NULLIF($6,''))`,
		jobID, report.State, instanceID, operationID, report.ErrorCode, report.ErrorStage)
	if err != nil {
		return nodev1.Job{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nodev1.Job{}, err
	}
	return s.JobForNode(ctx, nodeID, jobID)
}

func (f FrozenCreate) Contract() nodev1.FrozenCreate {
	return nodev1.FrozenCreate{IdempotencyKey: f.IdempotencyKey, Template: f.TemplatePath,
		Port: f.Port, FingerprintSHA256: hex.EncodeToString(f.FingerprintSHA)}
}

func applyAllocationJobReport(ctx context.Context, tx pgx.Tx, allocationID, nodeID, kind, state, instanceID, errorCode string,
	join *nodev1.JoinInfo, joinErrorCode string) error {
	var requestID, selectionMode, currentAllocationState string
	if err := tx.QueryRow(ctx, `SELECT a.server_request_id,r.node_selection_mode,a.state FROM allocations a
		JOIN server_requests r ON r.id=a.server_request_id WHERE a.id=$1 FOR UPDATE OF a`, allocationID).
		Scan(&requestID, &selectionMode, &currentAllocationState); err != nil {
		return err
	}
	// Quarantine is a business and capacity safety boundary. A late create
	// success or accepted report may close its NodeJob, but it cannot put the
	// old Allocation back in ordinary running/creating state. Only a proved
	// resource terminal report may resolve it.
	if currentAllocationState == "quarantined" && !(kind == "stop" && state == "succeeded") &&
		!(kind == "create" && state == "rejected_no_effect") {
		return nil
	}
	at := time.Now().UTC()
	allocationState, requestState := "", ""
	switch kind {
	case "create":
		switch state {
		case "accepted":
			allocationState, requestState = "creating", "creating"
		case "unknown":
			allocationState, requestState = "create_unknown", "creating"
		case "succeeded":
			allocationState, requestState = "running", "running"
		case "rejected_no_effect":
			allocationState, requestState = "released_no_effect", "unavailable"
			if selectionMode == "auto" {
				requestState = "waiting"
			}
		case "failed_with_effect":
			if errorCode == "IDENTITY_UNVERIFIED" {
				allocationState, requestState = "quarantined", "quarantined"
			} else {
				allocationState, requestState = "failed_unreclaimed", "failed_unreclaimed"
			}
		}
	case "stop":
		switch state {
		case "accepted", "unknown":
			allocationState, requestState = "stopping", "stopping"
		case "succeeded":
			allocationState, requestState = "reclaimed", "ended"
			if currentAllocationState == "quarantined" {
				var pausedIntent bool
				if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM next_game_intents
					WHERE source_request_id=$1 AND state='paused')`, requestID).Scan(&pausedIntent); err != nil {
					return err
				}
				if pausedIntent {
					// A paused next-game intent still needs the owner to
					// decide whether to continue after quarantine.
					requestState = "quarantined"
				}
			}
		case "failed_with_effect":
			allocationState, requestState = "quarantined", "quarantined"
		}
	}
	if allocationState == "" {
		return fmt.Errorf("%w: unsupported business job report", ErrJobConflict)
	}
	_, err := tx.Exec(ctx, `UPDATE allocations SET state=$2,error_code=NULLIF($3,''),
		ready_at=CASE WHEN $2='running' THEN COALESCE(ready_at,$4) ELSE ready_at END,
		reclaimed_at=CASE WHEN $2='reclaimed' THEN COALESCE(reclaimed_at,$4) ELSE reclaimed_at END,
		quarantined_at=CASE WHEN $2='quarantined' THEN COALESCE(quarantined_at,$4) ELSE quarantined_at END
		WHERE id=$1`, allocationID, allocationState, errorCode, at)
	if err != nil {
		return err
	}
	if kind == "create" && state == "succeeded" {
		if join != nil {
			var currentRevision string
			if err := tx.QueryRow(ctx, `SELECT entry_config_revision FROM node_entry_capabilities WHERE node_id=$1`, nodeID).Scan(&currentRevision); err != nil {
				return err
			}
			if join.EntryConfigRevision != currentRevision {
				join, joinErrorCode = nil, "NETWORK_CONFIG_CHANGED"
			}
		}
		if join != nil {
			_, err = tx.Exec(ctx, `UPDATE allocations SET join_local_port=$2,join_public_port=$3,
				join_connect_host=$4,join_protocol_ip=NULLIF($5,''),join_entry_config_revision=$6,
				join_info_error_code=NULL,join_info_available_at=COALESCE(join_info_available_at,$7)
				WHERE id=$1`, allocationID, join.LocalPort, join.PublicPort, join.ConnectHost, join.ProtocolIP,
				join.EntryConfigRevision, at)
		} else {
			_, err = tx.Exec(ctx, `UPDATE allocations SET join_info_error_code=$2 WHERE id=$1`, allocationID, joinErrorCode)
		}
		if err != nil {
			return err
		}
	}
	_, err = tx.Exec(ctx, `UPDATE server_requests SET state=$2,updated_at=$3 WHERE id=$1 AND state <> 'abandoned'`, requestID, requestState, at)
	if err != nil {
		return err
	}
	if allocationState == "quarantined" {
		if err := pauseNextGameIntent(ctx, tx, requestID); err != nil {
			return err
		}
	}
	if kind == "stop" && state == "succeeded" && currentAllocationState != "quarantined" {
		if err := consumeNextGameIntent(ctx, tx, requestID, false); err != nil {
			return err
		}
	}
	if kind == "create" && state == "failed_with_effect" && instanceID != "" && errorCode != "IDENTITY_UNVERIFIED" {
		stopJobID, err := NewID()
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `INSERT INTO node_jobs(id,node_id,kind,integration_only,allocation_id,instance_id)
			VALUES($1,$2,'stop',false,$3,$4) ON CONFLICT DO NOTHING`, stopJobID, nodeID, allocationID, instanceID)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `UPDATE allocations SET state='stopping' WHERE id=$1`, allocationID)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `UPDATE server_requests SET state='stopping',updated_at=$2 WHERE id=$1`, requestID, at)
		if err != nil {
			return err
		}
	}
	return nil
}
