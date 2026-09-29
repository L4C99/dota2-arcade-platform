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
	return s.claimNextJob(ctx, nodeID, false, nodev1.RequiredContentValidationV102)
}

func (s *Store) ClaimNextJobWithCapability(ctx context.Context, nodeID, capability string) (*nodev1.Job, error) {
	return s.claimNextJob(ctx, nodeID, false, capability)
}

// ClaimIndependentStop retains the restricted A.4 stop route.
func (s *Store) ClaimIndependentStop(ctx context.Context, nodeID string) (*nodev1.Job, error) {
	return s.claimNextJob(ctx, nodeID, true, nodev1.RequiredContentValidationV102)
}

func (s *Store) ClaimIndependentStopWithCapability(ctx context.Context, nodeID, capability string) (*nodev1.Job, error) {
	return s.claimNextJob(ctx, nodeID, true, capability)
}

func (s *Store) claimNextJob(ctx context.Context, nodeID string, independentStop bool, capability string) (*nodev1.Job, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	// Serialize claims on this node so two concurrent Controllers cannot each
	// pass the unresolved-dependency predicate for different pending rows.
	var lockedNode string
	if err := tx.QueryRow(ctx, `SELECT id FROM nodes WHERE id=$1 FOR UPDATE`, nodeID).Scan(&lockedNode); err != nil {
		return nil, err
	}
	var jobID string
	err = tx.QueryRow(ctx, `SELECT j.id FROM node_jobs j
        JOIN nodes n ON n.id=j.node_id JOIN node_reports r ON r.node_id=n.id
        WHERE j.node_id=$1 AND j.state='pending' AND n.enabled AND r.compatibility_status='compatible'
		AND j.required_capability IN ('legacy_v1',$4)
        AND n.last_heartbeat > $2
		AND NOT EXISTS(SELECT 1 FROM node_jobs other WHERE other.node_id=j.node_id AND other.id<>j.id
		  AND other.state IN ('claimed','accepted','unknown')
		  AND ((j.allocation_id IS NOT NULL AND other.allocation_id=j.allocation_id)
		    OR (j.instance_id IS NOT NULL AND other.instance_id=j.instance_id)))
        AND (NOT $3 OR (j.kind='stop' AND j.instance_id IS NOT NULL
          AND EXISTS(SELECT 1 FROM node_jobs c JOIN allocations a ON a.id=c.allocation_id
            WHERE c.allocation_id=j.allocation_id AND c.kind='create' AND c.node_id=j.node_id
            AND c.instance_id=j.instance_id AND c.state IN ('succeeded','failed_with_effect')
            AND COALESCE(c.error_code,'')<>'IDENTITY_UNVERIFIED'
            AND a.state NOT IN ('reclaimed','released_no_effect'))
          AND NOT EXISTS(SELECT 1 FROM node_jobs other WHERE other.node_id=j.node_id AND other.id<>j.id
            AND other.state IN ('claimed','accepted','unknown')
            AND (other.allocation_id=j.allocation_id OR other.instance_id=j.instance_id))))
        ORDER BY j.created_at,j.id FOR UPDATE OF j SKIP LOCKED LIMIT 1`, nodeID, time.Now().UTC().Add(-nodeOnlineWindow), independentStop, capability).Scan(&jobID)
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
	job, err := s.JobForNodeWithCapability(ctx, nodeID, jobID, capability)
	if err != nil {
		return nil, err
	}
	return &job, nil
}

func (s *Store) OpenJobsForNode(ctx context.Context, nodeID string) ([]nodev1.Job, error) {
	return s.OpenJobsForNodeWithCapability(ctx, nodeID, nodev1.RequiredContentValidationV102)
}

func (s *Store) OpenJobsForNodeWithCapability(ctx context.Context, nodeID, capability string) ([]nodev1.Job, error) {
	rows, err := s.Pool.Query(ctx, `SELECT j.id,j.kind,j.state,j.integration_only,
		COALESCE(j.template_binding_key,''),j.requested_port,
        COALESCE(j.instance_id,''),COALESCE(j.operation_id,''),
        COALESCE(e.core_idempotency_key,''),COALESCE(e.resolved_template_path,''),
        COALESCE(e.requested_port,0),COALESCE(encode(e.request_fingerprint,'hex'),''),
		COALESCE(EXTRACT(EPOCH FROM e.prepared_at)::bigint,0),j.required_capability,
		COALESCE(j.expected_template_fingerprint_sha256,''),COALESCE(j.template_binding_generation,0),
		COALESCE(j.expected_workshop_id,''),COALESCE(j.expected_content_version_id,''),
		COALESCE(j.expected_vpk_sha256,''),
		COALESCE(e.template_manifest_algorithm,''),COALESCE(e.template_fingerprint_sha256,'')
        FROM node_jobs j LEFT JOIN node_job_executions e ON e.node_job_id=j.id
        WHERE j.node_id=$1 AND j.required_capability IN ('legacy_v1',$2) AND j.state IN ('pending','claimed','accepted','unknown')
        ORDER BY j.created_at,j.id`, nodeID, capability)
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
	return s.JobForNodeWithCapability(ctx, nodeID, jobID, nodev1.RequiredContentValidationV102)
}

func (s *Store) JobForNodeWithCapability(ctx context.Context, nodeID, jobID, capability string) (nodev1.Job, error) {
	row := s.Pool.QueryRow(ctx, `SELECT j.id,j.kind,j.state,j.integration_only,
		COALESCE(j.template_binding_key,''),j.requested_port,
        COALESCE(j.instance_id,''),COALESCE(j.operation_id,''),
        COALESCE(e.core_idempotency_key,''),COALESCE(e.resolved_template_path,''),
        COALESCE(e.requested_port,0),COALESCE(encode(e.request_fingerprint,'hex'),''),
		COALESCE(EXTRACT(EPOCH FROM e.prepared_at)::bigint,0),j.required_capability,
		COALESCE(j.expected_template_fingerprint_sha256,''),COALESCE(j.template_binding_generation,0),
		COALESCE(j.expected_workshop_id,''),COALESCE(j.expected_content_version_id,''),
		COALESCE(j.expected_vpk_sha256,''),
		COALESCE(e.template_manifest_algorithm,''),COALESCE(e.template_fingerprint_sha256,'')
        FROM node_jobs j LEFT JOIN node_job_executions e ON e.node_job_id=j.id
        WHERE j.node_id=$1 AND j.id=$2 AND j.required_capability IN ('legacy_v1',$3)`, nodeID, jobID, capability)
	return scanJob(row)
}

type jobScanner interface{ Scan(dest ...any) error }

func scanJob(row jobScanner) (nodev1.Job, error) {
	var job nodev1.Job
	var key, template, fingerprint string
	var port int
	var manifestAlgorithm, manifestSHA, requiredCapability string
	err := row.Scan(&job.ID, &job.Kind, &job.State, &job.IntegrationOnly,
		&job.TemplateBindingKey, &job.RequestedPort,
		&job.InstanceID, &job.OperationID, &key, &template, &port, &fingerprint, &job.PreparedAtUnix,
		&requiredCapability, &job.ExpectedTemplateFingerprintSHA256, &job.TemplateBindingGeneration,
		&job.ExpectedWorkshopID, &job.ExpectedContentVersionID, &job.ExpectedVPKSHA256,
		&manifestAlgorithm, &manifestSHA)
	if err != nil {
		return nodev1.Job{}, err
	}
	if requiredCapability != "legacy_v1" {
		job.RequiredCapability = requiredCapability
	}
	if key != "" {
		job.FrozenCreate = &nodev1.FrozenCreate{IdempotencyKey: key, Template: template, Port: port, FingerprintSHA256: fingerprint,
			RequiredCapability: job.RequiredCapability, TemplateManifestAlgorithm: manifestAlgorithm, TemplateFingerprintSHA256: manifestSHA}
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
	return s.reportJob(ctx, nodeID, jobID, report, nil)
}

// BeginOperation is the durable linearization point before a v1.0.2 local
// core call. Unknown is intentionally retained even if the Controller dies
// before calling core; it is not evidence of a no-effect create.
func (s *Store) BeginOperation(ctx context.Context, nodeID, jobID string, input nodev1.OperationStartRequest) (nodev1.Job, error) {
	if err := input.Validate(); err != nil {
		return nodev1.Job{}, fmt.Errorf("%w: %v", ErrJobConflict, err)
	}
	return s.reportJob(ctx, nodeID, jobID, nodev1.ReportRequest{State: "unknown"}, &input)
}

func (s *Store) reportJob(ctx context.Context, nodeID, jobID string, report nodev1.ReportRequest, start *nodev1.OperationStartRequest) (nodev1.Job, error) {
	if err := report.Validate(); err != nil {
		return nodev1.Job{}, fmt.Errorf("%w: %v", ErrJobConflict, err)
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return nodev1.Job{}, err
	}
	defer tx.Rollback(ctx)
	// Discover the player owner without locking, then take public Node before
	// any Job/Allocation lock. The helper rechecks all discovered relations.
	var discoveredRequest *string
	if err := tx.QueryRow(ctx, `SELECT a.server_request_id FROM node_jobs j
		LEFT JOIN allocations a ON a.id=j.allocation_id WHERE j.id=$1 AND j.node_id=$2`, jobID, nodeID).Scan(&discoveredRequest); err != nil {
		return nodev1.Job{}, err
	}
	if discoveredRequest != nil {
		var lockedRequest string
		if err := tx.QueryRow(ctx, `SELECT id FROM server_requests WHERE id=$1 FOR UPDATE`, *discoveredRequest).Scan(&lockedRequest); err != nil {
			return nodev1.Job{}, err
		}
	}
	var lockedNode string
	if err := tx.QueryRow(ctx, `SELECT id FROM nodes WHERE id=$1 FOR SHARE`, nodeID).Scan(&lockedNode); err != nil {
		return nodev1.Job{}, err
	}
	if _, _, _, err = lockJobBusinessRows(ctx, tx, nodeID, jobID); err != nil {
		return nodev1.Job{}, err
	}
	var oldState, kind, instanceID, operationID, errorCode, errorStage, allocationID, requiredCapability string
	err = tx.QueryRow(ctx, `SELECT state,kind,COALESCE(instance_id,''),COALESCE(operation_id,''),
		COALESCE(error_code,''),COALESCE(error_stage,''),COALESCE(allocation_id::text,''),required_capability
		FROM node_jobs WHERE id=$1 AND node_id=$2`, jobID, nodeID).
		Scan(&oldState, &kind, &instanceID, &operationID, &errorCode, &errorStage, &allocationID, &requiredCapability)
	if err != nil {
		return nodev1.Job{}, err
	}
	if start != nil {
		if requiredCapability != nodev1.RequiredContentValidationV102 || kind != start.Kind ||
			(oldState != "claimed" && oldState != "unknown") || operationID != "" {
			return nodev1.Job{}, fmt.Errorf("%w: operation cannot start", ErrJobConflict)
		}
		if kind == "stop" {
			if instanceID == "" || instanceID != start.InstanceID {
				return nodev1.Job{}, fmt.Errorf("%w: stop instance changed", ErrJobConflict)
			}
		} else {
			var frozen nodev1.FrozenCreate
			err := tx.QueryRow(ctx, `SELECT core_idempotency_key,resolved_template_path,requested_port,
				encode(request_fingerprint,'hex'),required_capability,COALESCE(template_manifest_algorithm,''),
				COALESCE(template_fingerprint_sha256,'') FROM node_job_executions WHERE node_job_id=$1`, jobID).
				Scan(&frozen.IdempotencyKey, &frozen.Template, &frozen.Port, &frozen.FingerprintSHA256,
					&frozen.RequiredCapability, &frozen.TemplateManifestAlgorithm, &frozen.TemplateFingerprintSHA256)
			if err != nil || frozen != *start.FrozenCreate || instanceID != "" {
				return nodev1.Job{}, fmt.Errorf("%w: frozen create changed", ErrJobConflict)
			}
		}
		if oldState == "unknown" {
			if err := tx.Commit(ctx); err != nil {
				return nodev1.Job{}, err
			}
			return s.JobForNode(ctx, nodeID, jobID)
		}
	}
	if requiredCapability == nodev1.RequiredContentValidationV102 && oldState == "claimed" &&
		(report.State == "accepted" || report.State == "succeeded") {
		return nodev1.Job{}, fmt.Errorf("%w: missing durable operation start", ErrJobConflict)
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
		// Fixed core v0.1.1 cannot fence a previously sent unknown create.
		// Even a later post-key-lookup rejection is not a no-effect proof.
		if oldState == "unknown" || report.ErrorCode == "RECONCILED_NO_EFFECT" {
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
	capability := f.RequiredCapability
	if capability == "legacy_v1" {
		capability = ""
	}
	return nodev1.FrozenCreate{IdempotencyKey: f.IdempotencyKey, Template: f.TemplatePath,
		Port: f.Port, FingerprintSHA256: hex.EncodeToString(f.FingerprintSHA), RequiredCapability: capability,
		TemplateManifestAlgorithm: f.TemplateManifestAlgorithm, TemplateFingerprintSHA256: f.TemplateFingerprintSHA256}
}

func applyAllocationJobReport(ctx context.Context, tx pgx.Tx, allocationID, nodeID, kind, state, instanceID, errorCode string,
	join *nodev1.JoinInfo, joinErrorCode string) error {
	var requestID *string
	var runID *string
	var selectionMode, currentAllocationState string
	if err := tx.QueryRow(ctx, `SELECT a.server_request_id,a.validation_run_id,COALESCE(r.node_selection_mode,''),a.state FROM allocations a
		LEFT JOIN server_requests r ON r.id=a.server_request_id WHERE a.id=$1`, allocationID).
		Scan(&requestID, &runID, &selectionMode, &currentAllocationState); err != nil {
		return err
	}
	// Resource terminal states are monotonic. Keep the late job report as
	// history, but never mutate this attempt or its request again: a fresh
	// attempt or next-game request may already own the business slot.
	if currentAllocationState == "reclaimed" || currentAllocationState == "released_no_effect" {
		return nil
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
			var pausedIntent bool
			if requestID != nil {
				if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM next_game_intents
				WHERE source_request_id=$1 AND state='paused')`, requestID).Scan(&pausedIntent); err != nil {
					return err
				}
			}
			if pausedIntent {
				// A paused next-game intent still needs the owner to
				// decide whether to continue after quarantine.
				requestState = "quarantined"
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
	if requestID != nil {
		_, err = tx.Exec(ctx, `UPDATE server_requests SET state=$2,updated_at=$3 WHERE id=$1 AND state <> 'abandoned'`, requestID, requestState, at)
		if err != nil {
			return err
		}
	}
	if runID != nil {
		if err := advanceValidationFromJob(ctx, tx, *runID, kind, state, allocationState, errorCode, join != nil); err != nil {
			return err
		}
	}
	if allocationState == "quarantined" && requestID != nil {
		if err := pauseNextGameIntent(ctx, tx, *requestID); err != nil {
			return err
		}
	}
	if kind == "stop" && state == "succeeded" && currentAllocationState != "quarantined" && requestID != nil {
		if err := consumeNextGameIntent(ctx, tx, *requestID, false); err != nil {
			return err
		}
	}
	if kind == "create" && state == "failed_with_effect" && instanceID != "" && errorCode != "IDENTITY_UNVERIFIED" {
		stopJobID, err := NewID()
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `INSERT INTO node_jobs(id,node_id,kind,integration_only,allocation_id,instance_id,required_capability)
			VALUES($1,$2,'stop',false,$3,$4,(SELECT required_capability FROM node_jobs WHERE allocation_id=$3 AND kind='create')) ON CONFLICT DO NOTHING`, stopJobID, nodeID, allocationID, instanceID)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `UPDATE allocations SET state='stopping' WHERE id=$1`, allocationID)
		if err != nil {
			return err
		}
		if requestID != nil {
			_, err = tx.Exec(ctx, `UPDATE server_requests SET state='stopping',updated_at=$2 WHERE id=$1 AND state<>'abandoned'`, requestID, at)
			if err != nil {
				return err
			}
		}
	}
	return nil
}
