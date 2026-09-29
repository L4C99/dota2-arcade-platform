package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	"github.com/jackc/pgx/v5"
)

var ErrCASConflict = errors.New("CAS_CONFLICT")
var ErrValidationIncomplete = errors.New("VALIDATION_INCOMPLETE")
var ErrScopedResourcesActive = errors.New("SCOPED_RESOURCES_ACTIVE")
var ErrCapacityFull = errors.New("CAPACITY_FULL")
var ErrContentFactMismatch = errors.New("CONTENT_FACT_MISMATCH")
var ErrTemplateFactMismatch = errors.New("TEMPLATE_FACT_MISMATCH")
var ErrInventoryUnknown = errors.New("INVENTORY_UNKNOWN")
var ErrUnaccountedInstance = errors.New("UNACCOUNTED_INSTANCE")

type ReleasePresetPlan struct {
	PresetID                      string `json:"presetId"`
	ExpectedOldTemplateRevisionID string `json:"expectedOldTemplateRevisionId"`
	NewTemplateRevisionID         string `json:"newTemplateRevisionId"`
	ExpectedOldAccepting          bool   `json:"expectedOldAccepting"`
	NewAccepting                  bool   `json:"newAccepting"`
	ValidationRunID               string `json:"validationRunId,omitempty"`
}

type ReleasePublishRequest struct {
	GameID                      string              `json:"gameId"`
	ExpectedOldContentVersionID string              `json:"expectedOldContentVersionId"`
	NewContentVersionID         string              `json:"newContentVersionId"`
	Presets                     []ReleasePresetPlan `json:"presets"`
	RollbackOfReleaseID         string              `json:"rollbackOfReleaseId,omitempty"`
	RequestID                   string              `json:"requestId"`
}

func i3PayloadHash(v any) (string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:]), nil
}

func priorI3Request(ctx context.Context, tx pgx.Tx, action, requestID, hash, adminID string) (string, error) {
	var id, oldHash, oldAdmin string
	err := tx.QueryRow(ctx, `SELECT result_id,payload_sha256,actor_admin_user_id FROM admin_i3_requests
		WHERE action=$1 AND request_id=$2`, action, requestID).Scan(&id, &oldHash, &oldAdmin)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if oldHash != hash || oldAdmin != adminID {
		return "", ErrCASConflict
	}
	return id, nil
}

// PublishRelease commits the formal content and per-Preset template pointers,
// immutable history, and audit as one CAS transaction. It never acts on files.
func (s *Store) PublishRelease(ctx context.Context, adminID string, request ReleasePublishRequest) (string, error) {
	if request.GameID == "" || request.NewContentVersionID == "" || request.RequestID == "" || len(request.RequestID) > 128 || len(request.Presets) == 0 {
		return "", ErrInvalidAdminAction
	}
	hash, err := i3PayloadHash(request)
	if err != nil {
		return "", err
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)
	// Two retries of this action serialize before taking business row locks.
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(6056625,hashtext($1))`, "release.publish:"+request.RequestID); err != nil {
		return "", err
	}
	if id, err := priorI3Request(ctx, tx, "release.publish", request.RequestID, hash, adminID); err != nil || id != "" {
		if err != nil {
			return "", err
		}
		return id, tx.Commit(ctx)
	}
	var oldContent string
	if err := tx.QueryRow(ctx, `SELECT COALESCE(current_content_version_id,'') FROM arcade_games WHERE id=$1 FOR UPDATE`, request.GameID).Scan(&oldContent); err != nil {
		return "", err
	}
	if oldContent != request.ExpectedOldContentVersionID {
		return "", ErrCASConflict
	}
	var owned bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM content_versions WHERE arcade_game_id=$1 AND id=$2)`, request.GameID, request.NewContentVersionID).Scan(&owned); err != nil {
		return "", err
	}
	if !owned {
		return "", ErrInvalidAdminAction
	}
	type existingPreset struct {
		id, template     string
		accept, upgraded bool
	}
	rows, err := tx.Query(ctx, `SELECT id,template_revision_id,accepting_new_requests,validation_contract='v1_0_2'
		FROM game_presets WHERE arcade_game_id=$1 ORDER BY id FOR UPDATE`, request.GameID)
	if err != nil {
		return "", err
	}
	current := make(map[string]existingPreset)
	first := false
	for rows.Next() {
		var p existingPreset
		if err := rows.Scan(&p.id, &p.template, &p.accept, &p.upgraded); err != nil {
			rows.Close()
			return "", err
		}
		if !p.upgraded {
			first = true
		}
		current[p.id] = p
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return "", err
	}
	rows.Close()
	var priorHistory bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM content_releases WHERE arcade_game_id=$1)`, request.GameID).Scan(&priorHistory); err != nil {
		return "", err
	}
	first = first || !priorHistory
	if first || oldContent != request.NewContentVersionID {
		if len(request.Presets) != len(current) {
			return "", ErrCASConflict
		}
	}
	planByID := make(map[string]ReleasePresetPlan)
	for _, p := range request.Presets {
		old, ok := current[p.PresetID]
		if !ok || planByID[p.PresetID].PresetID != "" || old.template != p.ExpectedOldTemplateRevisionID || old.accept != p.ExpectedOldAccepting || p.NewTemplateRevisionID == "" {
			return "", ErrCASConflict
		}
		if p.NewAccepting && p.ValidationRunID == "" {
			return "", ErrValidationIncomplete
		}
		if !p.NewAccepting && p.ValidationRunID != "" {
			return "", ErrInvalidAdminAction
		}
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM template_revisions WHERE arcade_game_id=$1 AND id=$2)`, request.GameID, p.NewTemplateRevisionID).Scan(&owned); err != nil {
			return "", err
		}
		if !owned {
			return "", ErrInvalidAdminAction
		}
		planByID[p.PresetID] = p
	}
	if (first || oldContent != request.NewContentVersionID) && len(planByID) != len(current) {
		return "", ErrCASConflict
	}
	if request.RollbackOfReleaseID != "" {
		var revertedOld, revertedNew string
		err := tx.QueryRow(ctx, `SELECT COALESCE(old_content_version_id,''),new_content_version_id FROM content_releases
			WHERE id=$1 AND arcade_game_id=$2`, request.RollbackOfReleaseID, request.GameID).Scan(&revertedOld, &revertedNew)
		if err != nil || revertedNew != oldContent || revertedOld != request.NewContentVersionID {
			return "", ErrCASConflict
		}
	}
	// Read Run identities before Node locks, then acquire proof Nodes in ID order.
	proofNode := make(map[string]string)
	nodes := make(map[string]bool)
	for _, p := range request.Presets {
		if !p.NewAccepting {
			continue
		}
		var nodeID string
		if err := tx.QueryRow(ctx, `SELECT node_id FROM validation_runs WHERE id::text=$1`, p.ValidationRunID).Scan(&nodeID); err != nil {
			return "", ErrValidationIncomplete
		}
		proofNode[p.PresetID] = nodeID
		nodes[nodeID] = true
	}
	if len(proofNode) == 0 {
		return "", ErrValidationIncomplete
	}
	nodeIDs := make([]string, 0, len(nodes))
	for id := range nodes {
		nodeIDs = append(nodeIDs, id)
	}
	sort.Strings(nodeIDs)
	for _, nodeID := range nodeIDs {
		var locked string
		if err := tx.QueryRow(ctx, `SELECT id FROM nodes WHERE id=$1 FOR UPDATE`, nodeID).Scan(&locked); err != nil {
			return "", err
		}
	}
	for _, p := range request.Presets {
		if !p.NewAccepting {
			continue
		}
		nodeID := proofNode[p.PresetID]
		var closed bool
		if err := tx.QueryRow(ctx, `SELECT NOT accepting_new_allocations FROM node_content_bindings
			WHERE node_id=$1 AND arcade_game_id=$2 FOR SHARE`, nodeID, request.GameID).Scan(&closed); err != nil || !closed {
			return "", ErrValidationIncomplete
		}
		id, err := effectiveValidationProof(ctx, tx, nodeID, request.GameID, p.PresetID, request.NewContentVersionID, p.NewTemplateRevisionID, p.ValidationRunID)
		if err != nil {
			return "", err
		}
		if id == "" {
			return "", ErrValidationIncomplete
		}
	}
	releaseID, err := NewID()
	if err != nil {
		return "", err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO content_releases(id,arcade_game_id,old_content_version_id,new_content_version_id,published_by,rollback_of_release_id)
		VALUES($1,$2,NULLIF($3,''),$4,$5,NULLIF($6,'')::uuid)`, releaseID, request.GameID, oldContent, request.NewContentVersionID, adminID, request.RollbackOfReleaseID); err != nil {
		return "", err
	}
	for _, p := range request.Presets {
		nodeID := proofNode[p.PresetID]
		if _, err := tx.Exec(ctx, `INSERT INTO content_release_presets
			(release_id,arcade_game_id,game_preset_id,old_template_revision_id,new_template_revision_id,
			old_accepting_new_requests,new_accepting_new_requests,validation_run_id,validation_node_id)
			VALUES($1,$2,$3,$4,$5,$6,$7,NULLIF($8,'')::uuid,NULLIF($9,'')::uuid)`,
			releaseID, request.GameID, p.PresetID, p.ExpectedOldTemplateRevisionID, p.NewTemplateRevisionID,
			p.ExpectedOldAccepting, p.NewAccepting, p.ValidationRunID, nodeID); err != nil {
			return "", err
		}
		if _, err := tx.Exec(ctx, `UPDATE game_presets SET template_revision_id=$2,
			accepting_new_requests=$3,validation_contract='v1_0_2' WHERE id=$1`, p.PresetID, p.NewTemplateRevisionID, p.NewAccepting); err != nil {
			return "", err
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE arcade_games SET current_content_version_id=$2 WHERE id=$1`, request.GameID, request.NewContentVersionID); err != nil {
		return "", err
	}
	if err := auditAdmin(ctx, tx, adminID, "release.publish", "arcade_game", request.GameID,
		map[string]any{"releaseId": releaseID, "oldContentVersionId": oldContent, "newContentVersionId": request.NewContentVersionID,
			"presets": request.Presets, "rollbackOfReleaseId": request.RollbackOfReleaseID}); err != nil {
		return "", err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO admin_i3_requests(action,request_id,payload_sha256,result_id,actor_admin_user_id)
		VALUES('release.publish',$1,$2,$3,$4)`, request.RequestID, hash, releaseID, adminID); err != nil {
		return "", fmt.Errorf("release idempotency: %w", err)
	}
	return releaseID, tx.Commit(ctx)
}

type ReleasePresetRecord struct {
	PresetID              string `json:"presetId"`
	OldTemplateRevisionID string `json:"oldTemplateRevisionId"`
	NewTemplateRevisionID string `json:"newTemplateRevisionId"`
	OldAccepting          bool   `json:"oldAccepting"`
	NewAccepting          bool   `json:"newAccepting"`
	ValidationRunID       string `json:"validationRunId,omitempty"`
	ValidationNodeID      string `json:"validationNodeId,omitempty"`
}

type ReleaseRecord struct {
	ID                  string                `json:"id"`
	GameID              string                `json:"gameId"`
	OldContentVersionID string                `json:"oldContentVersionId"`
	NewContentVersionID string                `json:"newContentVersionId"`
	PublishedBy         string                `json:"publishedBy"`
	PublishedAt         string                `json:"publishedAt"`
	RollbackOfReleaseID string                `json:"rollbackOfReleaseId,omitempty"`
	Presets             []ReleasePresetRecord `json:"presets"`
}

func (s *Store) ReleaseDetail(ctx context.Context, id string) (ReleaseRecord, error) {
	var out ReleaseRecord
	err := s.Pool.QueryRow(ctx, `SELECT id,arcade_game_id,COALESCE(old_content_version_id,''),new_content_version_id,
		published_by,published_at::text,COALESCE(rollback_of_release_id::text,'') FROM content_releases WHERE id=$1`, id).
		Scan(&out.ID, &out.GameID, &out.OldContentVersionID, &out.NewContentVersionID, &out.PublishedBy, &out.PublishedAt, &out.RollbackOfReleaseID)
	if err != nil {
		return ReleaseRecord{}, err
	}
	out.Presets = []ReleasePresetRecord{}
	rows, err := s.Pool.Query(ctx, `SELECT game_preset_id,old_template_revision_id,new_template_revision_id,
		old_accepting_new_requests,new_accepting_new_requests,COALESCE(validation_run_id::text,''),COALESCE(validation_node_id::text,'')
		FROM content_release_presets WHERE release_id=$1 ORDER BY game_preset_id`, id)
	if err != nil {
		return ReleaseRecord{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var p ReleasePresetRecord
		if err := rows.Scan(&p.PresetID, &p.OldTemplateRevisionID, &p.NewTemplateRevisionID, &p.OldAccepting, &p.NewAccepting, &p.ValidationRunID, &p.ValidationNodeID); err != nil {
			return ReleaseRecord{}, err
		}
		out.Presets = append(out.Presets, p)
	}
	return out, rows.Err()
}
