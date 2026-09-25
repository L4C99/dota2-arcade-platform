package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

var ErrInvalidAdminAction = errors.New("invalid administrator action")

type AdminAction struct {
	Action    string  `json:"action"`
	TargetID  string  `json:"targetId"`
	GameID    string  `json:"gameId,omitempty"`
	Accepting *bool   `json:"accepting,omitempty"`
	Enabled   *bool   `json:"enabled,omitempty"`
	Draining  *bool   `json:"draining,omitempty"`
	Priority  *int    `json:"priority,omitempty"`
	Desired   *int    `json:"desired,omitempty"`
	Message   *string `json:"message,omitempty"`
	Entry     string  `json:"entry,omitempty"`
	Verified  *bool   `json:"verified,omitempty"`
	Confirmed bool    `json:"confirmed,omitempty"`
}

func auditAdmin(ctx context.Context, tx pgx.Tx, adminID, action, targetType, targetID string, change map[string]any) error {
	id, err := NewID()
	if err != nil {
		return err
	}
	encoded, err := json.Marshal(change)
	if err != nil {
		return err
	}
	command, err := tx.Exec(ctx, `INSERT INTO audit_events(id,actor_admin_user_id,actor_kind,action,target_type,target_id,result,state_change)
		SELECT $1,id,'admin',$3,$4,$5,'succeeded',$6 FROM admin_users WHERE id=$2 AND enabled`,
		id, adminID, action, targetType, targetID, encoded)
	if err != nil {
		return err
	}
	if command.RowsAffected() != 1 {
		return ErrInvalidCredentials
	}
	return nil
}

func auditOperator(ctx context.Context, tx pgx.Tx, kind, action, targetType, targetID string, change map[string]any) error {
	id, err := NewID()
	if err != nil {
		return err
	}
	encoded, err := json.Marshal(change)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO audit_events(id,actor_kind,action,target_type,target_id,result,state_change)
		VALUES($1,$2,$3,$4,$5,'succeeded',$6)`, id, kind, action, targetType, targetID, encoded)
	return err
}

func (s *Store) ApplyAdminAction(ctx context.Context, adminID string, a AdminAction) error {
	if len(a.TargetID) > 200 || len(a.GameID) > 200 || a.Message != nil && len(*a.Message) > 1000 {
		return ErrInvalidAdminAction
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	change := map[string]any{}
	targetType := ""
	targetID := a.TargetID
	switch a.Action {
	case "global.update":
		targetType, targetID = "platform_settings", "global"
		if a.Accepting == nil && a.Message == nil {
			return ErrInvalidAdminAction
		}
		var oldAccept bool
		var oldMessage string
		if err := tx.QueryRow(ctx, `SELECT accepting_new_requests,maintenance_message FROM platform_settings WHERE singleton FOR UPDATE`).Scan(&oldAccept, &oldMessage); err != nil {
			return err
		}
		accept, message := oldAccept, oldMessage
		if a.Accepting != nil {
			accept = *a.Accepting
		}
		if a.Message != nil {
			message = strings.TrimSpace(*a.Message)
		}
		if _, err := tx.Exec(ctx, `UPDATE platform_settings SET accepting_new_requests=$1,maintenance_message=$2 WHERE singleton`, accept, message); err != nil {
			return err
		}
		change = map[string]any{"acceptingBefore": oldAccept, "acceptingAfter": accept, "messageBefore": oldMessage, "messageAfter": message}
	case "announcement.update":
		targetType, targetID = "site_announcement", "global"
		if a.Message == nil {
			return ErrInvalidAdminAction
		}
		var old string
		if err := tx.QueryRow(ctx, `SELECT message FROM site_announcements WHERE singleton FOR UPDATE`).Scan(&old); err != nil {
			return err
		}
		message := strings.TrimSpace(*a.Message)
		if _, err := tx.Exec(ctx, `UPDATE site_announcements SET message=$1,updated_at=now() WHERE singleton`, message); err != nil {
			return err
		}
		change = map[string]any{"before": old, "after": message}
	case "game.update", "preset.update":
		if a.TargetID == "" || a.Accepting == nil && a.Enabled == nil && a.Message == nil {
			return ErrInvalidAdminAction
		}
		table := "arcade_games"
		targetType = "arcade_game"
		if a.Action == "preset.update" {
			table = "game_presets"
			targetType = "game_preset"
		}
		var oldEnabled, oldAccept bool
		var oldMessage string
		if err := tx.QueryRow(ctx, `SELECT enabled,accepting_new_requests,maintenance_message FROM `+table+` WHERE id=$1 FOR UPDATE`, a.TargetID).Scan(&oldEnabled, &oldAccept, &oldMessage); err != nil {
			return err
		}
		enabled, accept, message := oldEnabled, oldAccept, oldMessage
		if a.Enabled != nil {
			enabled = *a.Enabled
		}
		if a.Accepting != nil {
			accept = *a.Accepting
		}
		if a.Message != nil {
			message = strings.TrimSpace(*a.Message)
		}
		if _, err := tx.Exec(ctx, `UPDATE `+table+` SET enabled=$2,accepting_new_requests=$3,maintenance_message=$4 WHERE id=$1`, a.TargetID, enabled, accept, message); err != nil {
			return err
		}
		change = map[string]any{"enabledBefore": oldEnabled, "enabledAfter": enabled, "acceptingBefore": oldAccept, "acceptingAfter": accept, "messageBefore": oldMessage, "messageAfter": message}
	case "node.update":
		targetType = "node"
		if a.TargetID == "" || a.Draining == nil && a.Priority == nil && a.Desired == nil {
			return ErrInvalidAdminAction
		}
		var oldDrain bool
		var oldPriority, oldDesired, hard int
		if err := tx.QueryRow(ctx, `SELECT n.draining,n.priority,n.desired_max_instances,COALESCE(r.hard_max_instances,0)
			FROM nodes n LEFT JOIN node_reports r ON r.node_id=n.id WHERE n.id=$1 FOR UPDATE OF n`, a.TargetID).Scan(&oldDrain, &oldPriority, &oldDesired, &hard); err != nil {
			return err
		}
		drain, priority, desired := oldDrain, oldPriority, oldDesired
		if a.Draining != nil {
			drain = *a.Draining
		}
		if a.Priority != nil {
			priority = *a.Priority
		}
		if a.Desired != nil {
			desired = *a.Desired
		}
		if desired < 0 || desired > hard {
			return ErrInvalidDesiredCapacity
		}
		if _, err := tx.Exec(ctx, `UPDATE nodes SET draining=$2,priority=$3,desired_max_instances=$4 WHERE id=$1`, a.TargetID, drain, priority, desired); err != nil {
			return err
		}
		change = map[string]any{"drainingBefore": oldDrain, "drainingAfter": drain, "priorityBefore": oldPriority, "priorityAfter": priority, "desiredBefore": oldDesired, "desiredAfter": desired}
	case "node.reconcile":
		targetType = "node"
		var exists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM nodes WHERE id=$1)`, a.TargetID).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return pgx.ErrNoRows
		}
		var generation int64
		if err := tx.QueryRow(ctx, `INSERT INTO node_reconcile_requests(node_id,requested_generation,requested_at)
			VALUES($1,1,now()) ON CONFLICT(node_id) DO UPDATE SET
			requested_generation=node_reconcile_requests.requested_generation+1,requested_at=now()
			RETURNING requested_generation`, a.TargetID).Scan(&generation); err != nil {
			return err
		}
		change = map[string]any{"requestedGeneration": generation}
	case "binding.update":
		targetType = "node_content_binding"
		targetID = a.TargetID + ":" + a.GameID
		if a.TargetID == "" || a.GameID == "" || a.Accepting == nil {
			return ErrInvalidAdminAction
		}
		var old bool
		if err := tx.QueryRow(ctx, `SELECT accepting_new_allocations FROM node_content_bindings WHERE node_id=$1 AND arcade_game_id=$2 FOR UPDATE`, a.TargetID, a.GameID).Scan(&old); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE node_content_bindings SET accepting_new_allocations=$3 WHERE node_id=$1 AND arcade_game_id=$2`, a.TargetID, a.GameID, *a.Accepting); err != nil {
			return err
		}
		change = map[string]any{"acceptingBefore": old, "acceptingAfter": *a.Accepting}
	case "entry.update":
		targetType = "node_entry"
		targetID = a.TargetID + ":" + a.Entry
		if a.TargetID == "" || (a.Entry != "steam" && a.Entry != "steamchina") || a.Verified == nil && a.Enabled == nil {
			return ErrInvalidAdminAction
		}
		if a.Verified != nil && *a.Verified && !a.Confirmed {
			return ErrInvalidAdminAction
		}
		verifiedCol, enabledCol, atCol, byCol := "steam_entry_verified", "steam_entry_enabled", "steam_verified_at", "steam_verified_by"
		if a.Entry == "steamchina" {
			verifiedCol, enabledCol, atCol, byCol = "steamchina_entry_verified", "steamchina_entry_enabled", "steamchina_verified_at", "steamchina_verified_by"
		}
		var oldVerified, oldEnabled bool
		var a2sEnabled, a2sOK bool
		var revision string
		if err := tx.QueryRow(ctx, `SELECT `+verifiedCol+`,`+enabledCol+`,a2s_enabled,a2s_query_ok,entry_config_revision FROM node_entry_capabilities WHERE node_id=$1 FOR UPDATE`, a.TargetID).Scan(&oldVerified, &oldEnabled, &a2sEnabled, &a2sOK, &revision); err != nil {
			return err
		}
		verified, enabled := oldVerified, oldEnabled
		if a.Verified != nil {
			verified = *a.Verified
			if !verified {
				enabled = false
			}
		}
		if a.Enabled != nil {
			enabled = *a.Enabled
		}
		if enabled && !verified {
			return ErrInvalidAdminAction
		}
		// Verification is an explicit human claim for the current Controller-reported revision.
		// It never updates network/A2S facts and is deliberately withheld when A2S evidence is absent.
		if a.Verified != nil && *a.Verified && (!a2sEnabled || !a2sOK) {
			return ErrInvalidAdminAction
		}
		query := fmt.Sprintf(`UPDATE node_entry_capabilities SET %s=$2,%s=$3,%s=CASE WHEN $2 AND NOT %s THEN now() WHEN $2 THEN %s ELSE NULL END,
			%s=CASE WHEN $2 AND NOT %s THEN $4::uuid WHEN $2 THEN %s ELSE NULL END WHERE node_id=$1`, verifiedCol, enabledCol, atCol, verifiedCol, atCol, byCol, verifiedCol, byCol)
		if _, err := tx.Exec(ctx, query, a.TargetID, verified, enabled, adminID); err != nil {
			return err
		}
		change = map[string]any{"verifiedBefore": oldVerified, "verifiedAfter": verified, "enabledBefore": oldEnabled, "enabledAfter": enabled, "revision": revision}
	case "request.cancel":
		targetType = "server_request"
		var state string
		if err := tx.QueryRow(ctx, `SELECT state FROM server_requests WHERE id=$1 FOR UPDATE`, a.TargetID).Scan(&state); err != nil {
			return err
		}
		var attempts int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM allocations WHERE server_request_id=$1`, a.TargetID).Scan(&attempts); err != nil {
			return err
		}
		if state != "waiting" || attempts != 0 {
			return fmt.Errorf("%w: request is not pure waiting", ErrJobConflict)
		}
		if _, err := tx.Exec(ctx, `UPDATE server_requests SET state='cancelled',updated_at=now() WHERE id=$1`, a.TargetID); err != nil {
			return err
		}
		change = map[string]any{"before": state, "after": "cancelled"}
	case "request.stop":
		targetType = "server_request"
		var state string
		if err := tx.QueryRow(ctx, `SELECT state FROM server_requests WHERE id=$1 FOR UPDATE`, a.TargetID).Scan(&state); err != nil {
			return err
		}
		if state != "running" && state != "quarantined" {
			return fmt.Errorf("%w: request cannot be stopped", ErrJobConflict)
		}
		var allocationID, nodeID, instanceID, createError string
		err := tx.QueryRow(ctx, `SELECT a.id,a.node_id,j.instance_id,COALESCE(j.error_code,'') FROM allocations a
			JOIN node_jobs j ON j.allocation_id=a.id AND j.kind='create' AND j.instance_id IS NOT NULL
			WHERE a.server_request_id=$1 AND a.state IN ('running','quarantined')
			ORDER BY a.attempt_sequence DESC LIMIT 1 FOR UPDATE OF a`, a.TargetID).Scan(&allocationID, &nodeID, &instanceID, &createError)
		if err != nil {
			return err
		}
		if instanceID == "" || createError == "IDENTITY_UNVERIFIED" {
			return fmt.Errorf("%w: untrusted instance identity", ErrJobConflict)
		}
		var open bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM node_jobs WHERE allocation_id=$1 AND kind='stop' AND state IN ('pending','claimed','accepted','unknown'))`, allocationID).Scan(&open); err != nil {
			return err
		}
		if open {
			return fmt.Errorf("%w: stop already in progress", ErrJobConflict)
		}
		jobID, err := NewID()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO node_jobs(id,node_id,kind,integration_only,allocation_id,instance_id)
			VALUES($1,$2,'stop',false,$3,$4)`, jobID, nodeID, allocationID, instanceID); err != nil {
			return err
		}
		if state == "running" {
			if _, err := tx.Exec(ctx, `UPDATE allocations SET state='stopping' WHERE id=$1`, allocationID); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `UPDATE server_requests SET state='stopping',updated_at=now() WHERE id=$1`, a.TargetID); err != nil {
				return err
			}
		}
		change = map[string]any{"requestBefore": state, "stopJobId": jobID, "allocationId": allocationID, "requestAfter": map[bool]string{true: "stopping", false: "quarantined"}[state == "running"]}
	case "allocation.quarantine":
		targetType = "allocation"
		var requestID, state string
		if err := tx.QueryRow(ctx, `SELECT server_request_id,state FROM allocations WHERE id=$1 FOR UPDATE`, a.TargetID).Scan(&requestID, &state); err != nil {
			return err
		}
		if state == "reclaimed" || state == "released_no_effect" {
			return fmt.Errorf("%w: resource already terminal", ErrJobConflict)
		}
		if state == "reserved" {
			var effectPossible bool
			if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM node_jobs WHERE allocation_id=$1 AND kind='create' AND state<>'pending')`, a.TargetID).Scan(&effectPossible); err != nil {
				return err
			}
			if !effectPossible {
				return fmt.Errorf("%w: pure reservation cannot be quarantined", ErrJobConflict)
			}
		}
		if state != "quarantined" {
			if _, err := tx.Exec(ctx, `UPDATE allocations SET state='quarantined',error_code='MANUAL_QUARANTINE',quarantined_at=COALESCE(quarantined_at,now()) WHERE id=$1`, a.TargetID); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `UPDATE server_requests SET state='quarantined',updated_at=now() WHERE id=$1 AND state<>'abandoned'`, requestID); err != nil {
				return err
			}
			if err := pauseNextGameIntent(ctx, tx, requestID); err != nil {
				return err
			}
		}
		change = map[string]any{"before": state, "after": "quarantined", "capacityReleased": false}
	default:
		return ErrInvalidAdminAction
	}
	if err := auditAdmin(ctx, tx, adminID, a.Action, targetType, targetID, change); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
