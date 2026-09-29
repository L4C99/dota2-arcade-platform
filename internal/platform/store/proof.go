package store

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
)

// effectiveValidationProof is the current, persisted Node × Preset proof.
// Callers lock the Game/Preset and then the Node before using its result for
// admission or publication. A historical PASS alone is never sufficient.
// The content and template arguments are the combination the caller intends
// to admit; publication may therefore check a candidate before pointer swap.
func effectiveValidationProof(ctx context.Context, tx pgx.Tx, nodeID, gameID, presetID, contentID, revisionID, runID string) (string, error) {
	var id string
	err := tx.QueryRow(ctx, `SELECT v.id FROM validation_runs v
		JOIN allocations va ON va.validation_run_id=v.id AND va.state='reclaimed'
		JOIN content_versions cv ON cv.arcade_game_id=v.arcade_game_id AND cv.id=v.content_version_id
		JOIN nodes n ON n.id=v.node_id
		JOIN node_reports nr ON nr.node_id=n.id
		JOIN node_content_bindings cb ON cb.node_id=n.id AND cb.arcade_game_id=v.arcade_game_id
		JOIN node_template_bindings tb ON tb.node_id=n.id AND tb.template_revision_id=v.template_revision_id
		JOIN node_template_facts tf ON tf.node_id=n.id AND tf.template_revision_id=v.template_revision_id
		JOIN LATERAL (
			SELECT i.complete,i.state,i.received_at,i.unaccounted_count,i.scan_id
			FROM node_inventory_snapshots i WHERE i.node_id=n.id
			ORDER BY i.received_at DESC,i.scan_id DESC LIMIT 1
		) inv ON true
		WHERE v.node_id=$1 AND v.arcade_game_id=$2 AND v.game_preset_id=$3
		AND v.content_version_id=$4 AND v.template_revision_id=$5
		AND ($6='' OR v.id::text=$6)
		AND v.state='passed' AND v.result_code='PASS' AND v.human_result='pass'
		AND v.ready_at IS NOT NULL AND v.join_info_available_at IS NOT NULL
		AND cb.maintenance_epoch=v.maintenance_epoch
		AND cb.content_fact_revision=v.content_fact_revision
		AND cb.reported_state='confirmed'
		AND cb.reported_content_version_id=v.content_version_id
		AND cb.reported_content_sha256=v.content_sha256
		AND cv.content_sha256=v.content_sha256
		AND cb.reported_at > $7::timestamptz - interval '2 minutes'
		AND tb.binding_key=v.template_binding_key
		AND tb.binding_generation=v.template_binding_generation
		AND tb.expected_template_fingerprint_sha256=v.template_fingerprint_sha256
		AND tf.binding_key=v.template_binding_key
		AND tf.reported_state='confirmed'
		AND tf.manifest_algorithm='template-manifest-sha256-v1'
		AND tf.reported_fingerprint_sha256=v.template_fingerprint_sha256
		AND tf.template_fact_revision=v.template_fact_revision
		AND tf.received_at > $7::timestamptz - interval '2 minutes'
		AND n.enabled AND n.accepting_new_requests AND NOT n.draining
		AND n.last_heartbeat > $7::timestamptz - interval '2 minutes'
		AND nr.compatibility_status='compatible'
		AND 'contentValidationV102'=ANY(nr.capabilities)
		AND nr.inventory_state='confirmed' AND nr.inventory_scan_id=inv.scan_id
		AND inv.complete AND inv.state='confirmed' AND inv.unaccounted_count=0
		AND inv.received_at > $7::timestamptz - interval '2 minutes'
		ORDER BY v.passed_at DESC,v.id DESC LIMIT 1`,
		nodeID, gameID, presetID, contentID, revisionID, runID, time.Now().UTC()).Scan(&id)
	if err == pgx.ErrNoRows {
		return "", nil
	}
	return id, err
}
