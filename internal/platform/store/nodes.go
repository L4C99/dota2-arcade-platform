package store

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/L4C99/dota2-arcade-platform/internal/contracts/nodev1"
	"github.com/L4C99/dota2-arcade-platform/internal/platform/auth"
	"github.com/jackc/pgx/v5"
)

var ErrNodeUnauthorized = errors.New("node unauthorized")

// RegisterNode is called only from the local administrator CLI. The raw
// secret is returned once and only its SHA-256 verifier is stored.
func (s *Store) RegisterNode(ctx context.Context, name, osName string) (nodeID, secret string, err error) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 128 || (osName != "windows" && osName != "linux") {
		return "", "", fmt.Errorf("invalid node registration")
	}
	nodeID, err = NewID()
	if err != nil {
		return "", "", err
	}
	secret, hash, err := auth.NewToken()
	if err != nil {
		return "", "", err
	}
	_, err = s.Pool.Exec(ctx, "INSERT INTO nodes(id,display_name,os,secret_hash) VALUES($1,$2,$3,$4)", nodeID, name, osName, hash[:])
	if err != nil {
		return "", "", err
	}
	return nodeID, secret, nil
}

func (s *Store) AuthenticateNode(ctx context.Context, nodeID, secret string) error {
	provided, ok := auth.TokenHash(secret)
	if !ok {
		return ErrNodeUnauthorized
	}
	var stored []byte
	var enabled bool
	err := s.Pool.QueryRow(ctx, "SELECT secret_hash,enabled FROM nodes WHERE id=$1", nodeID).Scan(&stored, &enabled)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNodeUnauthorized
	}
	if err != nil {
		return err
	}
	if !enabled || subtle.ConstantTimeCompare(provided[:], stored) != 1 {
		return ErrNodeUnauthorized
	}
	return nil
}

func (s *Store) RecordHeartbeat(ctx context.Context, nodeID string, h nodev1.Heartbeat) (nodev1.HeartbeatResult, error) {
	if err := h.Validate(); err != nil {
		return nodev1.HeartbeatResult{}, err
	}
	if h.Content == nil {
		h.Content = []nodev1.ContentFact{}
	}
	network, err := json.Marshal(h.Network)
	if err != nil {
		return nodev1.HeartbeatResult{}, err
	}
	content, err := json.Marshal(h.Content)
	if err != nil {
		return nodev1.HeartbeatResult{}, err
	}
	status := "incompatible"
	if nodev1.Compatible(h) {
		status = "compatible"
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return nodev1.HeartbeatResult{}, err
	}
	defer tx.Rollback(ctx)
	var reportedAt time.Time
	err = tx.QueryRow(ctx, `UPDATE nodes SET last_heartbeat=now() WHERE id=$1 AND os=$2 AND enabled
        RETURNING last_heartbeat`, nodeID, h.OS).Scan(&reportedAt)
	if err != nil {
		return nodev1.HeartbeatResult{}, err
	}
	// A missing or unrecognized Controller fact cannot leave an older binding
	// looking confirmed. The Platform controls only the accepting flag.
	if _, err := tx.Exec(ctx, `UPDATE node_content_bindings SET reported_state='unknown',
		reported_content_version_id=NULL,reported_at=$2 WHERE node_id=$1`, nodeID, reportedAt); err != nil {
		return nodev1.HeartbeatResult{}, err
	}
	for _, fact := range h.Content {
		var gameID string
		var versionID *string
		err := tx.QueryRow(ctx, `SELECT g.id,c.id FROM arcade_games g
			LEFT JOIN content_versions c ON c.arcade_game_id=g.id AND c.id=$2
			WHERE g.workshop_id=$1`, fact.WorkshopID, fact.ContentVersionID).Scan(&gameID, &versionID)
		if errors.Is(err, pgx.ErrNoRows) {
			continue
		}
		if err != nil {
			return nodev1.HeartbeatResult{}, err
		}
		state := "unknown"
		if fact.State != "confirmed" {
			versionID = nil
		} else if versionID != nil {
			state = "confirmed"
		}
		_, err = tx.Exec(ctx, `INSERT INTO node_content_bindings(node_id,arcade_game_id,reported_content_version_id,reported_state,reported_at)
			VALUES($1,$2,$3,$4,$5) ON CONFLICT(node_id,arcade_game_id) DO UPDATE SET
			reported_content_version_id=EXCLUDED.reported_content_version_id,
			reported_state=EXCLUDED.reported_state,reported_at=EXCLUDED.reported_at`,
			nodeID, gameID, versionID, state, reportedAt)
		if err != nil {
			return nodev1.HeartbeatResult{}, err
		}
	}
	_, err = tx.Exec(ctx, `INSERT INTO node_reports(node_id,controller_version,node_api_version,d2core_version,d2core_commit,
        d2core_protocol_version,compatibility_status,hard_max_instances,network_facts,content_facts,reported_at)
        VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
        ON CONFLICT (node_id) DO UPDATE SET
        controller_version=EXCLUDED.controller_version,node_api_version=EXCLUDED.node_api_version,d2core_version=EXCLUDED.d2core_version,
        d2core_commit=EXCLUDED.d2core_commit,d2core_protocol_version=EXCLUDED.d2core_protocol_version,
        compatibility_status=EXCLUDED.compatibility_status,hard_max_instances=EXCLUDED.hard_max_instances,
        network_facts=EXCLUDED.network_facts,content_facts=EXCLUDED.content_facts,reported_at=EXCLUDED.reported_at`,
		nodeID, h.ControllerVersion, h.NodeAPIVersion, h.D2CoreVersion, h.D2CoreCommit, h.D2CoreProtocolVersion,
		status, h.HardMaxInstances, network, content, reportedAt)
	if err != nil {
		return nodev1.HeartbeatResult{}, err
	}
	revision := nodev1.EntryConfigRevision(h.Network)
	_, err = tx.Exec(ctx, `INSERT INTO node_entry_capabilities(node_id,entry_config_revision,a2s_enabled,a2s_query_ok,reported_at)
		VALUES($1,$2,$3,$4,$5) ON CONFLICT(node_id) DO UPDATE SET
		entry_config_revision=EXCLUDED.entry_config_revision,
		a2s_enabled=EXCLUDED.a2s_enabled,a2s_query_ok=EXCLUDED.a2s_query_ok,
		steam_entry_verified=CASE WHEN node_entry_capabilities.entry_config_revision=EXCLUDED.entry_config_revision THEN node_entry_capabilities.steam_entry_verified ELSE false END,
		steam_entry_enabled=CASE WHEN node_entry_capabilities.entry_config_revision=EXCLUDED.entry_config_revision THEN node_entry_capabilities.steam_entry_enabled ELSE false END,
		steam_verified_at=CASE WHEN node_entry_capabilities.entry_config_revision=EXCLUDED.entry_config_revision THEN node_entry_capabilities.steam_verified_at ELSE NULL END,
		steam_verified_by=CASE WHEN node_entry_capabilities.entry_config_revision=EXCLUDED.entry_config_revision THEN node_entry_capabilities.steam_verified_by ELSE NULL END,
		steamchina_entry_verified=CASE WHEN node_entry_capabilities.entry_config_revision=EXCLUDED.entry_config_revision THEN node_entry_capabilities.steamchina_entry_verified ELSE false END,
		steamchina_entry_enabled=CASE WHEN node_entry_capabilities.entry_config_revision=EXCLUDED.entry_config_revision THEN node_entry_capabilities.steamchina_entry_enabled ELSE false END,
		steamchina_verified_at=CASE WHEN node_entry_capabilities.entry_config_revision=EXCLUDED.entry_config_revision THEN node_entry_capabilities.steamchina_verified_at ELSE NULL END,
		steamchina_verified_by=CASE WHEN node_entry_capabilities.entry_config_revision=EXCLUDED.entry_config_revision THEN node_entry_capabilities.steamchina_verified_by ELSE NULL END,
		reported_at=EXCLUDED.reported_at`, nodeID, revision, h.Network.A2SEnabled, h.Network.A2SEnabled && h.A2SQueryOK, reportedAt)
	if err != nil {
		return nodev1.HeartbeatResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nodev1.HeartbeatResult{}, err
	}
	return nodev1.HeartbeatResult{CompatibilityStatus: status, ReportedAt: reportedAt.UTC().Format(time.RFC3339Nano)}, nil
}

func (s *Store) NodeCompatible(ctx context.Context, nodeID string) (bool, error) {
	var status string
	err := s.Pool.QueryRow(ctx, `SELECT r.compatibility_status FROM node_reports r JOIN nodes n ON n.id=r.node_id
        WHERE n.id=$1 AND n.enabled`, nodeID).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return status == "compatible", err
}
