package store

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

type FrozenCreate struct {
	NodeJobID      string `json:"nodeJobId"`
	IdempotencyKey string `json:"idempotencyKey"`
	TemplatePath   string `json:"template"`
	Port           int    `json:"port"`
	FingerprintSHA []byte `json:"-"`
}

func CoreIdempotencyKey(nodeJobID string) string {
	return "nodejob-" + strings.ReplaceAll(nodeJobID, "-", "")
}

func createFingerprint(key, template string, port int) [32]byte {
	request, _ := json.Marshal(struct {
		IdempotencyKey string `json:"idempotencyKey"`
		Template       string `json:"template"`
		Port           int    `json:"port"`
	}{key, template, port})
	return sha256.Sum256(request)
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

// PrepareCreate persists every d2core create parameter before the first core
// call. A repeated prepare must match the previously frozen request exactly.
func (s *Store) PrepareCreate(ctx context.Context, nodeID, jobID, template string, port int) (FrozenCreate, error) {
	if template == "" || port < 0 || port > 65535 {
		return FrozenCreate{}, fmt.Errorf("invalid create request")
	}
	key := CoreIdempotencyKey(jobID)
	fingerprint := createFingerprint(key, template, port)
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return FrozenCreate{}, err
	}
	defer tx.Rollback(ctx)
	var kind, state string
	err = tx.QueryRow(ctx, "SELECT kind,state FROM node_jobs WHERE id=$1 AND node_id=$2 FOR UPDATE", jobID, nodeID).Scan(&kind, &state)
	if err != nil {
		return FrozenCreate{}, err
	}
	if kind != "create" || state == "pending" || state == "succeeded" || state == "rejected_no_effect" || state == "failed_with_effect" {
		return FrozenCreate{}, fmt.Errorf("job cannot be prepared in state %s", state)
	}
	_, err = tx.Exec(ctx, `INSERT INTO node_job_executions(node_job_id,core_idempotency_key,resolved_template_path,requested_port,request_fingerprint)
        VALUES($1,$2,$3,$4,$5) ON CONFLICT (node_job_id) DO NOTHING`, jobID, key, template, port, fingerprint[:])
	if err != nil {
		return FrozenCreate{}, err
	}
	var frozen FrozenCreate
	frozen.NodeJobID = jobID
	err = tx.QueryRow(ctx, `SELECT core_idempotency_key,resolved_template_path,requested_port,request_fingerprint
        FROM node_job_executions WHERE node_job_id=$1`, jobID).
		Scan(&frozen.IdempotencyKey, &frozen.TemplatePath, &frozen.Port, &frozen.FingerprintSHA)
	if err != nil {
		return FrozenCreate{}, err
	}
	if frozen.IdempotencyKey != key || frozen.TemplatePath != template || frozen.Port != port || !bytes.Equal(frozen.FingerprintSHA, fingerprint[:]) {
		return FrozenCreate{}, errors.New("frozen create request conflict")
	}
	if err := tx.Commit(ctx); err != nil {
		return FrozenCreate{}, err
	}
	return frozen, nil
}

func (s *Store) FrozenCreateForJob(ctx context.Context, nodeID, jobID string) (FrozenCreate, error) {
	var frozen FrozenCreate
	frozen.NodeJobID = jobID
	err := s.Pool.QueryRow(ctx, `SELECT e.core_idempotency_key,e.resolved_template_path,e.requested_port,e.request_fingerprint
        FROM node_job_executions e JOIN node_jobs j ON j.id=e.node_job_id
        WHERE j.id=$1 AND j.node_id=$2`, jobID, nodeID).
		Scan(&frozen.IdempotencyKey, &frozen.TemplatePath, &frozen.Port, &frozen.FingerprintSHA)
	if errors.Is(err, pgx.ErrNoRows) {
		return FrozenCreate{}, pgx.ErrNoRows
	}
	return frozen, err
}
