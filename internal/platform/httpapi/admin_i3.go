package httpapi

import (
	"errors"
	"net/http"

	"github.com/L4C99/dota2-arcade-platform/internal/platform/store"
	"github.com/jackc/pgx/v5"
)

func i3Error(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrInvalidAdminAction):
		writeJSON(w, http.StatusBadRequest, map[string]string{"code": "INVALID_ACTION"})
	case errors.Is(err, store.ErrCASConflict):
		writeJSON(w, http.StatusConflict, map[string]string{"code": "CAS_CONFLICT"})
	case errors.Is(err, store.ErrValidationIncomplete):
		writeJSON(w, http.StatusConflict, map[string]string{"code": "VALIDATION_INCOMPLETE"})
	case errors.Is(err, store.ErrScopedResourcesActive):
		writeJSON(w, http.StatusConflict, map[string]string{"code": "SCOPED_RESOURCES_ACTIVE"})
	case errors.Is(err, store.ErrCapacityFull):
		writeJSON(w, http.StatusConflict, map[string]string{"code": "CAPACITY_FULL"})
	case errors.Is(err, store.ErrContentFactMismatch):
		writeJSON(w, http.StatusConflict, map[string]string{"code": "CONTENT_FACT_MISMATCH"})
	case errors.Is(err, store.ErrTemplateFactMismatch):
		writeJSON(w, http.StatusConflict, map[string]string{"code": "TEMPLATE_FACT_MISMATCH"})
	case errors.Is(err, store.ErrInventoryUnknown):
		writeJSON(w, http.StatusConflict, map[string]string{"code": "INVENTORY_UNKNOWN"})
	case errors.Is(err, store.ErrUnaccountedInstance):
		writeJSON(w, http.StatusConflict, map[string]string{"code": "UNACCOUNTED_INSTANCE"})
	case errors.Is(err, store.ErrJobConflict):
		writeJSON(w, http.StatusConflict, map[string]string{"code": "ACTION_CONFLICT"})
	case errors.Is(err, pgx.ErrNoRows):
		writeJSON(w, http.StatusNotFound, map[string]string{"code": "NOT_FOUND"})
	case errors.Is(err, store.ErrInvalidCredentials):
		writeJSON(w, http.StatusUnauthorized, map[string]string{"code": "UNAUTHORIZED"})
	default:
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"code": "ACTION_UNAVAILABLE"})
	}
}

func (a *api) adminMaintenanceBegin(w http.ResponseWriter, r *http.Request) {
	if !a.checkOrigin(w, r) {
		return
	}
	adminID, ok := a.authenticatedAdmin(w, r)
	if !ok {
		return
	}
	var input struct {
		NodeID                   string `json:"nodeId"`
		GameID                   string `json:"gameId"`
		ExpectedMaintenanceEpoch int64  `json:"expectedMaintenanceEpoch"`
		RequestID                string `json:"requestId"`
		Reason                   string `json:"reason"`
	}
	if decodeJSON(r, &input) != nil || !nodeIDPattern.MatchString(input.NodeID) || !nodeIDPattern.MatchString(input.GameID) || input.RequestID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"code": "INVALID_ACTION"})
		return
	}
	epoch, err := a.store.BeginScopedMaintenance(r.Context(), input.NodeID, input.GameID, adminID, input.RequestID, input.Reason, input.ExpectedMaintenanceEpoch)
	if err != nil {
		i3Error(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"maintenanceEpoch": epoch})
}

func (a *api) adminMaintenanceStatus(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.authenticatedAdmin(w, r); !ok {
		return
	}
	nodeID, gameID := r.URL.Query().Get("nodeId"), r.URL.Query().Get("gameId")
	if !nodeIDPattern.MatchString(nodeID) || !nodeIDPattern.MatchString(gameID) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"code": "INVALID_ACTION"})
		return
	}
	status, err := a.store.ScopedMaintenanceStatus(r.Context(), nodeID, gameID)
	if err != nil {
		i3Error(w, err)
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (a *api) adminValidationStart(w http.ResponseWriter, r *http.Request) {
	if !a.checkOrigin(w, r) {
		return
	}
	adminID, ok := a.authenticatedAdmin(w, r)
	if !ok {
		return
	}
	var input struct {
		NodeID                      string `json:"nodeId"`
		GameID                      string `json:"gameId"`
		PresetID                    string `json:"presetId"`
		ContentVersionID            string `json:"contentVersionId"`
		TemplateRevisionID          string `json:"templateRevisionId"`
		ExpectedMaintenanceEpoch    int64  `json:"expectedMaintenanceEpoch"`
		ExpectedTemplateFingerprint string `json:"expectedTemplateFingerprint"`
		RequestID                   string `json:"requestId"`
	}
	if decodeJSON(r, &input) != nil || !nodeIDPattern.MatchString(input.NodeID) || !nodeIDPattern.MatchString(input.GameID) || !nodeIDPattern.MatchString(input.PresetID) || input.RequestID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"code": "INVALID_ACTION"})
		return
	}
	run, err := a.store.ReserveValidation(r.Context(), store.ValidationCandidate{NodeID: input.NodeID, GameID: input.GameID, PresetID: input.PresetID,
		ContentVersionID: input.ContentVersionID, TemplateRevisionID: input.TemplateRevisionID, AdminID: adminID,
		ExpectedMaintenanceEpoch: input.ExpectedMaintenanceEpoch, ExpectedTemplateFingerprintSHA256: input.ExpectedTemplateFingerprint, RequestID: input.RequestID})
	if err != nil {
		i3Error(w, err)
		return
	}
	writeJSON(w, http.StatusOK, run)
}

func (a *api) adminValidationConfirm(w http.ResponseWriter, r *http.Request) {
	if !a.checkOrigin(w, r) {
		return
	}
	adminID, ok := a.authenticatedAdmin(w, r)
	if !ok {
		return
	}
	id := r.PathValue("id")
	var input struct {
		Result           string `json:"result"`
		Confirmed        bool   `json:"confirmed"`
		ExpectedRunState string `json:"expectedRunState"`
	}
	if !nodeIDPattern.MatchString(id) || decodeJSON(r, &input) != nil || !input.Confirmed || input.ExpectedRunState == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"code": "INVALID_ACTION"})
		return
	}
	run, err := a.store.ConfirmValidationHuman(r.Context(), id, adminID, input.Result, input.ExpectedRunState)
	if err != nil {
		i3Error(w, err)
		return
	}
	writeJSON(w, http.StatusOK, run)
}

func (a *api) adminValidationStop(w http.ResponseWriter, r *http.Request) {
	if !a.checkOrigin(w, r) {
		return
	}
	adminID, ok := a.authenticatedAdmin(w, r)
	if !ok {
		return
	}
	id := r.PathValue("id")
	var input struct {
		ExpectedRunState string `json:"expectedRunState"`
	}
	if !nodeIDPattern.MatchString(id) || decodeJSON(r, &input) != nil || input.ExpectedRunState == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"code": "INVALID_ACTION"})
		return
	}
	run, err := a.store.StopValidation(r.Context(), id, adminID, input.ExpectedRunState)
	if err != nil {
		i3Error(w, err)
		return
	}
	writeJSON(w, http.StatusOK, run)
}

func (a *api) adminValidationDetail(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.authenticatedAdmin(w, r); !ok {
		return
	}
	id := r.PathValue("id")
	if !nodeIDPattern.MatchString(id) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"code": "INVALID_ACTION"})
		return
	}
	run, err := a.store.ValidationDetail(r.Context(), id)
	if err != nil {
		i3Error(w, err)
		return
	}
	writeJSON(w, http.StatusOK, run)
}

func (a *api) adminReleasePublish(w http.ResponseWriter, r *http.Request) {
	if !a.checkOrigin(w, r) {
		return
	}
	adminID, ok := a.authenticatedAdmin(w, r)
	if !ok {
		return
	}
	var input store.ReleasePublishRequest
	if decodeJSON(r, &input) != nil || !nodeIDPattern.MatchString(input.GameID) ||
		(input.RollbackOfReleaseID != "" && !nodeIDPattern.MatchString(input.RollbackOfReleaseID)) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"code": "INVALID_ACTION"})
		return
	}
	for _, p := range input.Presets {
		if !nodeIDPattern.MatchString(p.PresetID) || (p.ValidationRunID != "" && !nodeIDPattern.MatchString(p.ValidationRunID)) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"code": "INVALID_ACTION"})
			return
		}
	}
	id, err := a.store.PublishRelease(r.Context(), adminID, input)
	if err != nil {
		i3Error(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"releaseId": id})
}

func (a *api) adminReleaseDetail(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.authenticatedAdmin(w, r); !ok {
		return
	}
	id := r.PathValue("id")
	if !nodeIDPattern.MatchString(id) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"code": "INVALID_ACTION"})
		return
	}
	info, err := a.store.ReleaseDetail(r.Context(), id)
	if err != nil {
		i3Error(w, err)
		return
	}
	writeJSON(w, http.StatusOK, info)
}
