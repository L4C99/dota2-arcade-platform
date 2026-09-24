package store

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"

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
        AND n.last_heartbeat > now() - interval '2 minutes'
        ORDER BY j.created_at,j.id FOR UPDATE OF j SKIP LOCKED LIMIT 1`, nodeID).Scan(&jobID)
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
        COALESCE(j.instance_id,''),COALESCE(j.operation_id,''),
        COALESCE(e.core_idempotency_key,''),COALESCE(e.resolved_template_path,''),
        COALESCE(e.requested_port,0),COALESCE(encode(e.request_fingerprint,'hex'),'')
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
        COALESCE(j.instance_id,''),COALESCE(j.operation_id,''),
        COALESCE(e.core_idempotency_key,''),COALESCE(e.resolved_template_path,''),
        COALESCE(e.requested_port,0),COALESCE(encode(e.request_fingerprint,'hex'),'')
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
		&job.InstanceID, &job.OperationID, &key, &template, &port, &fingerprint)
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
	var oldState, kind, instanceID, operationID, errorCode, errorStage string
	err = tx.QueryRow(ctx, `SELECT state,kind,COALESCE(instance_id,''),COALESCE(operation_id,''),
        COALESCE(error_code,''),COALESCE(error_stage,'') FROM node_jobs WHERE id=$1 AND node_id=$2 FOR UPDATE`, jobID, nodeID).
		Scan(&oldState, &kind, &instanceID, &operationID, &errorCode, &errorStage)
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
		if !hasFrozen {
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
	}
	if report.State == "failed_with_effect" && instanceID == "" && operationID == "" {
		return nodev1.Job{}, fmt.Errorf("%w: effectful failure lacks core IDs", ErrJobConflict)
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
