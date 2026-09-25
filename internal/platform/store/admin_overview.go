package store

import (
	"context"
	"time"
)

type AdminSettings struct {
	AcceptingNewRequests bool   `json:"acceptingNewRequests"`
	MaintenanceMessage   string `json:"maintenanceMessage"`
	SiteAnnouncement     string `json:"siteAnnouncement"`
}

type AdminCounts struct {
	Waiting           int `json:"waiting"`
	Creating          int `json:"creating"`
	Running           int `json:"running"`
	Stopping          int `json:"stopping"`
	Quarantined       int `json:"quarantined"`
	FailedUnreclaimed int `json:"failedUnreclaimed"`
}

type AdminGame struct {
	ID                      string `json:"id"`
	DisplayName             string `json:"displayName"`
	WorkshopID              string `json:"workshopId"`
	CurrentContentVersionID string `json:"currentContentVersionId"`
	MaintenanceMessage      string `json:"maintenanceMessage"`
	Enabled                 bool   `json:"enabled"`
	AcceptingNewRequests    bool   `json:"acceptingNewRequests"`
}

type AdminPreset struct {
	ID                   string `json:"id"`
	ArcadeGameID         string `json:"arcadeGameId"`
	DisplayName          string `json:"displayName"`
	TemplateRevisionID   string `json:"templateRevisionId"`
	MaintenanceMessage   string `json:"maintenanceMessage"`
	Enabled              bool   `json:"enabled"`
	AcceptingNewRequests bool   `json:"acceptingNewRequests"`
	MaxPlayers           int    `json:"maxPlayers"`
}

type AdminNode struct {
	ID                   string     `json:"id"`
	DisplayName          string     `json:"displayName"`
	OS                   string     `json:"os"`
	Connectivity         string     `json:"connectivity"`
	ControllerVersion    string     `json:"controllerVersion"`
	D2CoreVersion        string     `json:"d2coreVersion"`
	D2CoreCommit         string     `json:"d2coreCommit"`
	Compatibility        string     `json:"compatibility"`
	RecentErrorCode      string     `json:"recentErrorCode"`
	Enabled              bool       `json:"enabled"`
	AcceptingNewRequests bool       `json:"acceptingNewRequests"`
	Draining             bool       `json:"draining"`
	Priority             int        `json:"priority"`
	Hard                 int        `json:"hard"`
	Desired              int        `json:"desired"`
	Occupied             int        `json:"occupied"`
	LastHeartbeat        *time.Time `json:"lastHeartbeat"`
	ReconcileRequested   int64      `json:"reconcileRequested"`
	ReconcileCompleted   int64      `json:"reconcileCompleted"`
}

type AdminBinding struct {
	NodeID                   string     `json:"nodeId"`
	ArcadeGameID             string     `json:"arcadeGameId"`
	ReportedContentVersionID string     `json:"reportedContentVersionId"`
	ReportedState            string     `json:"reportedState"`
	ReportedAt               *time.Time `json:"reportedAt"`
	AcceptingNewAllocations  bool       `json:"acceptingNewAllocations"`
}

type AdminEntry struct {
	NodeID               string     `json:"nodeId"`
	A2SEnabled           bool       `json:"a2sEnabled"`
	A2SQueryOK           bool       `json:"a2sQueryOk"`
	SteamVerified        bool       `json:"steamVerified"`
	SteamEnabled         bool       `json:"steamEnabled"`
	SteamChinaVerified   bool       `json:"steamChinaVerified"`
	SteamChinaEnabled    bool       `json:"steamChinaEnabled"`
	SteamVerifiedAt      *time.Time `json:"steamVerifiedAt"`
	SteamChinaVerifiedAt *time.Time `json:"steamChinaVerifiedAt"`
}

type AdminRequest struct {
	ID                string    `json:"id"`
	ArcadeGameID      string    `json:"arcadeGameId"`
	GamePresetID      string    `json:"gamePresetId"`
	State             string    `json:"state"`
	NodeSelectionMode string    `json:"nodeSelectionMode"`
	OwnerPartyID      *string   `json:"ownerPartyId"`
	ManualNodeID      *string   `json:"manualNodeId"`
	RequestedAt       time.Time `json:"requestedAt"`
}

type AdminParty struct {
	ID                string     `json:"id"`
	LeaderDisplayName string     `json:"leaderDisplayName"`
	MemberCount       int        `json:"memberCount"`
	DissolvedAt       *time.Time `json:"dissolvedAt"`
}

type AdminAllocation struct {
	ID                 string    `json:"id"`
	ServerRequestID    string    `json:"serverRequestId"`
	NodeID             string    `json:"nodeId"`
	ContentVersionID   string    `json:"contentVersionId"`
	TemplateRevisionID string    `json:"templateRevisionId"`
	State              string    `json:"state"`
	ErrorCode          string    `json:"errorCode"`
	AttemptSequence    int       `json:"attemptSequence"`
	AssignedAt         time.Time `json:"assignedAt"`
}

type AdminJob struct {
	ID           string    `json:"id"`
	NodeID       string    `json:"nodeId"`
	AllocationID string    `json:"allocationId"`
	Kind         string    `json:"kind"`
	State        string    `json:"state"`
	ErrorCode    string    `json:"errorCode"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

type AuditEvent struct {
	ID            string         `json:"id"`
	ActorKind     string         `json:"actorKind"`
	Action        string         `json:"action"`
	TargetType    string         `json:"targetType"`
	TargetID      string         `json:"targetId"`
	Result        string         `json:"result"`
	ActorUsername string         `json:"actorUsername"`
	StateChange   map[string]any `json:"stateChange"`
	CreatedAt     time.Time      `json:"createdAt"`
}

type AdminOverview struct {
	Settings    AdminSettings     `json:"settings"`
	Counts      AdminCounts       `json:"counts"`
	Games       []AdminGame       `json:"games"`
	Presets     []AdminPreset     `json:"presets"`
	Nodes       []AdminNode       `json:"nodes"`
	Bindings    []AdminBinding    `json:"bindings"`
	Entries     []AdminEntry      `json:"entries"`
	Requests    []AdminRequest    `json:"requests"`
	Parties     []AdminParty      `json:"parties"`
	Allocations []AdminAllocation `json:"allocations"`
	Jobs        []AdminJob        `json:"jobs"`
	Audit       []AuditEvent      `json:"audit"`
}

// AdminOverview deliberately omits node secrets, private network facts,
// frozen local paths, operation tokens, and raw Controller errors.
func (s *Store) AdminOverview(ctx context.Context) (AdminOverview, error) {
	o := AdminOverview{Games: []AdminGame{}, Presets: []AdminPreset{}, Nodes: []AdminNode{},
		Bindings: []AdminBinding{}, Entries: []AdminEntry{}, Requests: []AdminRequest{}, Parties: []AdminParty{},
		Allocations: []AdminAllocation{}, Jobs: []AdminJob{}, Audit: []AuditEvent{}}
	if err := s.Pool.QueryRow(ctx, `SELECT p.accepting_new_requests,p.maintenance_message,a.message
		FROM platform_settings p CROSS JOIN site_announcements a WHERE p.singleton AND a.singleton`).
		Scan(&o.Settings.AcceptingNewRequests, &o.Settings.MaintenanceMessage, &o.Settings.SiteAnnouncement); err != nil {
		return AdminOverview{}, err
	}
	if err := s.Pool.QueryRow(ctx, `SELECT
		(SELECT count(*) FROM server_requests WHERE state='waiting'),
		(SELECT count(*) FROM allocations WHERE state IN ('reserved','creating','create_unknown')),
		(SELECT count(*) FROM allocations WHERE state='running'),
		(SELECT count(*) FROM allocations WHERE state='stopping'),
		(SELECT count(*) FROM allocations WHERE state='quarantined'),
		(SELECT count(*) FROM allocations WHERE state='failed_unreclaimed')`).
		Scan(&o.Counts.Waiting, &o.Counts.Creating, &o.Counts.Running, &o.Counts.Stopping, &o.Counts.Quarantined, &o.Counts.FailedUnreclaimed); err != nil {
		return AdminOverview{}, err
	}
	rows, err := s.Pool.Query(ctx, `SELECT id,display_name,workshop_id,COALESCE(current_content_version_id,''),maintenance_message,enabled,accepting_new_requests
		FROM arcade_games ORDER BY display_name,id`)
	if err != nil {
		return AdminOverview{}, err
	}
	for rows.Next() {
		var x AdminGame
		if err := rows.Scan(&x.ID, &x.DisplayName, &x.WorkshopID, &x.CurrentContentVersionID, &x.MaintenanceMessage, &x.Enabled, &x.AcceptingNewRequests); err != nil {
			rows.Close()
			return AdminOverview{}, err
		}
		o.Games = append(o.Games, x)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return AdminOverview{}, err
	}
	rows.Close()
	rows, err = s.Pool.Query(ctx, `SELECT p.id,COALESCE(u.display_name,''),
		(SELECT count(*) FROM party_members m WHERE m.party_id=p.id),p.dissolved_at
		FROM parties p LEFT JOIN users u ON u.id=p.leader_user_id ORDER BY p.created_at DESC,p.id DESC LIMIT 100`)
	if err != nil {
		return AdminOverview{}, err
	}
	for rows.Next() {
		var x AdminParty
		if err := rows.Scan(&x.ID, &x.LeaderDisplayName, &x.MemberCount, &x.DissolvedAt); err != nil {
			rows.Close()
			return AdminOverview{}, err
		}
		o.Parties = append(o.Parties, x)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return AdminOverview{}, err
	}
	rows.Close()
	rows, err = s.Pool.Query(ctx, `SELECT id,arcade_game_id,display_name,template_revision_id,maintenance_message,enabled,accepting_new_requests,max_players
		FROM game_presets ORDER BY display_name,id`)
	if err != nil {
		return AdminOverview{}, err
	}
	for rows.Next() {
		var x AdminPreset
		if err := rows.Scan(&x.ID, &x.ArcadeGameID, &x.DisplayName, &x.TemplateRevisionID, &x.MaintenanceMessage, &x.Enabled, &x.AcceptingNewRequests, &x.MaxPlayers); err != nil {
			rows.Close()
			return AdminOverview{}, err
		}
		o.Presets = append(o.Presets, x)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return AdminOverview{}, err
	}
	rows.Close()
	rows, err = s.Pool.Query(ctx, `SELECT n.id,n.display_name,COALESCE(n.os,''),n.enabled,n.accepting_new_requests,n.draining,n.priority,n.desired_max_instances,n.last_heartbeat,
		COALESCE(r.hard_max_instances,0),COALESCE(r.controller_version,''),COALESCE(r.d2core_version,''),COALESCE(r.d2core_commit,''),COALESCE(r.compatibility_status,''),
		(SELECT count(*) FROM allocations a WHERE a.node_id=n.id AND a.state NOT IN ('reclaimed','released_no_effect')),
		COALESCE((SELECT COALESCE(j.error_code,'') FROM node_jobs j WHERE j.node_id=n.id AND j.error_code IS NOT NULL ORDER BY j.updated_at DESC LIMIT 1),''),
		COALESCE(q.requested_generation,0),COALESCE(q.completed_generation,0)
		FROM nodes n LEFT JOIN node_reports r ON r.node_id=n.id LEFT JOIN node_reconcile_requests q ON q.node_id=n.id ORDER BY n.display_name,n.id`)
	if err != nil {
		return AdminOverview{}, err
	}
	for rows.Next() {
		var x AdminNode
		if err := rows.Scan(&x.ID, &x.DisplayName, &x.OS, &x.Enabled, &x.AcceptingNewRequests, &x.Draining, &x.Priority, &x.Desired, &x.LastHeartbeat, &x.Hard, &x.ControllerVersion, &x.D2CoreVersion, &x.D2CoreCommit, &x.Compatibility, &x.Occupied, &x.RecentErrorCode, &x.ReconcileRequested, &x.ReconcileCompleted); err != nil {
			rows.Close()
			return AdminOverview{}, err
		}
		x.Connectivity = NodeConnectivity(x.LastHeartbeat, time.Now())
		o.Nodes = append(o.Nodes, x)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return AdminOverview{}, err
	}
	rows.Close()
	rows, err = s.Pool.Query(ctx, `SELECT node_id,arcade_game_id,COALESCE(reported_content_version_id,''),reported_state,reported_at,accepting_new_allocations
		FROM node_content_bindings ORDER BY node_id,arcade_game_id`)
	if err != nil {
		return AdminOverview{}, err
	}
	for rows.Next() {
		var x AdminBinding
		if err := rows.Scan(&x.NodeID, &x.ArcadeGameID, &x.ReportedContentVersionID, &x.ReportedState, &x.ReportedAt, &x.AcceptingNewAllocations); err != nil {
			rows.Close()
			return AdminOverview{}, err
		}
		o.Bindings = append(o.Bindings, x)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return AdminOverview{}, err
	}
	rows.Close()
	rows, err = s.Pool.Query(ctx, `SELECT node_id,a2s_enabled,a2s_query_ok,steam_entry_verified,steam_entry_enabled,steam_verified_at,
		steamchina_entry_verified,steamchina_entry_enabled,steamchina_verified_at FROM node_entry_capabilities ORDER BY node_id`)
	if err != nil {
		return AdminOverview{}, err
	}
	for rows.Next() {
		var x AdminEntry
		if err := rows.Scan(&x.NodeID, &x.A2SEnabled, &x.A2SQueryOK, &x.SteamVerified, &x.SteamEnabled, &x.SteamVerifiedAt, &x.SteamChinaVerified, &x.SteamChinaEnabled, &x.SteamChinaVerifiedAt); err != nil {
			rows.Close()
			return AdminOverview{}, err
		}
		o.Entries = append(o.Entries, x)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return AdminOverview{}, err
	}
	rows.Close()
	rows, err = s.Pool.Query(ctx, `SELECT id,arcade_game_id,game_preset_id,state,node_selection_mode,owner_party_id,manual_node_id,requested_at
		FROM server_requests ORDER BY requested_at DESC,id DESC LIMIT 100`)
	if err != nil {
		return AdminOverview{}, err
	}
	for rows.Next() {
		var x AdminRequest
		if err := rows.Scan(&x.ID, &x.ArcadeGameID, &x.GamePresetID, &x.State, &x.NodeSelectionMode, &x.OwnerPartyID, &x.ManualNodeID, &x.RequestedAt); err != nil {
			rows.Close()
			return AdminOverview{}, err
		}
		o.Requests = append(o.Requests, x)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return AdminOverview{}, err
	}
	rows.Close()
	rows, err = s.Pool.Query(ctx, `SELECT id,server_request_id,node_id,content_version_id,template_revision_id,state,COALESCE(error_code,''),attempt_sequence,assigned_at
		FROM allocations ORDER BY assigned_at DESC,id DESC LIMIT 100`)
	if err != nil {
		return AdminOverview{}, err
	}
	for rows.Next() {
		var x AdminAllocation
		if err := rows.Scan(&x.ID, &x.ServerRequestID, &x.NodeID, &x.ContentVersionID, &x.TemplateRevisionID, &x.State, &x.ErrorCode, &x.AttemptSequence, &x.AssignedAt); err != nil {
			rows.Close()
			return AdminOverview{}, err
		}
		o.Allocations = append(o.Allocations, x)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return AdminOverview{}, err
	}
	rows.Close()
	rows, err = s.Pool.Query(ctx, `SELECT id,node_id,COALESCE(allocation_id::text,''),kind,state,COALESCE(error_code,''),updated_at
		FROM node_jobs WHERE state IN ('pending','claimed','accepted','unknown') ORDER BY updated_at DESC,id DESC LIMIT 100`)
	if err != nil {
		return AdminOverview{}, err
	}
	for rows.Next() {
		var x AdminJob
		if err := rows.Scan(&x.ID, &x.NodeID, &x.AllocationID, &x.Kind, &x.State, &x.ErrorCode, &x.UpdatedAt); err != nil {
			rows.Close()
			return AdminOverview{}, err
		}
		o.Jobs = append(o.Jobs, x)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return AdminOverview{}, err
	}
	rows.Close()
	rows, err = s.Pool.Query(ctx, `SELECT e.id,e.actor_kind,e.action,e.target_type,e.target_id,e.result,e.state_change,e.created_at,
		COALESCE(a.username,'') FROM audit_events e LEFT JOIN admin_users a ON a.id=e.actor_admin_user_id
		ORDER BY e.created_at DESC,e.id DESC LIMIT 100`)
	if err != nil {
		return AdminOverview{}, err
	}
	for rows.Next() {
		var x AuditEvent
		if err := rows.Scan(&x.ID, &x.ActorKind, &x.Action, &x.TargetType, &x.TargetID, &x.Result, &x.StateChange, &x.CreatedAt, &x.ActorUsername); err != nil {
			rows.Close()
			return AdminOverview{}, err
		}
		o.Audit = append(o.Audit, x)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return AdminOverview{}, err
	}
	rows.Close()
	return o, nil
}
