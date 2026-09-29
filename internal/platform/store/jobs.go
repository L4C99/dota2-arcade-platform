package store

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/L4C99/dota2-arcade-platform/internal/contracts/nodev1"
	"github.com/jackc/pgx/v5"
)

type FrozenCreate struct {
	NodeJobID                 string `json:"nodeJobId"`
	IdempotencyKey            string `json:"idempotencyKey"`
	TemplatePath              string `json:"template"`
	Port                      int    `json:"port"`
	FingerprintSHA            []byte `json:"-"`
	RequiredCapability        string `json:"requiredCapability"`
	TemplateManifestAlgorithm string `json:"templateManifestAlgorithm,omitempty"`
	TemplateFingerprintSHA256 string `json:"templateFingerprintSha256,omitempty"`
}

var ErrJobConflict = errors.New("node job conflict")

func CoreIdempotencyKey(nodeJobID string) string {
	return "nodejob-" + strings.ReplaceAll(nodeJobID, "-", "")
}

// CreateIntegrationJob is the P0-only dispatch entry point. P1 will create
// ordinary jobs through an Allocation, using the same durable job machinery.
func (s *Store) CreateIntegrationJob(ctx context.Context, nodeID, kind, instanceID string) (string, error) {
	if kind != "create" && kind != "stop" {
		return "", fmt.Errorf("unsupported job kind")
	}
	if (kind == "create" && instanceID != "") || (kind == "stop" && instanceID == "") {
		return "", fmt.Errorf("invalid instance ID for %s", kind)
	}
	id, err := NewID()
	if err != nil {
		return "", err
	}
	_, err = s.Pool.Exec(ctx, `INSERT INTO node_jobs(id,node_id,kind,integration_only,instance_id)
        VALUES($1,$2,$3,true,NULLIF($4,''))`, id, nodeID, kind, instanceID)
	return id, err
}

// CreateIntegrationCreateJob records the logical template binding. Only the
// Controller may resolve it to a trusted local absolute path before prepare.
func (s *Store) CreateIntegrationCreateJob(ctx context.Context, nodeID, bindingKey string, port int) (string, error) {
	if len(bindingKey) < 1 || len(bindingKey) > 128 || port < 0 || port > 65535 {
		return "", fmt.Errorf("invalid integration create target")
	}
	id, err := NewID()
	if err != nil {
		return "", err
	}
	_, err = s.Pool.Exec(ctx, `INSERT INTO node_jobs(id,node_id,kind,integration_only,template_binding_key,requested_port)
		VALUES($1,$2,'create',true,$3,$4)`, id, nodeID, bindingKey, port)
	return id, err
}

// PrepareCreate persists every d2core create parameter before the first core
// call. A repeated prepare must match the previously frozen request exactly.
func (s *Store) PrepareCreate(ctx context.Context, nodeID, jobID, template string, port int) (FrozenCreate, error) {
	return s.prepareCreate(ctx, nodeID, jobID, template, port, "", "")
}

// PrepareCreateWithManifest is the Store boundary for I2's verified manifest
// report. The legacy entry point cannot freeze a v1.0.2 execution.
func (s *Store) PrepareCreateWithManifest(ctx context.Context, nodeID, jobID, template string, port int, algorithm, fingerprintSHA256 string) (FrozenCreate, error) {
	return s.prepareCreate(ctx, nodeID, jobID, template, port, algorithm, fingerprintSHA256)
}

func (s *Store) prepareCreate(ctx context.Context, nodeID, jobID, template string, port int, algorithm, manifestSHA string) (FrozenCreate, error) {
	if template == "" || port < 0 || port > 65535 {
		return FrozenCreate{}, fmt.Errorf("%w: invalid create request", ErrJobConflict)
	}
	key := CoreIdempotencyKey(jobID)
	fingerprint := nodev1.CreateFingerprint(key, template, port)
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return FrozenCreate{}, err
	}
	defer tx.Rollback(ctx)
	if _, _, _, err = lockJobBusinessRows(ctx, tx, nodeID, jobID); err != nil {
		return FrozenCreate{}, err
	}
	var kind, state, requiredCapability string
	err = tx.QueryRow(ctx, "SELECT kind,state,required_capability FROM node_jobs WHERE id=$1 AND node_id=$2", jobID, nodeID).Scan(&kind, &state, &requiredCapability)
	if err != nil {
		return FrozenCreate{}, err
	}
	if kind != "create" || state == "pending" || state == "succeeded" || state == "rejected_no_effect" || state == "failed_with_effect" {
		return FrozenCreate{}, fmt.Errorf("%w: job cannot be prepared in state %s", ErrJobConflict, state)
	}
	if requiredCapability == "content_validation_v102" {
		var expected string
		if err := tx.QueryRow(ctx, `SELECT expected_template_fingerprint_sha256 FROM node_jobs
			WHERE id=$1 AND node_id=$2`, jobID, nodeID).Scan(&expected); err != nil {
			return FrozenCreate{}, err
		}
		if algorithm != "template-manifest-sha256-v1" || manifestSHA != expected {
			return FrozenCreate{}, fmt.Errorf("%w: manifest identity mismatch", ErrJobConflict)
		}
	} else if algorithm != "" || manifestSHA != "" {
		return FrozenCreate{}, ErrJobConflict
	}
	_, err = tx.Exec(ctx, `INSERT INTO node_job_executions(node_job_id,core_idempotency_key,resolved_template_path,requested_port,request_fingerprint,required_capability,template_manifest_algorithm,template_fingerprint_sha256)
		VALUES($1,$2,$3,$4,$5,$6,NULLIF($7,''),NULLIF($8,'')) ON CONFLICT (node_job_id) DO NOTHING`, jobID, key, template, port, fingerprint[:], requiredCapability, algorithm, manifestSHA)
	if err != nil {
		return FrozenCreate{}, err
	}
	var frozen FrozenCreate
	frozen.NodeJobID = jobID
	err = tx.QueryRow(ctx, `SELECT core_idempotency_key,resolved_template_path,requested_port,request_fingerprint,required_capability,
		COALESCE(template_manifest_algorithm,''),COALESCE(template_fingerprint_sha256,'')
        FROM node_job_executions WHERE node_job_id=$1`, jobID).
		Scan(&frozen.IdempotencyKey, &frozen.TemplatePath, &frozen.Port, &frozen.FingerprintSHA, &frozen.RequiredCapability, &frozen.TemplateManifestAlgorithm, &frozen.TemplateFingerprintSHA256)
	if err != nil {
		return FrozenCreate{}, err
	}
	if frozen.IdempotencyKey != key || frozen.TemplatePath != template || frozen.Port != port || frozen.RequiredCapability != requiredCapability || frozen.TemplateManifestAlgorithm != algorithm || frozen.TemplateFingerprintSHA256 != manifestSHA || !bytes.Equal(frozen.FingerprintSHA, fingerprint[:]) {
		return FrozenCreate{}, fmt.Errorf("%w: frozen create request changed", ErrJobConflict)
	}
	startedAt := time.Now().UTC()
	_, err = tx.Exec(ctx, `UPDATE allocations a SET state='creating',
		create_started_at=COALESCE(a.create_started_at,$2)
		FROM node_jobs j WHERE j.id=$1 AND j.allocation_id=a.id AND a.state IN ('reserved','create_unknown','creating')`,
		jobID, startedAt)
	if err != nil {
		return FrozenCreate{}, err
	}
	_, err = tx.Exec(ctx, `UPDATE server_requests r SET state='creating',updated_at=$2
		FROM allocations a JOIN node_jobs j ON j.allocation_id=a.id
		WHERE j.id=$1 AND a.server_request_id=r.id AND r.state='allocating'`, jobID, startedAt)
	if err != nil {
		return FrozenCreate{}, err
	}
	_, err = tx.Exec(ctx, `UPDATE validation_runs v SET state='create_observing',updated_at=$2
		FROM allocations a JOIN node_jobs j ON j.allocation_id=a.id
		WHERE j.id=$1 AND a.validation_run_id=v.id AND v.state='create_pending'`, jobID, startedAt)
	if err != nil {
		return FrozenCreate{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return FrozenCreate{}, err
	}
	return frozen, nil
}

func (s *Store) FrozenCreateForJob(ctx context.Context, nodeID, jobID string) (FrozenCreate, error) {
	var frozen FrozenCreate
	frozen.NodeJobID = jobID
	err := s.Pool.QueryRow(ctx, `SELECT e.core_idempotency_key,e.resolved_template_path,e.requested_port,e.request_fingerprint,
		e.required_capability,COALESCE(e.template_manifest_algorithm,''),COALESCE(e.template_fingerprint_sha256,'')
        FROM node_job_executions e JOIN node_jobs j ON j.id=e.node_job_id
        WHERE j.id=$1 AND j.node_id=$2`, jobID, nodeID).
		Scan(&frozen.IdempotencyKey, &frozen.TemplatePath, &frozen.Port, &frozen.FingerprintSHA,
			&frozen.RequiredCapability, &frozen.TemplateManifestAlgorithm, &frozen.TemplateFingerprintSHA256)
	if errors.Is(err, pgx.ErrNoRows) {
		return FrozenCreate{}, pgx.ErrNoRows
	}
	return frozen, err
}
