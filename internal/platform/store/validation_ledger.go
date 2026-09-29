package store

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"
)

// ValidationRun is an immutable candidate identity with monotonic lifecycle evidence.
// The Allocation and NodeJobs are read by relation, never copied into mutable IDs here.
type ValidationRun struct {
	ID, NodeID, GameID, PresetID, ContentVersionID, ContentSHA256                          string
	TemplateRevisionID, TemplateBindingKey, TemplateFingerprintSHA256                      string
	StartedBy                                                                              string
	TemplateBindingGeneration, MaintenanceEpoch, ContentFactRevision, TemplateFactRevision int64
	State, FailureCode, ResultCode, HumanResult                                            string
	HumanConfirmedBy                                                                       *string
	HumanConfirmedAt, ReadyAt, JoinInfoAvailableAt, PassedAt                               *time.Time
	CreatedAt                                                                              time.Time
	AllocationID, CreateJobID, StopJobID                                                   string
}

type ValidationCandidate struct {
	NodeID, GameID, PresetID, ContentVersionID, TemplateRevisionID, AdminID string
	ExpectedMaintenanceEpoch                                                int64
	ExpectedTemplateFingerprintSHA256                                       string
}

type ScopedMaintenanceStatus struct {
	NodeID, GameID                                     string
	MaintenanceEpoch                                   int64
	Closed                                             bool
	TargetOccupied, TargetUnresolvedJobs, NodeOccupied int
}

// ScopedMaintenanceStatus is an observation for Admin/I2/I3. A consuming
// transaction must repeat the gate under the Node lock before taking action.
func (s *Store) ScopedMaintenanceStatus(ctx context.Context, nodeID, gameID string) (ScopedMaintenanceStatus, error) {
	var x ScopedMaintenanceStatus
	x.NodeID, x.GameID = nodeID, gameID
	err := s.Pool.QueryRow(ctx, `SELECT b.maintenance_epoch,NOT b.accepting_new_allocations,
		(SELECT count(*) FROM allocations a WHERE a.node_id=$1 AND a.arcade_game_id=$2
		 AND a.state NOT IN ('reclaimed','released_no_effect')),
		(SELECT count(*) FROM node_jobs j JOIN allocations a ON a.id=j.allocation_id
		 WHERE a.node_id=$1 AND a.arcade_game_id=$2 AND j.state IN ('pending','claimed','accepted','unknown')),
		(SELECT count(*) FROM allocations a WHERE a.node_id=$1
		 AND a.state NOT IN ('reclaimed','released_no_effect'))
		FROM node_content_bindings b WHERE b.node_id=$1 AND b.arcade_game_id=$2`, nodeID, gameID).
		Scan(&x.MaintenanceEpoch, &x.Closed, &x.TargetOccupied, &x.TargetUnresolvedJobs, &x.NodeOccupied)
	return x, err
}

// BeginScopedMaintenance serializes with scheduler reservations at the Node
// lock. A request ID returns the prior epoch only for the identical operation.
func (s *Store) BeginScopedMaintenance(ctx context.Context, nodeID, gameID, adminID, requestID, reason string, expectedEpoch int64) (int64, error) {
	if requestID == "" || len(requestID) > 128 || reason == "" || expectedEpoch < 0 {
		return 0, ErrJobConflict
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	var epoch int64
	var oldNode, oldGame, oldAdmin, oldReason string
	err = tx.QueryRow(ctx, `SELECT node_id,arcade_game_id,actor_admin_user_id,reason,epoch
		FROM maintenance_begin_requests WHERE request_id=$1`, requestID).
		Scan(&oldNode, &oldGame, &oldAdmin, &oldReason, &epoch)
	if err == nil {
		if oldNode != nodeID || oldGame != gameID || oldAdmin != adminID || oldReason != reason || epoch != expectedEpoch+1 {
			return 0, ErrJobConflict
		}
		return epoch, tx.Commit(ctx)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return 0, err
	}
	var locked string
	if err := tx.QueryRow(ctx, `SELECT id FROM nodes WHERE id=$1 FOR UPDATE`, nodeID).Scan(&locked); err != nil {
		return 0, err
	}
	err = tx.QueryRow(ctx, `UPDATE node_content_bindings SET accepting_new_allocations=false,
		maintenance_epoch=maintenance_epoch+1 WHERE node_id=$1 AND arcade_game_id=$2
		AND maintenance_epoch=$3 RETURNING maintenance_epoch`, nodeID, gameID, expectedEpoch).Scan(&epoch)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrJobConflict
	}
	if err != nil {
		return 0, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO maintenance_begin_requests
		(request_id,node_id,arcade_game_id,epoch,actor_admin_user_id,reason)
		VALUES($1,$2,$3,$4,$5,$6)`, requestID, nodeID, gameID, epoch, adminID, reason); err != nil {
		return 0, err
	}
	if err := auditAdmin(ctx, tx, adminID, "maintenance.begin", "node_content_binding", nodeID+":"+gameID,
		map[string]any{"epoch": epoch, "reason": reason, "requestId": requestID}); err != nil {
		return 0, err
	}
	return epoch, tx.Commit(ctx)
}

// UpsertTemplateBinding changes generation only when its declared identity changes.
func (s *Store) UpsertTemplateBinding(ctx context.Context, nodeID, revisionID, key, fingerprint string, expectedGeneration int64) (int64, error) {
	if key == "" || len(key) > 128 || !contentSHA256Pattern.MatchString(fingerprint) || expectedGeneration < 0 {
		return 0, ErrJobConflict
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	var locked string
	if err := tx.QueryRow(ctx, `SELECT id FROM nodes WHERE id=$1 FOR UPDATE`, nodeID).Scan(&locked); err != nil {
		return 0, err
	}
	var oldKey string
	var oldFingerprint *string
	var generation int64
	err = tx.QueryRow(ctx, `SELECT binding_key,expected_template_fingerprint_sha256,binding_generation
		FROM node_template_bindings WHERE node_id=$1 AND template_revision_id=$2 FOR UPDATE`, nodeID, revisionID).
		Scan(&oldKey, &oldFingerprint, &generation)
	if errors.Is(err, pgx.ErrNoRows) {
		if expectedGeneration != 0 {
			return 0, ErrJobConflict
		}
		generation = 1
		_, err = tx.Exec(ctx, `INSERT INTO node_template_bindings
			(node_id,template_revision_id,binding_key,expected_template_fingerprint_sha256,binding_generation)
			VALUES($1,$2,$3,$4,1)`, nodeID, revisionID, key, fingerprint)
	} else if err == nil {
		if generation != expectedGeneration {
			return 0, ErrJobConflict
		}
		if oldKey != key || oldFingerprint == nil || *oldFingerprint != fingerprint {
			generation++
			_, err = tx.Exec(ctx, `UPDATE node_template_bindings SET binding_key=$3,
				expected_template_fingerprint_sha256=$4,binding_generation=$5
				WHERE node_id=$1 AND template_revision_id=$2`, nodeID, revisionID, key, fingerprint, generation)
		}
	}
	if err != nil {
		return 0, err
	}
	return generation, tx.Commit(ctx)
}

// RecordTemplateFact is a Store entry point for a future authenticated
// Controller report. It never writes the administrator's expected identity.
func (s *Store) RecordTemplateFact(ctx context.Context, nodeID, revisionID, key, state, algorithm, fingerprint string) (int64, error) {
	if state != "unknown" && state != "confirmed" {
		return 0, ErrJobConflict
	}
	if state == "confirmed" && (key == "" || algorithm != "template-manifest-sha256-v1" || !contentSHA256Pattern.MatchString(fingerprint)) {
		return 0, ErrJobConflict
	}
	if state == "unknown" {
		key, algorithm, fingerprint = "", "", ""
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	var locked string
	if err := tx.QueryRow(ctx, `SELECT id FROM nodes WHERE id=$1 FOR UPDATE`, nodeID).Scan(&locked); err != nil {
		return 0, err
	}
	var revision int64
	err = tx.QueryRow(ctx, `INSERT INTO node_template_facts
		(node_id,template_revision_id,binding_key,reported_state,manifest_algorithm,reported_fingerprint_sha256,template_fact_revision,received_at)
		VALUES($1,$2,NULLIF($3,''),$4,NULLIF($5,''),NULLIF($6,''),CASE WHEN $4='confirmed' THEN 1 ELSE 0 END,now())
		ON CONFLICT(node_id,template_revision_id) DO UPDATE SET
		template_fact_revision=node_template_facts.template_fact_revision+CASE WHEN
		(node_template_facts.binding_key,node_template_facts.reported_state,node_template_facts.manifest_algorithm,node_template_facts.reported_fingerprint_sha256)
		IS DISTINCT FROM
		(EXCLUDED.binding_key,EXCLUDED.reported_state,EXCLUDED.manifest_algorithm,EXCLUDED.reported_fingerprint_sha256)
		THEN 1 ELSE 0 END,
		binding_key=EXCLUDED.binding_key,reported_state=EXCLUDED.reported_state,
		manifest_algorithm=EXCLUDED.manifest_algorithm,reported_fingerprint_sha256=EXCLUDED.reported_fingerprint_sha256,
		received_at=EXCLUDED.received_at RETURNING template_fact_revision`, nodeID, revisionID, key, state, algorithm, fingerprint).Scan(&revision)
	if err != nil {
		return 0, err
	}
	return revision, tx.Commit(ctx)
}

type InventoryInstance struct{ InstanceID, Lifecycle, Process, Cleanup, CurrentOperationID string }
type InventorySnapshot struct {
	NodeID, ScanID, State, ErrorCode string
	Complete                         bool
	ReceivedAt                       time.Time
	Instances                        []InventoryInstance
	UnaccountedInstanceIDs           []string
}

// RecordInventory persists a whole scan and its accounting under the Node
// lock. A partial scan remains unknown even when its instance list is empty.
func (s *Store) RecordInventory(ctx context.Context, nodeID, scanID string, complete bool, errorCode string, instances []InventoryInstance) (InventorySnapshot, error) {
	if scanID == "" || len(scanID) > 128 || complete && errorCode != "" || !complete && errorCode == "" {
		return InventorySnapshot{}, ErrJobConflict
	}
	if !complete && len(instances) != 0 {
		return InventorySnapshot{}, ErrJobConflict
	}
	seen := make(map[string]bool, len(instances))
	for _, x := range instances {
		if x.InstanceID == "" || seen[x.InstanceID] || x.Lifecycle == "" || x.Process == "" || x.Cleanup == "" {
			return InventorySnapshot{}, ErrJobConflict
		}
		seen[x.InstanceID] = true
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return InventorySnapshot{}, err
	}
	defer tx.Rollback(ctx)
	var locked string
	if err := tx.QueryRow(ctx, `SELECT id FROM nodes WHERE id=$1 FOR UPDATE`, nodeID).Scan(&locked); err != nil {
		return InventorySnapshot{}, err
	}
	var duplicate bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM node_inventory_snapshots WHERE node_id=$1 AND scan_id=$2)`, nodeID, scanID).Scan(&duplicate); err != nil {
		return InventorySnapshot{}, err
	}
	if duplicate {
		return InventorySnapshot{}, ErrJobConflict
	}
	result := InventorySnapshot{NodeID: nodeID, ScanID: scanID, Complete: complete, ErrorCode: errorCode, State: "unknown", Instances: instances, UnaccountedInstanceIDs: []string{}}
	if complete {
		result.State = "confirmed"
		for _, x := range instances {
			if x.Lifecycle == "unknown" || x.Process == "unknown" || x.Cleanup == "unknown" {
				result.State = "unknown"
			}
		}
	}
	accounted := make(map[string]string)
	if complete {
		rows, err := tx.Query(ctx, `SELECT j.instance_id,a.id FROM node_jobs j JOIN allocations a ON a.id=j.allocation_id
			WHERE j.node_id=$1 AND j.kind='create' AND j.instance_id IS NOT NULL
			AND a.node_id=$1 AND a.state NOT IN ('reclaimed','released_no_effect')`, nodeID)
		if err != nil {
			return InventorySnapshot{}, err
		}
		for rows.Next() {
			var instanceID, allocationID string
			if err := rows.Scan(&instanceID, &allocationID); err != nil {
				rows.Close()
				return InventorySnapshot{}, err
			}
			accounted[instanceID] = allocationID
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return InventorySnapshot{}, err
		}
		rows.Close()
	}
	for _, x := range instances {
		if accounted[x.InstanceID] == "" {
			result.UnaccountedInstanceIDs = append(result.UnaccountedInstanceIDs, x.InstanceID)
		}
	}
	sort.Strings(result.UnaccountedInstanceIDs)
	err = tx.QueryRow(ctx, `INSERT INTO node_inventory_snapshots
		(node_id,scan_id,complete,state,error_code,instance_count,unaccounted_count,unaccounted_instance_ids)
		VALUES($1,$2,$3,$4,NULLIF($5,''),$6,$7,$8) RETURNING received_at`, nodeID, scanID, complete, result.State, errorCode, len(instances), len(result.UnaccountedInstanceIDs), result.UnaccountedInstanceIDs).Scan(&result.ReceivedAt)
	if err != nil {
		return InventorySnapshot{}, err
	}
	for _, x := range instances {
		var owner *string
		if allocationID := accounted[x.InstanceID]; allocationID != "" {
			owner = &allocationID
		}
		if _, err := tx.Exec(ctx, `INSERT INTO node_inventory_instances
			(node_id,scan_id,instance_id,lifecycle,process,cleanup,current_operation_id,accounted_allocation_id)
			VALUES($1,$2,$3,$4,$5,$6,NULLIF($7,''),$8)`, nodeID, scanID, x.InstanceID, x.Lifecycle, x.Process, x.Cleanup, x.CurrentOperationID, owner); err != nil {
			return InventorySnapshot{}, err
		}
	}
	return result, tx.Commit(ctx)
}

// ReserveValidation atomically creates one Run, capacity occupying Allocation,
// and pending create Job. New Controller facts are prerequisites, never
// synthesized from an administrator's expected values.
func (s *Store) ReserveValidation(ctx context.Context, c ValidationCandidate) (ValidationRun, error) {
	if c.ExpectedMaintenanceEpoch <= 0 || !contentSHA256Pattern.MatchString(c.ExpectedTemplateFingerprintSHA256) {
		return ValidationRun{}, ErrJobConflict
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return ValidationRun{}, err
	}
	defer tx.Rollback(ctx)
	var contentSHA string
	if err := tx.QueryRow(ctx, `SELECT v.content_sha256 FROM arcade_games g
		JOIN game_presets p ON p.arcade_game_id=g.id AND p.id=$2
		JOIN content_versions v ON v.arcade_game_id=g.id AND v.id=$3
		WHERE g.id=$1 FOR SHARE OF g,p`, c.GameID, c.PresetID, c.ContentVersionID).
		Scan(&contentSHA); err != nil {
		return ValidationRun{}, err
	}
	var enabled bool
	var heartbeat *time.Time
	var hard, desired int
	var compatibility string
	var capabilities []string
	if err := tx.QueryRow(ctx, `SELECT n.enabled,n.last_heartbeat,n.desired_max_instances,
		COALESCE(r.hard_max_instances,0),COALESCE(r.compatibility_status,''),COALESCE(r.capabilities,'{}')
		FROM nodes n LEFT JOIN node_reports r ON r.node_id=n.id WHERE n.id=$1 FOR UPDATE OF n`, c.NodeID).
		Scan(&enabled, &heartbeat, &desired, &hard, &compatibility, &capabilities); err != nil {
		return ValidationRun{}, err
	}
	capable := false
	for _, capability := range capabilities {
		if capability == "contentValidationV102" {
			capable = true
		}
	}
	if !enabled || NodeConnectivity(heartbeat, time.Now().UTC()) != "online" || compatibility != "compatible" || !capable {
		return ValidationRun{}, fmt.Errorf("%w: node proof unavailable", ErrJobConflict)
	}
	var epoch, contentRevision int64
	var closed bool
	var contentState, reportedVersion, reportedSHA string
	if err := tx.QueryRow(ctx, `SELECT maintenance_epoch,NOT accepting_new_allocations,content_fact_revision,
		reported_state,COALESCE(reported_content_version_id,''),COALESCE(reported_content_sha256,'')
		FROM node_content_bindings WHERE node_id=$1 AND arcade_game_id=$2 FOR SHARE`, c.NodeID, c.GameID).
		Scan(&epoch, &closed, &contentRevision, &contentState, &reportedVersion, &reportedSHA); err != nil {
		return ValidationRun{}, err
	}
	if !closed || epoch != c.ExpectedMaintenanceEpoch || contentRevision == 0 || contentState != "confirmed" || reportedVersion != c.ContentVersionID || reportedSHA != contentSHA {
		return ValidationRun{}, fmt.Errorf("%w: content gate", ErrJobConflict)
	}
	var key, expectedFingerprint string
	var generation int64
	if err := tx.QueryRow(ctx, `SELECT binding_key,COALESCE(expected_template_fingerprint_sha256,''),binding_generation
		FROM node_template_bindings WHERE node_id=$1 AND template_revision_id=$2 FOR SHARE`, c.NodeID, c.TemplateRevisionID).
		Scan(&key, &expectedFingerprint, &generation); err != nil {
		return ValidationRun{}, err
	}
	var factKey, factState, factAlgorithm, factFingerprint string
	var templateRevision int64
	var templateReceived *time.Time
	if err := tx.QueryRow(ctx, `SELECT COALESCE(binding_key,''),reported_state,COALESCE(manifest_algorithm,''),
		COALESCE(reported_fingerprint_sha256,''),template_fact_revision,received_at
		FROM node_template_facts WHERE node_id=$1 AND template_revision_id=$2`, c.NodeID, c.TemplateRevisionID).
		Scan(&factKey, &factState, &factAlgorithm, &factFingerprint, &templateRevision, &templateReceived); err != nil {
		return ValidationRun{}, err
	}
	if generation == 0 || expectedFingerprint != c.ExpectedTemplateFingerprintSHA256 || factState != "confirmed" || factAlgorithm != "template-manifest-sha256-v1" || factKey != key || factFingerprint != expectedFingerprint || templateRevision == 0 || templateReceived == nil || time.Since(*templateReceived) >= nodeOnlineWindow {
		return ValidationRun{}, fmt.Errorf("%w: template gate", ErrJobConflict)
	}
	var inventoryComplete bool
	var inventoryState string
	var inventoryReceived time.Time
	var unaccounted int
	if err := tx.QueryRow(ctx, `SELECT complete,state,received_at,unaccounted_count FROM node_inventory_snapshots
		WHERE node_id=$1 ORDER BY received_at DESC,scan_id DESC LIMIT 1`, c.NodeID).
		Scan(&inventoryComplete, &inventoryState, &inventoryReceived, &unaccounted); err != nil {
		return ValidationRun{}, err
	}
	if !inventoryComplete || inventoryState != "confirmed" || time.Since(inventoryReceived) >= nodeOnlineWindow || unaccounted != 0 {
		return ValidationRun{}, fmt.Errorf("%w: inventory gate", ErrJobConflict)
	}
	var oldResources, openJobs, occupied int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM allocations WHERE node_id=$1 AND arcade_game_id=$2
		AND state NOT IN ('reclaimed','released_no_effect')`, c.NodeID, c.GameID).Scan(&oldResources); err != nil {
		return ValidationRun{}, err
	}
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM node_jobs j JOIN allocations a ON a.id=j.allocation_id
		WHERE a.node_id=$1 AND a.arcade_game_id=$2 AND j.state IN ('pending','claimed','accepted','unknown')`, c.NodeID, c.GameID).Scan(&openJobs); err != nil {
		return ValidationRun{}, err
	}
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM allocations WHERE node_id=$1
		AND state NOT IN ('reclaimed','released_no_effect')`, c.NodeID).Scan(&occupied); err != nil {
		return ValidationRun{}, err
	}
	if oldResources != 0 || openJobs != 0 || occupied >= min(hard, desired) {
		return ValidationRun{}, fmt.Errorf("%w: resources or capacity", ErrJobConflict)
	}
	var integrationJobs int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM node_jobs WHERE node_id=$1 AND integration_only
		AND state IN ('pending','claimed','accepted','unknown')`, c.NodeID).Scan(&integrationJobs); err != nil {
		return ValidationRun{}, err
	}
	if integrationJobs != 0 {
		return ValidationRun{}, fmt.Errorf("%w: integration job unresolved", ErrJobConflict)
	}
	runID, err := NewID()
	if err != nil {
		return ValidationRun{}, err
	}
	allocationID, err := NewID()
	if err != nil {
		return ValidationRun{}, err
	}
	jobID, err := NewID()
	if err != nil {
		return ValidationRun{}, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO validation_runs
		(id,started_by_admin_user_id,node_id,arcade_game_id,game_preset_id,content_version_id,content_sha256,
		template_revision_id,template_binding_key,template_fingerprint_sha256,template_binding_generation,
		maintenance_epoch,content_fact_revision,template_fact_revision)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`, runID, c.AdminID, c.NodeID, c.GameID, c.PresetID, c.ContentVersionID, contentSHA, c.TemplateRevisionID, key, expectedFingerprint, generation, epoch, contentRevision, templateRevision)
	if err != nil {
		return ValidationRun{}, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO allocations
		(id,purpose,validation_run_id,arcade_game_id,attempt_sequence,node_id,content_version_id,
		template_revision_id,state,assigned_at)
		VALUES($1,'validation',$2,$3,1,$4,$5,$6,'reserved',now())`, allocationID, runID, c.GameID, c.NodeID, c.ContentVersionID, c.TemplateRevisionID)
	if err != nil {
		return ValidationRun{}, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO node_jobs
		(id,node_id,kind,integration_only,allocation_id,template_binding_key,requested_port,required_capability)
		VALUES($1,$2,'create',false,$3,$4,0,'content_validation_v102')`, jobID, c.NodeID, allocationID, key)
	if err != nil {
		return ValidationRun{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return ValidationRun{}, err
	}
	return s.ValidationRun(ctx, runID)
}

func (s *Store) ValidationRun(ctx context.Context, runID string) (ValidationRun, error) {
	var r ValidationRun
	err := s.Pool.QueryRow(ctx, `SELECT v.id,v.node_id,v.arcade_game_id,v.game_preset_id,v.content_version_id,v.content_sha256,
		v.template_revision_id,v.template_binding_key,v.template_fingerprint_sha256,v.started_by_admin_user_id,
		v.template_binding_generation,v.maintenance_epoch,v.content_fact_revision,v.template_fact_revision,
		v.state,COALESCE(v.failure_code,''),COALESCE(v.result_code,''),v.human_result,v.human_confirmed_by,
		v.human_confirmed_at,v.ready_at,v.join_info_available_at,v.passed_at,v.created_at,
		COALESCE(a.id::text,''),COALESCE(c.id::text,''),COALESCE(st.id::text,'')
		FROM validation_runs v LEFT JOIN allocations a ON a.validation_run_id=v.id
		LEFT JOIN node_jobs c ON c.allocation_id=a.id AND c.kind='create'
		LEFT JOIN LATERAL (SELECT id FROM node_jobs WHERE allocation_id=a.id AND kind='stop' ORDER BY created_at DESC,id DESC LIMIT 1) st ON true
		WHERE v.id=$1`, runID).Scan(&r.ID, &r.NodeID, &r.GameID, &r.PresetID, &r.ContentVersionID, &r.ContentSHA256,
		&r.TemplateRevisionID, &r.TemplateBindingKey, &r.TemplateFingerprintSHA256, &r.StartedBy,
		&r.TemplateBindingGeneration, &r.MaintenanceEpoch, &r.ContentFactRevision, &r.TemplateFactRevision,
		&r.State, &r.FailureCode, &r.ResultCode, &r.HumanResult, &r.HumanConfirmedBy, &r.HumanConfirmedAt,
		&r.ReadyAt, &r.JoinInfoAvailableAt, &r.PassedAt, &r.CreatedAt, &r.AllocationID, &r.CreateJobID, &r.StopJobID)
	return r, err
}

func advanceValidationFromJob(ctx context.Context, tx pgx.Tx, runID, kind, jobState, allocationState, errorCode string, hasJoin bool) error {
	var current, human string
	if err := tx.QueryRow(ctx, `SELECT state,human_result FROM validation_runs WHERE id=$1 FOR UPDATE`, runID).Scan(&current, &human); err != nil {
		return err
	}
	if current == "passed" || current == "failed" {
		return nil
	}
	next := ""
	switch kind {
	case "create":
		switch jobState {
		case "accepted":
			if current == "create_pending" {
				next = "create_observing"
			}
		case "unknown":
			if current == "create_pending" || current == "create_observing" {
				next = "create_unknown"
			}
		case "succeeded":
			if hasJoin {
				next = "awaiting_human"
			} else {
				next = "ready_no_join"
			}
		case "rejected_no_effect":
			next = "failed"
		case "failed_with_effect":
			if allocationState == "quarantined" {
				next = "quarantined"
			} else {
				next = "cleanup_required"
			}
		}
	case "stop":
		switch jobState {
		case "accepted":
			next = "reclaim_observing"
		case "unknown":
			next = "stop_unknown"
		case "succeeded":
			if human == "fail" {
				next = "failed"
			} else {
				next = "reclaim_observing"
			}
		case "failed_with_effect":
			next = "quarantined"
		}
	}
	if next == "" || next == current {
		return nil
	}
	_, err := tx.Exec(ctx, `UPDATE validation_runs SET state=$2,
		failure_code=CASE WHEN $2='failed' AND human_result='fail' THEN 'HUMAN_FAILED'
		 WHEN $2 IN ('failed','quarantined','cleanup_required') THEN COALESCE(NULLIF($3,''),failure_code) ELSE failure_code END,
		result_code=CASE WHEN $2='failed' THEN 'FAIL' ELSE result_code END,
		ready_at=CASE WHEN $2 IN ('ready_no_join','awaiting_human') THEN COALESCE(ready_at,now()) ELSE ready_at END,
		join_info_available_at=CASE WHEN $2='awaiting_human' THEN COALESCE(join_info_available_at,now()) ELSE join_info_available_at END,
		updated_at=now() WHERE id=$1`, runID, next, errorCode)
	return err
}

// FinalizeValidationFailure closes a failed attempt only after its capacity
// reservation reached a proved resource terminal state and every Job settled.
func (s *Store) FinalizeValidationFailure(ctx context.Context, runID, reason string) (ValidationRun, error) {
	if reason == "" || len(reason) > 128 {
		return ValidationRun{}, ErrJobConflict
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return ValidationRun{}, err
	}
	defer tx.Rollback(ctx)
	var allocationID, nodeID string
	if err := tx.QueryRow(ctx, `SELECT id,node_id FROM allocations WHERE validation_run_id=$1`, runID).Scan(&allocationID, &nodeID); err != nil {
		return ValidationRun{}, err
	}
	if _, _, err := lockBusinessAllocation(ctx, tx, allocationID, nodeID); err != nil {
		return ValidationRun{}, err
	}
	var runState, allocationState string
	if err := tx.QueryRow(ctx, `SELECT v.state,a.state FROM validation_runs v JOIN allocations a ON a.validation_run_id=v.id
		WHERE v.id=$1`, runID).Scan(&runState, &allocationState); err != nil {
		return ValidationRun{}, err
	}
	if runState == "passed" {
		return ValidationRun{}, ErrJobConflict
	}
	if runState == "failed" {
		if err := tx.Commit(ctx); err != nil {
			return ValidationRun{}, err
		}
		return s.ValidationRun(ctx, runID)
	}
	if allocationState != "reclaimed" && allocationState != "released_no_effect" {
		return ValidationRun{}, ErrJobConflict
	}
	var unresolved int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM node_jobs WHERE allocation_id=$1
		AND state IN ('pending','claimed','accepted','unknown')`, allocationID).Scan(&unresolved); err != nil {
		return ValidationRun{}, err
	}
	if unresolved != 0 {
		return ValidationRun{}, ErrJobConflict
	}
	if _, err := tx.Exec(ctx, `UPDATE validation_runs SET state='failed',result_code='FAIL',failure_code=$2,updated_at=now()
		WHERE id=$1`, runID, reason); err != nil {
		return ValidationRun{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return ValidationRun{}, err
	}
	return s.ValidationRun(ctx, runID)
}

func advanceValidationFromFact(ctx context.Context, tx pgx.Tx, runID, outcome string, hasJoin bool) error {
	var state string
	if err := tx.QueryRow(ctx, `SELECT state FROM validation_runs WHERE id=$1 FOR UPDATE`, runID).Scan(&state); err != nil {
		return err
	}
	if state == "passed" || state == "failed" {
		return nil
	}
	next := ""
	switch outcome {
	case "ready":
		if state == "create_pending" || state == "create_observing" || state == "create_unknown" || state == "ready_no_join" {
			if hasJoin {
				next = "awaiting_human"
			} else {
				next = "ready_no_join"
			}
		}
	case "reclaimed":
		// ReportInstanceFact has already persisted the Allocation's reclaim proof.
		// Only a formal stop Job report advances the Run into reclaim observation;
		// a resource fact cannot bypass human confirmation or leave quarantine.
	case "quarantined":
		next = "quarantined"
	}
	if next == "" || next == state {
		return nil
	}
	_, err := tx.Exec(ctx, `UPDATE validation_runs SET state=$2,
		ready_at=CASE WHEN $2 IN ('ready_no_join','awaiting_human') THEN COALESCE(ready_at,now()) ELSE ready_at END,
		join_info_available_at=CASE WHEN $2='awaiting_human' THEN COALESCE(join_info_available_at,now()) ELSE join_info_available_at END,
		updated_at=now() WHERE id=$1`, runID, next)
	return err
}

// ConfirmValidationHuman stores an audited claim and queues a formal stop.
// It cannot turn a human pass into final PASS before full reclaim.
func (s *Store) ConfirmValidationHuman(ctx context.Context, runID, adminID, result string) (ValidationRun, error) {
	if result != "pass" && result != "fail" {
		return ValidationRun{}, ErrJobConflict
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return ValidationRun{}, err
	}
	defer tx.Rollback(ctx)
	var allocationID, nodeID string
	if err := tx.QueryRow(ctx, `SELECT a.id,a.node_id FROM allocations a WHERE a.validation_run_id=$1`, runID).Scan(&allocationID, &nodeID); err != nil {
		return ValidationRun{}, err
	}
	if _, _, err := lockBusinessAllocation(ctx, tx, allocationID, nodeID); err != nil {
		return ValidationRun{}, err
	}
	var runState, human, allocationState, instanceID, createState string
	var readyAt, joinAt *time.Time
	err = tx.QueryRow(ctx, `SELECT v.state,v.human_result,a.state,j.state,COALESCE(j.instance_id,''),v.ready_at,v.join_info_available_at
		FROM validation_runs v JOIN allocations a ON a.validation_run_id=v.id
		JOIN node_jobs j ON j.allocation_id=a.id AND j.kind='create' WHERE v.id=$1`, runID).
		Scan(&runState, &human, &allocationState, &createState, &instanceID, &readyAt, &joinAt)
	if err != nil {
		return ValidationRun{}, err
	}
	if human != "pending" || instanceID == "" || createState != "succeeded" || allocationState != "running" ||
		(result == "pass" && (runState != "awaiting_human" || readyAt == nil || joinAt == nil)) {
		return ValidationRun{}, ErrJobConflict
	}
	stopID, err := NewID()
	if err != nil {
		return ValidationRun{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE validation_runs SET human_result=$2,human_confirmed_by=$3,
		human_confirmed_at=now(),state='stop_pending',updated_at=now() WHERE id=$1`, runID, result, adminID); err != nil {
		return ValidationRun{}, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO node_jobs
		(id,node_id,kind,integration_only,allocation_id,instance_id,required_capability)
		VALUES($1,$2,'stop',false,$3,$4,'content_validation_v102')`, stopID, nodeID, allocationID, instanceID); err != nil {
		return ValidationRun{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE allocations SET state='stopping' WHERE id=$1`, allocationID); err != nil {
		return ValidationRun{}, err
	}
	if err := auditAdmin(ctx, tx, adminID, "validation.confirm", "validation_run", runID, map[string]any{"humanResult": result, "stopJobId": stopID}); err != nil {
		return ValidationRun{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return ValidationRun{}, err
	}
	return s.ValidationRun(ctx, runID)
}

// FinalizeValidationRun is deliberately separate from the human claim. It
// requires current facts and the full resource terminal proof in one transaction.
func (s *Store) FinalizeValidationRun(ctx context.Context, runID string) (ValidationRun, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return ValidationRun{}, err
	}
	defer tx.Rollback(ctx)
	var allocationID, nodeID, gameID string
	if err := tx.QueryRow(ctx, `SELECT a.id,a.node_id,a.arcade_game_id FROM allocations a WHERE a.validation_run_id=$1`, runID).
		Scan(&allocationID, &nodeID, &gameID); err != nil {
		return ValidationRun{}, err
	}
	var lockedNode string
	if err := tx.QueryRow(ctx, `SELECT id FROM nodes WHERE id=$1 FOR UPDATE`, nodeID).Scan(&lockedNode); err != nil {
		return ValidationRun{}, err
	}
	if _, _, err := lockBusinessAllocation(ctx, tx, allocationID, nodeID); err != nil {
		return ValidationRun{}, err
	}
	var state, human, allocationState, createState, stopState, instanceID, stopInstanceID string
	var readyAt, joinAt *time.Time
	var epoch, contentRevision, templateRevision, bindingGeneration int64
	var contentState, version, sha, templateState, key, algorithm, fingerprint, expectedFingerprint string
	var bindingClosed bool
	err = tx.QueryRow(ctx, `SELECT v.state,v.human_result,a.state,c.state,COALESCE(c.instance_id,''),
		COALESCE(st.state,''),COALESCE(st.instance_id,''),v.ready_at,v.join_info_available_at,b.maintenance_epoch,b.content_fact_revision,
		COALESCE(tf.template_fact_revision,0),tb.binding_generation,b.reported_state,
		COALESCE(b.reported_content_version_id,''),COALESCE(b.reported_content_sha256,''),
		COALESCE(tf.reported_state,'unknown'),COALESCE(tf.binding_key,''),COALESCE(tf.manifest_algorithm,''),
		COALESCE(tf.reported_fingerprint_sha256,''),COALESCE(tb.expected_template_fingerprint_sha256,''),
		NOT b.accepting_new_allocations
		FROM validation_runs v JOIN allocations a ON a.validation_run_id=v.id
		JOIN node_jobs c ON c.allocation_id=a.id AND c.kind='create'
		JOIN node_content_bindings b ON b.node_id=v.node_id AND b.arcade_game_id=v.arcade_game_id
		JOIN node_template_bindings tb ON tb.node_id=v.node_id AND tb.template_revision_id=v.template_revision_id
		LEFT JOIN node_template_facts tf ON tf.node_id=v.node_id AND tf.template_revision_id=v.template_revision_id
		LEFT JOIN LATERAL (SELECT state,instance_id FROM node_jobs WHERE allocation_id=a.id AND kind='stop' ORDER BY created_at DESC,id DESC LIMIT 1) st ON true
		WHERE v.id=$1`, runID).Scan(&state, &human, &allocationState, &createState, &instanceID, &stopState, &stopInstanceID, &readyAt, &joinAt,
		&epoch, &contentRevision, &templateRevision, &bindingGeneration, &contentState, &version, &sha, &templateState, &key, &algorithm, &fingerprint, &expectedFingerprint, &bindingClosed)
	if err != nil {
		return ValidationRun{}, err
	}
	var candidateEpoch, candidateContentRevision, candidateTemplateRevision, candidateGeneration int64
	var candidateVersion, candidateSHA, candidateKey, candidateFingerprint string
	if err := tx.QueryRow(ctx, `SELECT maintenance_epoch,content_fact_revision,template_fact_revision,
		template_binding_generation,content_version_id,content_sha256,template_binding_key,template_fingerprint_sha256
		FROM validation_runs WHERE id=$1`, runID).Scan(&candidateEpoch, &candidateContentRevision, &candidateTemplateRevision, &candidateGeneration,
		&candidateVersion, &candidateSHA, &candidateKey, &candidateFingerprint); err != nil {
		return ValidationRun{}, err
	}
	if state == "passed" {
		return s.ValidationRun(ctx, runID)
	}
	if human != "pass" || state != "reclaim_observing" || allocationState != "reclaimed" ||
		createState != "succeeded" || stopState != "succeeded" || instanceID == "" || stopInstanceID != instanceID || readyAt == nil || joinAt == nil ||
		!bindingClosed || epoch != candidateEpoch || contentRevision != candidateContentRevision ||
		templateRevision != candidateTemplateRevision || bindingGeneration != candidateGeneration ||
		contentState != "confirmed" || version != candidateVersion || sha != candidateSHA ||
		templateState != "confirmed" || key != candidateKey || algorithm != "template-manifest-sha256-v1" ||
		fingerprint != candidateFingerprint || expectedFingerprint != candidateFingerprint {
		return ValidationRun{}, ErrJobConflict
	}
	var unresolved, otherResources int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM node_jobs j JOIN allocations a ON a.id=j.allocation_id
		WHERE a.validation_run_id=$1 AND j.state IN ('pending','claimed','accepted','unknown')`, runID).Scan(&unresolved); err != nil {
		return ValidationRun{}, err
	}
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM allocations WHERE node_id=$1 AND arcade_game_id=$2 AND id<>$3
		AND state NOT IN ('reclaimed','released_no_effect')`, nodeID, gameID, allocationID).Scan(&otherResources); err != nil {
		return ValidationRun{}, err
	}
	if unresolved != 0 || otherResources != 0 {
		return ValidationRun{}, ErrJobConflict
	}
	var enabled bool
	var heartbeat, contentReceived, templateReceived *time.Time
	var compatibility string
	if err := tx.QueryRow(ctx, `SELECT n.enabled,n.last_heartbeat,COALESCE(r.compatibility_status,''),b.reported_at,tf.received_at
		FROM nodes n LEFT JOIN node_reports r ON r.node_id=n.id
		JOIN node_content_bindings b ON b.node_id=n.id AND b.arcade_game_id=$2
		JOIN validation_runs v ON v.id=$3
		JOIN node_template_facts tf ON tf.node_id=n.id AND tf.template_revision_id=v.template_revision_id
		WHERE n.id=$1`, nodeID, gameID, runID).Scan(&enabled, &heartbeat, &compatibility, &contentReceived, &templateReceived); err != nil {
		return ValidationRun{}, err
	}
	if !enabled || compatibility != "compatible" || NodeConnectivity(heartbeat, time.Now().UTC()) != "online" ||
		contentReceived == nil || time.Since(*contentReceived) >= nodeOnlineWindow ||
		templateReceived == nil || time.Since(*templateReceived) >= nodeOnlineWindow {
		return ValidationRun{}, ErrJobConflict
	}
	var inventoryComplete bool
	var inventoryState string
	var inventoryReceived time.Time
	var unaccounted int
	if err := tx.QueryRow(ctx, `SELECT complete,state,received_at,unaccounted_count FROM node_inventory_snapshots
		WHERE node_id=$1 ORDER BY received_at DESC,scan_id DESC LIMIT 1`, nodeID).
		Scan(&inventoryComplete, &inventoryState, &inventoryReceived, &unaccounted); err != nil {
		return ValidationRun{}, err
	}
	if !inventoryComplete || inventoryState != "confirmed" || time.Since(inventoryReceived) >= nodeOnlineWindow || unaccounted != 0 {
		return ValidationRun{}, ErrJobConflict
	}
	if _, err := tx.Exec(ctx, `UPDATE validation_runs SET state='passed',result_code='PASS',passed_at=now(),updated_at=now() WHERE id=$1`, runID); err != nil {
		return ValidationRun{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return ValidationRun{}, err
	}
	return s.ValidationRun(ctx, runID)
}
