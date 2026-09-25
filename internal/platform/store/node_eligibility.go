package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// NodeEligibility is the single predicate shared by scheduling and player
// availability reads. Capacity is only advisory in a player read; the
// allocator rechecks it while holding the node row lock.
type NodeEligibility struct {
	ID, DisplayName, BindingKey                  string
	Priority, Hard, Desired, Occupied            int
	Enabled, Accepting, Draining, ContentAccept  bool
	LastHeartbeat                                *time.Time
	Compatibility, ContentState, ReportedVersion string
}

type NodeChoice struct {
	ID             string `json:"id"`
	DisplayName    string `json:"displayName"`
	Status         string `json:"status"`
	Connectivity   string `json:"connectivity"`
	Reason         string `json:"reason"`
	AvailableSlots int    `json:"availableSlots"`
	Selectable     bool   `json:"selectable"`
}

const nodeOnlineWindow = 2 * time.Minute
const nodeOfflineWindow = 5 * time.Minute

// Node connectivity is derived from the server-recorded heartbeat time. It
// does not change Allocation state or prove an old d2core instance stopped.
func NodeConnectivity(last *time.Time, now time.Time) string {
	if last == nil || now.Sub(*last) >= nodeOfflineWindow {
		return "offline"
	}
	if now.Sub(*last) >= nodeOnlineWindow {
		return "stale"
	}
	return "online"
}

const nodeEligibilitySQL = `SELECT n.id,n.display_name,n.priority,n.enabled,n.accepting_new_requests,n.draining,
	n.last_heartbeat,n.desired_max_instances,COALESCE(r.hard_max_instances,0),
	COALESCE(r.compatibility_status,''),COALESCE(b.binding_key,''),
	COALESCE(c.reported_state,'unknown'),COALESCE(c.reported_content_version_id,''),
	COALESCE(c.accepting_new_allocations,false),
	(SELECT count(*) FROM allocations a WHERE a.node_id=n.id
	 AND a.state NOT IN ('reclaimed','released_no_effect'))
	FROM nodes n LEFT JOIN node_reports r ON r.node_id=n.id
	LEFT JOIN node_template_bindings b ON b.node_id=n.id AND b.template_revision_id=$2
	LEFT JOIN node_content_bindings c ON c.node_id=n.id AND c.arcade_game_id=$3
	WHERE n.id=$1`

type nodeRow interface{ Scan(...any) error }

func scanNodeEligibility(row nodeRow) (NodeEligibility, error) {
	var n NodeEligibility
	err := row.Scan(&n.ID, &n.DisplayName, &n.Priority, &n.Enabled, &n.Accepting, &n.Draining,
		&n.LastHeartbeat, &n.Desired, &n.Hard, &n.Compatibility, &n.BindingKey,
		&n.ContentState, &n.ReportedVersion, &n.ContentAccept, &n.Occupied)
	return n, err
}

func (n NodeEligibility) Reason(contentID string, now time.Time) string {
	if !n.Enabled || !n.Accepting || n.Draining {
		return "maintenance"
	}
	if NodeConnectivity(n.LastHeartbeat, now) != "online" {
		return "unreachable"
	}
	if n.Compatibility != "compatible" {
		return "incompatible"
	}
	if n.BindingKey == "" {
		return "template_unready"
	}
	if n.ContentState != "confirmed" || n.ReportedVersion != contentID || !n.ContentAccept {
		return "content_unready"
	}
	if n.Occupied >= min(n.Hard, n.Desired) {
		return "full"
	}
	return "available"
}

func (n NodeEligibility) Choice(contentID string, now time.Time) NodeChoice {
	reason := n.Reason(contentID, now)
	status := "available"
	if reason == "maintenance" {
		status = "maintenance"
	} else if reason != "available" && reason != "full" {
		status = "unavailable"
	} else if reason == "full" {
		status = "full"
	}
	return NodeChoice{ID: n.ID, DisplayName: n.DisplayName, Status: status, Connectivity: NodeConnectivity(n.LastHeartbeat, now), Reason: reason,
		AvailableSlots: max(0, min(n.Hard, n.Desired)-n.Occupied), Selectable: n.Enabled}
}

// PlayerNodeChoices contains presentation-safe fields only. It does not
// expose secrets, local paths, network facts or template binding keys.
func (s *Store) PlayerNodeChoices(ctx context.Context, gameID, presetID string) ([]NodeChoice, error) {
	var contentID, revisionID string
	err := s.Pool.QueryRow(ctx, `SELECT COALESCE(g.current_content_version_id,''),p.template_revision_id
		FROM arcade_games g JOIN game_presets p ON p.arcade_game_id=g.id
		WHERE g.id=$1 AND p.id=$2`, gameID, presetID).Scan(&contentID, &revisionID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrInvalidSelection
	}
	if err != nil {
		return nil, err
	}
	rows, err := s.Pool.Query(ctx, `SELECT id FROM nodes ORDER BY display_name,id`)
	if err != nil {
		return nil, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	choices := make([]NodeChoice, 0, len(ids))
	for _, id := range ids {
		n, err := scanNodeEligibility(s.Pool.QueryRow(ctx, nodeEligibilitySQL, id, revisionID, gameID))
		if err != nil {
			return nil, err
		}
		choices = append(choices, n.Choice(contentID, time.Now().UTC()))
	}
	return choices, nil
}
