package store

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
)

// TryAllocateOne locks eligible nodes in stable order and reserves one slot
// before committing the Allocation and its create job. Every scheduler path
// must use this same transactional reservation boundary.
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
		ORDER BY n.id FOR UPDATE OF n`, revisionID, gameID, contentID)
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
	var selected *node
	for i := range nodes {
		n := &nodes[i]
		// The candidate query may have waited for a concurrent heartbeat's
		// node lock. Re-read all Controller-reported eligibility and hard
		// capacity after the lock is ours, using a fresh READ COMMITTED
		// statement, before counting or reserving a slot.
		err := tx.QueryRow(ctx, `SELECT b.binding_key,n.desired_max_instances,r.hard_max_instances
			FROM nodes n JOIN node_reports r ON r.node_id=n.id
			JOIN node_template_bindings b ON b.node_id=n.id AND b.template_revision_id=$2
			JOIN node_content_bindings c ON c.node_id=n.id AND c.arcade_game_id=$3
			WHERE n.id=$1 AND n.enabled AND n.accepting_new_requests AND NOT n.draining
			AND n.last_heartbeat > now() - interval '2 minutes'
			AND r.compatibility_status='compatible'
			AND c.reported_state='confirmed' AND c.reported_content_version_id=$4
			AND c.accepting_new_allocations`, n.id, revisionID, gameID, contentID).
			Scan(&n.key, &n.desired, &n.hard)
		if errors.Is(err, pgx.ErrNoRows) {
			continue
		}
		if err != nil {
			return false, err
		}
		limit := min(n.hard, n.desired)
		if limit <= 0 {
			continue
		}
		var occupied int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM allocations WHERE node_id=$1
			AND state NOT IN ('reclaimed','released_no_effect')`, n.id).Scan(&occupied); err != nil {
			return false, err
		}
		if occupied < limit {
			selected = n
			break
		}
	}
	if selected == nil {
		return false, nil
	}
	n := selected
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
	ID                  string          `json:"id"`
	ServerRequestID     string          `json:"serverRequestId"`
	AttemptSequence     int             `json:"attemptSequence"`
	NodeID              string          `json:"nodeId"`
	NodeDisplayName     string          `json:"nodeDisplayName"`
	ContentVersionID    string          `json:"contentVersionId"`
	State               string          `json:"state"`
	AssignedAt          time.Time       `json:"assignedAt"`
	CreateStartedAt     *time.Time      `json:"createStartedAt,omitempty"`
	ReadyAt             *time.Time      `json:"readyAt,omitempty"`
	JoinInfoAvailableAt *time.Time      `json:"joinInfoAvailableAt,omitempty"`
	ReclaimedAt         *time.Time      `json:"reclaimedAt,omitempty"`
	ErrorCode           string          `json:"errorCode,omitempty"`
	JoinInfo            *PlayerJoinInfo `json:"joinInfo,omitempty"`
	JoinInfoErrorCode   string          `json:"joinInfoErrorCode,omitempty"`
}

type PlayerJoinInfo struct {
	ConnectCommand string `json:"connectCommand"`
	ConnectHost    string `json:"connectHost"`
	PublicPort     int    `json:"publicPort"`
	SteamURI       string `json:"steamUri,omitempty"`
	SteamChinaURI  string `json:"steamChinaUri,omitempty"`
}

func (s *Store) UserRequestAllocation(ctx context.Context, userID, requestID string) (*Allocation, error) {
	var a Allocation
	var localPort, publicPort *int
	var connectHost, protocolIP, joinRevision, currentRevision string
	var steamVerified, steamEnabled, chinaVerified, chinaEnabled bool
	err := s.Pool.QueryRow(ctx, `SELECT a.id,a.server_request_id,a.attempt_sequence,a.node_id,n.display_name,
		a.content_version_id,a.state,a.assigned_at,a.create_started_at,a.ready_at,a.join_info_available_at,
		a.reclaimed_at,COALESCE(a.error_code,''),a.join_local_port,a.join_public_port,
		COALESCE(a.join_connect_host,''),COALESCE(a.join_protocol_ip,''),COALESCE(a.join_entry_config_revision,''),
		COALESCE(a.join_info_error_code,''),COALESCE(c.entry_config_revision,''),
		COALESCE(c.steam_entry_verified,false),COALESCE(c.steam_entry_enabled,false),
		COALESCE(c.steamchina_entry_verified,false),COALESCE(c.steamchina_entry_enabled,false)
		FROM allocations a JOIN server_requests r ON r.id=a.server_request_id JOIN nodes n ON n.id=a.node_id
		LEFT JOIN node_entry_capabilities c ON c.node_id=a.node_id
		WHERE r.id=$2 AND ((r.owner_user_id=$1 AND NOT EXISTS (SELECT 1 FROM party_members WHERE user_id=$1))
		OR EXISTS (SELECT 1 FROM party_members m WHERE m.user_id=$1 AND m.party_id=r.owner_party_id))
		ORDER BY a.attempt_sequence DESC LIMIT 1`, userID, requestID).
		Scan(&a.ID, &a.ServerRequestID, &a.AttemptSequence, &a.NodeID, &a.NodeDisplayName, &a.ContentVersionID,
			&a.State, &a.AssignedAt, &a.CreateStartedAt, &a.ReadyAt, &a.JoinInfoAvailableAt, &a.ReclaimedAt, &a.ErrorCode,
			&localPort, &publicPort, &connectHost, &protocolIP, &joinRevision, &a.JoinInfoErrorCode, &currentRevision,
			&steamVerified, &steamEnabled, &chinaVerified, &chinaEnabled)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if a.State == "running" && a.JoinInfoAvailableAt != nil && a.ReadyAt != nil && localPort != nil && publicPort != nil &&
		connectHost != "" && joinRevision == currentRevision {
		a.JoinInfo = &PlayerJoinInfo{ConnectCommand: "connect " + net.JoinHostPort(connectHost, strconv.Itoa(*publicPort)),
			ConnectHost: connectHost, PublicPort: *publicPort}
		if protocolIP != "" && steamVerified && steamEnabled {
			a.JoinInfo.SteamURI = "steam://connect/" + net.JoinHostPort(protocolIP, strconv.Itoa(*publicPort))
		}
		if protocolIP != "" && chinaVerified && chinaEnabled {
			a.JoinInfo.SteamChinaURI = "steamchina://connect/" + net.JoinHostPort(protocolIP, strconv.Itoa(*publicPort))
		}
	} else if a.State == "running" && a.JoinInfoAvailableAt != nil && joinRevision != currentRevision {
		a.JoinInfoErrorCode = "NETWORK_CONFIG_CHANGED"
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
	r, err := scanServerRequest(tx.QueryRow(ctx, `SELECT id,arcade_game_id,game_preset_id,state,requested_at,updated_at
		FROM server_requests WHERE id=$1 AND `+ownerField+`=$2 FOR UPDATE`, requestID, ownerID))
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
