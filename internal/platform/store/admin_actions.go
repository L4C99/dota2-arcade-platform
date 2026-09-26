package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/L4C99/dota2-arcade-platform/internal/contracts/nodev1"
	"github.com/jackc/pgx/v5"
)

var ErrInvalidAdminAction = errors.New("invalid administrator action")
var workshopIDPattern = regexp.MustCompile(`^[0-9]+$`)
var contentSHA256Pattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

type AdminAction struct {
	Action             string  `json:"action"`
	TargetID           string  `json:"targetId"`
	GameID             string  `json:"gameId,omitempty"`
	Accepting          *bool   `json:"accepting,omitempty"`
	Enabled            *bool   `json:"enabled,omitempty"`
	Draining           *bool   `json:"draining,omitempty"`
	Priority           *int    `json:"priority,omitempty"`
	Desired            *int    `json:"desired,omitempty"`
	Message            *string `json:"message,omitempty"`
	Entry              string  `json:"entry,omitempty"`
	Verified           *bool   `json:"verified,omitempty"`
	Confirmed          bool    `json:"confirmed,omitempty"`
	WorkshopID         string  `json:"workshopId,omitempty"`
	DisplayName        string  `json:"displayName,omitempty"`
	ContentVersionID   string  `json:"contentVersionId,omitempty"`
	ContentSHA256      string  `json:"contentSha256,omitempty"`
	TemplateRevisionID string  `json:"templateRevisionId,omitempty"`
	BindingKey         string  `json:"bindingKey,omitempty"`
	Description        string  `json:"description,omitempty"`
	MaxPlayers         int     `json:"maxPlayers,omitempty"`
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
	if len(a.TargetID) > 200 || len(a.GameID) > 200 || a.Message != nil && len(*a.Message) > 1000 ||
		len(a.DisplayName) > 128 || len(a.ContentVersionID) > 128 || len(a.TemplateRevisionID) > 128 ||
		len(a.BindingKey) > 128 || len(a.Description) > 1000 {
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
	case "game.create":
		if a.TargetID != "" || !workshopIDPattern.MatchString(a.WorkshopID) || strings.TrimSpace(a.DisplayName) == "" {
			return ErrInvalidAdminAction
		}
		id, err := NewID()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO arcade_games(id,workshop_id,display_name,enabled,accepting_new_requests)
			VALUES($1,$2,$3,false,false)`, id, a.WorkshopID, strings.TrimSpace(a.DisplayName)); err != nil {
			return err
		}
		targetType, targetID = "arcade_game", id
		change = map[string]any{"workshopId": a.WorkshopID, "displayName": strings.TrimSpace(a.DisplayName), "enabled": false, "accepting": false}
	case "template.create":
		if a.TargetID == "" || strings.TrimSpace(a.TemplateRevisionID) == "" {
			return ErrInvalidAdminAction
		}
		if _, err := tx.Exec(ctx, `INSERT INTO template_revisions(id,arcade_game_id,description) VALUES($1,$2,$3)`,
			a.TemplateRevisionID, a.TargetID, strings.TrimSpace(a.Description)); err != nil {
			return err
		}
		targetType, targetID = "template_revision", a.TemplateRevisionID
		change = map[string]any{"arcadeGameId": a.TargetID, "description": strings.TrimSpace(a.Description)}
	case "content.create":
		if a.TargetID == "" || strings.TrimSpace(a.ContentVersionID) == "" || !contentSHA256Pattern.MatchString(a.ContentSHA256) {
			return ErrInvalidAdminAction
		}
		if _, err := tx.Exec(ctx, `INSERT INTO content_versions(id,arcade_game_id,content_sha256) VALUES($1,$2,$3)`,
			a.ContentVersionID, a.TargetID, a.ContentSHA256); err != nil {
			return err
		}
		targetType, targetID = "content_version", a.ContentVersionID
		change = map[string]any{"arcadeGameId": a.TargetID, "sha256": a.ContentSHA256}
	case "preset.create":
		if a.TargetID == "" || strings.TrimSpace(a.DisplayName) == "" || a.TemplateRevisionID == "" || a.MaxPlayers <= 0 {
			return ErrInvalidAdminAction
		}
		id, err := NewID()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO game_presets(id,arcade_game_id,display_name,max_players,template_revision_id,enabled,accepting_new_requests)
			VALUES($1,$2,$3,$4,$5,false,false)`, id, a.TargetID, strings.TrimSpace(a.DisplayName), a.MaxPlayers, a.TemplateRevisionID); err != nil {
			return err
		}
		targetType, targetID = "game_preset", id
		change = map[string]any{"arcadeGameId": a.TargetID, "displayName": strings.TrimSpace(a.DisplayName), "maxPlayers": a.MaxPlayers, "templateRevisionId": a.TemplateRevisionID, "enabled": false, "accepting": false}
	case "template_binding.upsert":
		if a.TargetID == "" || a.TemplateRevisionID == "" || strings.TrimSpace(a.BindingKey) == "" {
			return ErrInvalidAdminAction
		}
		var old string
		err := tx.QueryRow(ctx, `SELECT binding_key FROM node_template_bindings WHERE node_id=$1 AND template_revision_id=$2 FOR UPDATE`, a.TargetID, a.TemplateRevisionID).Scan(&old)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO node_template_bindings(node_id,template_revision_id,binding_key) VALUES($1,$2,$3)
			ON CONFLICT(node_id,template_revision_id) DO UPDATE SET binding_key=EXCLUDED.binding_key`,
			a.TargetID, a.TemplateRevisionID, strings.TrimSpace(a.BindingKey)); err != nil {
			return err
		}
		targetType, targetID = "node_template_binding", a.TargetID+":"+a.TemplateRevisionID
		change = map[string]any{"before": old, "after": strings.TrimSpace(a.BindingKey)}
	case "content.validate":
		if a.TargetID == "" || a.GameID == "" || a.ContentVersionID == "" || !a.Confirmed {
			return ErrInvalidAdminAction
		}
		var drained, compatibleAndFresh bool
		var state, version, reportedHash, expectedHash string
		var occupied int
		err := tx.QueryRow(ctx, `SELECT n.draining,COALESCE(n.last_heartbeat > now()-interval '2 minutes' AND r.compatibility_status='compatible',false),
			b.reported_state,COALESCE(b.reported_content_version_id,''),COALESCE(b.reported_content_sha256,''),v.content_sha256,
			(SELECT count(*) FROM allocations x WHERE x.node_id=n.id AND x.state NOT IN ('reclaimed','released_no_effect'))
			FROM nodes n JOIN node_content_bindings b ON b.node_id=n.id AND b.arcade_game_id=$2
			JOIN node_reports r ON r.node_id=n.id
			JOIN content_versions v ON v.arcade_game_id=b.arcade_game_id AND v.id=$3
			WHERE n.id=$1 FOR UPDATE OF n,b`,
			a.TargetID, a.GameID, a.ContentVersionID).Scan(&drained, &compatibleAndFresh, &state, &version, &reportedHash, &expectedHash, &occupied)
		if err != nil {
			return err
		}
		if !drained || !compatibleAndFresh || state != "confirmed" || version != a.ContentVersionID || reportedHash != expectedHash || occupied != 0 {
			return fmt.Errorf("%w: validation requires drained, empty node and confirmed version", ErrJobConflict)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO content_release_validations(node_id,arcade_game_id,content_version_id,verified_by)
			VALUES($1,$2,$3,$4) ON CONFLICT(node_id,arcade_game_id,content_version_id)
			DO UPDATE SET verified_by=EXCLUDED.verified_by,verified_at=now()`, a.TargetID, a.GameID, a.ContentVersionID, adminID); err != nil {
			return err
		}
		targetType, targetID = "content_validation", a.TargetID+":"+a.ContentVersionID
		change = map[string]any{"arcadeGameId": a.GameID, "contentVersionId": a.ContentVersionID, "humanConfirmed": true}
	case "content.publish":
		if a.TargetID == "" || a.ContentVersionID == "" || !a.Confirmed {
			return ErrInvalidAdminAction
		}
		var old string
		if err := tx.QueryRow(ctx, `SELECT COALESCE(current_content_version_id,'') FROM arcade_games WHERE id=$1 FOR UPDATE`, a.TargetID).Scan(&old); err != nil {
			return err
		}
		var readyNode string
		err := tx.QueryRow(ctx, `SELECT b.node_id FROM content_release_validations v
			JOIN node_content_bindings b ON b.node_id=v.node_id AND b.arcade_game_id=v.arcade_game_id
			JOIN content_versions c ON c.arcade_game_id=v.arcade_game_id AND c.id=v.content_version_id
			JOIN nodes n ON n.id=v.node_id JOIN node_reports r ON r.node_id=n.id
			WHERE v.arcade_game_id=$1 AND v.content_version_id=$2 AND b.reported_state='confirmed'
			AND b.reported_content_version_id=v.content_version_id AND b.reported_content_sha256=c.content_sha256
			AND n.enabled AND n.accepting_new_requests AND NOT n.draining AND r.compatibility_status='compatible'
			AND n.last_heartbeat > now()-interval '2 minutes' LIMIT 1 FOR SHARE OF b,n`, a.TargetID, a.ContentVersionID).Scan(&readyNode)
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("%w: no validated, currently eligible target node", ErrJobConflict)
		}
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE arcade_games SET current_content_version_id=$2 WHERE id=$1`, a.TargetID, a.ContentVersionID); err != nil {
			return err
		}
		targetType, targetID = "arcade_game", a.TargetID
		change = map[string]any{"contentVersionBefore": old, "contentVersionAfter": a.ContentVersionID, "validatedNodeId": readyNode}
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
		if a.Verified != nil && !*a.Verified {
			return ErrInvalidAdminAction // Same-revision attestation is monotonic.
		}
		if a.Verified != nil && *a.Verified && !a.Confirmed {
			return ErrInvalidAdminAction
		}
		if a.Verified != nil && a.Enabled != nil {
			return ErrInvalidAdminAction
		}
		verifiedCol, enabledCol, atCol, byCol := "steam_entry_verified", "steam_entry_enabled", "steam_verified_at", "steam_verified_by"
		if a.Entry == "steamchina" {
			verifiedCol, enabledCol, atCol, byCol = "steamchina_entry_verified", "steamchina_entry_enabled", "steamchina_verified_at", "steamchina_verified_by"
		}
		var oldVerified, oldEnabled bool
		var revision string
		if err := tx.QueryRow(ctx, `SELECT `+verifiedCol+`,`+enabledCol+`,entry_config_revision FROM node_entry_capabilities WHERE node_id=$1 FOR UPDATE`, a.TargetID).Scan(&oldVerified, &oldEnabled, &revision); err != nil {
			return err
		}
		verified, enabled := oldVerified, oldEnabled
		if a.Verified != nil {
			if oldVerified {
				return fmt.Errorf("%w: entry already verified for this revision", ErrJobConflict)
			}
			verified = true
		}
		if a.Enabled != nil {
			enabled = *a.Enabled
		}
		if enabled && !verified {
			return ErrInvalidAdminAction
		}
		// Verification is the administrator's explicit real-client claim for the
		// current Controller-reported revision, never an A2S or port-pool claim.
		if a.Verified != nil && *a.Verified {
			var networkRaw []byte
			if err := tx.QueryRow(ctx, `SELECT network_facts FROM node_reports WHERE node_id=$1`, a.TargetID).Scan(&networkRaw); err != nil {
				return err
			}
			var network nodev1.NetworkFacts
			if err := json.Unmarshal(networkRaw, &network); err != nil {
				return err
			}
			if revision == "" || revision != nodev1.EntryConfigRevision(network) || network.ProtocolIP == "" || len(nodev1.PublicPorts(network)) == 0 {
				return fmt.Errorf("%w: current protocol entry configuration is unavailable", ErrJobConflict)
			}
		}
		query := fmt.Sprintf(`UPDATE node_entry_capabilities SET %s=$2,%s=$3,
			%s=CASE WHEN $5 THEN CASE WHEN $2 THEN now() ELSE NULL END ELSE %s END,
			%s=CASE WHEN $5 THEN CASE WHEN $2 THEN $4::uuid ELSE NULL END ELSE %s END
			WHERE node_id=$1`, verifiedCol, enabledCol, atCol, atCol, byCol, byCol)
		if _, err := tx.Exec(ctx, query, a.TargetID, verified, enabled, adminID, a.Verified != nil); err != nil {
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
		if state != "running" && state != "quarantined" && state != "abandoned" {
			return fmt.Errorf("%w: request cannot be stopped", ErrJobConflict)
		}
		var allocationID, allocationState, nodeID, instanceID, createError string
		err := tx.QueryRow(ctx, `SELECT a.id,a.state,a.node_id,j.instance_id,COALESCE(j.error_code,'') FROM allocations a
			JOIN node_jobs j ON j.allocation_id=a.id AND j.kind='create' AND j.instance_id IS NOT NULL
			WHERE a.server_request_id=$1 AND a.state IN ('running','quarantined')
			ORDER BY a.attempt_sequence DESC LIMIT 1 FOR UPDATE OF a`, a.TargetID).Scan(&allocationID, &allocationState, &nodeID, &instanceID, &createError)
		if errors.Is(err, pgx.ErrNoRows) && state == "abandoned" {
			return fmt.Errorf("%w: abandoned resource is not quarantined", ErrJobConflict)
		}
		if err != nil {
			return err
		}
		if state == "abandoned" && allocationState != "quarantined" {
			return fmt.Errorf("%w: abandoned resource is not quarantined", ErrJobConflict)
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
		requestAfter := state
		if state == "running" {
			requestAfter = "stopping"
		}
		change = map[string]any{"requestBefore": state, "stopJobId": jobID, "allocationId": allocationID, "requestAfter": requestAfter}
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
