package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"
	"strings"

	"github.com/L4C99/dota2-arcade-platform/internal/contracts/nodev1"
	"github.com/L4C99/dota2-arcade-platform/internal/platform/store"
	"github.com/jackc/pgx/v5"
)

var nodeIDPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

func (a *api) authenticatedNode(w http.ResponseWriter, r *http.Request) (string, bool) {
	nodeID := r.Header.Get("X-Node-ID")
	secret, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !nodeIDPattern.MatchString(nodeID) || !ok || secret == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return "", false
	}
	if err := a.store.AuthenticateNode(r.Context(), nodeID, secret); err != nil {
		if errors.Is(err, store.ErrNodeUnauthorized) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
		} else {
			http.Error(w, "node authentication unavailable", http.StatusServiceUnavailable)
		}
		return "", false
	}
	return nodeID, true
}

func decodeNodeJSON(w http.ResponseWriter, r *http.Request, value any) error {
	if !strings.HasPrefix(strings.ToLower(r.Header.Get("Content-Type")), "application/json") {
		return errors.New("expected JSON")
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return errors.New("trailing JSON")
	}
	return nil
}

func (a *api) nodeHeartbeat(w http.ResponseWriter, r *http.Request) {
	nodeID, ok := a.authenticatedNode(w, r)
	if !ok {
		return
	}
	var heartbeat nodev1.Heartbeat
	if err := decodeNodeJSON(w, r, &heartbeat); err != nil {
		http.Error(w, "invalid heartbeat JSON", http.StatusBadRequest)
		return
	}
	if err := heartbeat.Validate(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	result, err := a.store.RecordHeartbeat(r.Context(), nodeID, heartbeat)
	if err != nil {
		http.Error(w, "heartbeat unavailable", http.StatusServiceUnavailable)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (a *api) nodeReconcileComplete(w http.ResponseWriter, r *http.Request) {
	nodeID, ok := a.authenticatedNode(w, r)
	if !ok {
		return
	}
	var input struct {
		Generation int64 `json:"generation"`
	}
	if err := decodeNodeJSON(w, r, &input); err != nil || input.Generation <= 0 {
		http.Error(w, "invalid reconcile generation", http.StatusBadRequest)
		return
	}
	if err := a.store.CompleteNodeReconcile(r.Context(), nodeID, input.Generation); err != nil {
		if errors.Is(err, store.ErrJobConflict) {
			http.Error(w, "stale reconcile generation", http.StatusConflict)
		} else {
			http.Error(w, "reconcile unavailable", http.StatusServiceUnavailable)
		}
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *api) nodeActiveAllocations(w http.ResponseWriter, r *http.Request) {
	nodeID, ok := a.authenticatedNode(w, r)
	if !ok {
		return
	}
	allocations, err := a.store.ActiveAllocationsForNode(r.Context(), nodeID)
	if err != nil {
		http.Error(w, "active allocations unavailable", http.StatusServiceUnavailable)
		return
	}
	writeJSON(w, http.StatusOK, allocations)
}

func (a *api) nodeInstanceFact(w http.ResponseWriter, r *http.Request) {
	nodeID, ok := a.authenticatedNode(w, r)
	if !ok {
		return
	}
	id := r.PathValue("id")
	if !nodeIDPattern.MatchString(id) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	var fact nodev1.InstanceFact
	if err := decodeNodeJSON(w, r, &fact); err != nil {
		http.Error(w, "invalid instance fact", http.StatusBadRequest)
		return
	}
	err := a.store.ReportInstanceFact(r.Context(), nodeID, id, fact)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		http.Error(w, "not found", http.StatusNotFound)
	case errors.Is(err, store.ErrJobConflict):
		http.Error(w, "invalid instance fact", http.StatusConflict)
	case err != nil:
		http.Error(w, "instance reconciliation unavailable", http.StatusServiceUnavailable)
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

func (a *api) nodeOpenJobs(w http.ResponseWriter, r *http.Request) {
	nodeID, ok := a.authenticatedNode(w, r)
	if !ok {
		return
	}
	jobs, err := a.store.OpenJobsForNode(r.Context(), nodeID)
	if err != nil {
		http.Error(w, "jobs unavailable", http.StatusServiceUnavailable)
		return
	}
	writeJSON(w, http.StatusOK, jobs)
}

func (a *api) nodeClaimJob(w http.ResponseWriter, r *http.Request) {
	nodeID, ok := a.authenticatedNode(w, r)
	if !ok {
		return
	}
	job, err := a.store.ClaimNextJob(r.Context(), nodeID)
	if err != nil {
		http.Error(w, "claim unavailable", http.StatusServiceUnavailable)
		return
	}
	if job == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	writeJSON(w, http.StatusOK, job)
}

func (a *api) nodeGetJob(w http.ResponseWriter, r *http.Request) {
	nodeID, ok := a.authenticatedNode(w, r)
	if !ok {
		return
	}
	job, err := a.store.JobForNode(r.Context(), nodeID, r.PathValue("id"))
	if errors.Is(err, pgx.ErrNoRows) {
		http.Error(w, "job not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "job unavailable", http.StatusServiceUnavailable)
		return
	}
	writeJSON(w, http.StatusOK, job)
}

func (a *api) nodePrepareJob(w http.ResponseWriter, r *http.Request) {
	nodeID, ok := a.authenticatedNode(w, r)
	if !ok {
		return
	}
	var request nodev1.PrepareCreateRequest
	if err := decodeNodeJSON(w, r, &request); err != nil {
		http.Error(w, "invalid prepare JSON", http.StatusBadRequest)
		return
	}
	frozen, err := a.store.PrepareCreate(r.Context(), nodeID, r.PathValue("id"), request.Template, request.Port)
	if errors.Is(err, pgx.ErrNoRows) {
		http.Error(w, "job not found", http.StatusNotFound)
		return
	}
	if errors.Is(err, store.ErrJobConflict) {
		http.Error(w, "prepare conflict", http.StatusConflict)
		return
	}
	if err != nil {
		http.Error(w, "prepare unavailable", http.StatusServiceUnavailable)
		return
	}
	writeJSON(w, http.StatusOK, frozen.Contract())
}

func (a *api) nodeReportJob(w http.ResponseWriter, r *http.Request) {
	nodeID, ok := a.authenticatedNode(w, r)
	if !ok {
		return
	}
	var request nodev1.ReportRequest
	if err := decodeNodeJSON(w, r, &request); err != nil {
		http.Error(w, "invalid report JSON", http.StatusBadRequest)
		return
	}
	if err := request.Validate(); err != nil {
		http.Error(w, "invalid report", http.StatusBadRequest)
		return
	}
	job, err := a.store.ReportJob(r.Context(), nodeID, r.PathValue("id"), request)
	if errors.Is(err, pgx.ErrNoRows) {
		http.Error(w, "job not found", http.StatusNotFound)
		return
	}
	if errors.Is(err, store.ErrJobConflict) {
		http.Error(w, "report conflict", http.StatusConflict)
		return
	}
	if err != nil {
		http.Error(w, "report unavailable", http.StatusServiceUnavailable)
		return
	}
	writeJSON(w, http.StatusOK, job)
}
