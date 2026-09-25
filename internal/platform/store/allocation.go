package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// TryAllocateOne performs the P1 single-node dispatch. The request row and
// selected node row remain locked through the capacity count and reservation.
// A future multi-node scheduler can reuse this reservation boundary.
func (s *Store) TryAllocateOne(ctx context.Context) (bool, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)
	var requestID string
	err = tx.QueryRow(ctx, `SELECT id FROM server_requests WHERE state='waiting'
		ORDER BY requested_at,id FOR UPDATE SKIP LOCKED LIMIT 1`).Scan(&requestID)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var gameID, contentID, revisionID string
	var globalAccept, gameEnabled, gameAccept, presetEnabled, presetAccept bool
	err = tx.QueryRow(ctx, `SELECT r.arcade_game_id,COALESCE(g.current_content_version_id,''),p.template_revision_id,
		s.accepting_new_requests,g.enabled,g.accepting_new_requests,p.enabled,p.accepting_new_requests
		FROM server_requests r JOIN arcade_games g ON g.id=r.arcade_game_id
		JOIN game_presets p ON p.id=r.game_preset_id
		JOIN platform_settings s ON s.singleton=true WHERE r.id=$1 FOR SHARE OF s,g,p`, requestID).
		Scan(&gameID, &contentID, &revisionID, &globalAccept, &gameEnabled, &gameAccept, &presetEnabled, &presetAccept)
	if err != nil {
		return false, err
	}
	if !gameEnabled || !presetEnabled {
		at := time.Now().UTC()
		_, err = tx.Exec(ctx, `UPDATE server_requests SET state='unavailable',updated_at=$2 WHERE id=$1`, requestID, at)
		if err != nil {
			return false, err
		}
		return true, tx.Commit(ctx)
	}
	if !globalAccept || !gameAccept || !presetAccept || contentID == "" {
		return false, nil
	}
	rows, err := tx.Query(ctx, `SELECT n.id,b.binding_key,n.desired_max_instances,r.hard_max_instances
		FROM nodes n JOIN node_reports r ON r.node_id=n.id
		JOIN node_template_bindings b ON b.node_id=n.id AND b.template_revision_id=$1
		JOIN node_content_bindings c ON c.node_id=n.id AND c.arcade_game_id=$2
		WHERE n.enabled AND n.accepting_new_requests AND NOT n.draining
		AND n.last_heartbeat > now() - interval '2 minutes'
		AND r.compatibility_status='compatible'
		AND c.reported_state='confirmed' AND c.reported_content_version_id=$3
		AND c.accepting_new_allocations
		ORDER BY n.id FOR UPDATE OF n LIMIT 2`, revisionID, gameID, contentID)
	if err != nil {
		return false, err
	}
	type node struct {
		id, key       string
		desired, hard int
	}
	nodes := []node{}
	for rows.Next() {
		var n node
		if err := rows.Scan(&n.id, &n.key, &n.desired, &n.hard); err != nil {
			rows.Close()
			return false, err
		}
		nodes = append(nodes, n)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return false, err
	}
	rows.Close()
	if len(nodes) == 0 {
		return false, nil
	}
	if len(nodes) != 1 {
		return false, fmt.Errorf("P1 requires exactly one eligible node")
	}
	n := nodes[0]
	limit := min(n.hard, n.desired)
	var occupied int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM allocations WHERE node_id=$1
		AND state NOT IN ('reclaimed','released_no_effect')`, n.id).Scan(&occupied); err != nil {
		return false, err
	}
	if occupied >= limit {
		return false, nil
	}
	var sequence int
	if err := tx.QueryRow(ctx, `SELECT COALESCE(MAX(attempt_sequence),0)+1 FROM allocations
		WHERE server_request_id=$1`, requestID).Scan(&sequence); err != nil {
		return false, err
	}
	allocationID, err := NewID()
	if err != nil {
		return false, err
	}
	jobID, err := NewID()
	if err != nil {
		return false, err
	}
	at := time.Now().UTC()
	_, err = tx.Exec(ctx, `INSERT INTO allocations(id,server_request_id,arcade_game_id,attempt_sequence,node_id,
		content_version_id,template_revision_id,state,assigned_at)
		VALUES($1,$2,$3,$4,$5,$6,$7,'reserved',$8)`, allocationID, requestID, gameID, sequence, n.id, contentID, revisionID, at)
	if err != nil {
		return false, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO node_jobs(id,node_id,kind,integration_only,allocation_id,template_binding_key,requested_port)
		VALUES($1,$2,'create',false,$3,$4,0)`, jobID, n.id, allocationID, n.key)
	if err != nil {
		return false, err
	}
	_, err = tx.Exec(ctx, `UPDATE server_requests SET state='allocating',updated_at=$2 WHERE id=$1`, requestID, at)
	if err != nil {
		return false, err
	}
	return true, tx.Commit(ctx)
}

type Allocation struct {
	ID                  string     `json:"id"`
	ServerRequestID     string     `json:"serverRequestId"`
	AttemptSequence     int        `json:"attemptSequence"`
	NodeID              string     `json:"nodeId"`
	NodeDisplayName     string     `json:"nodeDisplayName"`
	ContentVersionID    string     `json:"contentVersionId"`
	State               string     `json:"state"`
	AssignedAt          time.Time  `json:"assignedAt"`
	CreateStartedAt     *time.Time `json:"createStartedAt,omitempty"`
	ReadyAt             *time.Time `json:"readyAt,omitempty"`
	JoinInfoAvailableAt *time.Time `json:"joinInfoAvailableAt,omitempty"`
	ReclaimedAt         *time.Time `json:"reclaimedAt,omitempty"`
	ErrorCode           string     `json:"errorCode,omitempty"`
}

func (s *Store) UserRequestAllocation(ctx context.Context, userID, requestID string) (*Allocation, error) {
	var a Allocation
	err := s.Pool.QueryRow(ctx, `SELECT a.id,a.server_request_id,a.attempt_sequence,a.node_id,n.display_name,
		a.content_version_id,a.state,a.assigned_at,a.create_started_at,a.ready_at,a.join_info_available_at,
		a.reclaimed_at,COALESCE(a.error_code,'')
		FROM allocations a JOIN server_requests r ON r.id=a.server_request_id JOIN nodes n ON n.id=a.node_id
		WHERE r.owner_user_id=$1 AND r.id=$2 ORDER BY a.attempt_sequence DESC LIMIT 1`, userID, requestID).
		Scan(&a.ID, &a.ServerRequestID, &a.AttemptSequence, &a.NodeID, &a.NodeDisplayName, &a.ContentVersionID,
			&a.State, &a.AssignedAt, &a.CreateStartedAt, &a.ReadyAt, &a.JoinInfoAvailableAt, &a.ReclaimedAt, &a.ErrorCode)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &a, nil
}

// StopUserRequest creates a durable stop job only for the current owner's
// running allocation. The instance ID is read from the create job, never from
// the browser. Capacity remains occupied until the stop report proves reclaim.
func (s *Store) StopUserRequest(ctx context.Context, userID, requestID string) (ServerRequest, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return ServerRequest{}, err
	}
	defer tx.Rollback(ctx)
	r, err := scanServerRequest(tx.QueryRow(ctx, `SELECT id,arcade_game_id,game_preset_id,state,requested_at,updated_at
		FROM server_requests WHERE id=$1 AND owner_user_id=$2 FOR UPDATE`, requestID, userID))
	if err != nil {
		return ServerRequest{}, err
	}
	if r.State == "stopping" || r.State == "ended" {
		return r, tx.Commit(ctx)
	}
	if r.State != "running" {
		return ServerRequest{}, fmt.Errorf("%w: request is not running", ErrJobConflict)
	}
	var allocationID, nodeID, instanceID string
	err = tx.QueryRow(ctx, `SELECT a.id,a.node_id,j.instance_id
		FROM allocations a JOIN node_jobs j ON j.allocation_id=a.id AND j.kind='create' AND j.state='succeeded'
		WHERE a.server_request_id=$1 AND a.state='running' ORDER BY a.attempt_sequence DESC LIMIT 1 FOR UPDATE OF a`, requestID).
		Scan(&allocationID, &nodeID, &instanceID)
	if err != nil {
		return ServerRequest{}, err
	}
	if instanceID == "" {
		return ServerRequest{}, fmt.Errorf("%w: missing instance identity", ErrJobConflict)
	}
	jobID, err := NewID()
	if err != nil {
		return ServerRequest{}, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO node_jobs(id,node_id,kind,integration_only,allocation_id,instance_id)
		VALUES($1,$2,'stop',false,$3,$4)`, jobID, nodeID, allocationID, instanceID)
	if err != nil {
		return ServerRequest{}, err
	}
	at := time.Now().UTC()
	_, err = tx.Exec(ctx, `UPDATE allocations SET state='stopping' WHERE id=$1`, allocationID)
	if err != nil {
		return ServerRequest{}, err
	}
	r.State, r.UpdatedAt = "stopping", at
	_, err = tx.Exec(ctx, `UPDATE server_requests SET state='stopping',updated_at=$2 WHERE id=$1`, requestID, at)
	if err != nil {
		return ServerRequest{}, err
	}
	return r, tx.Commit(ctx)
}
