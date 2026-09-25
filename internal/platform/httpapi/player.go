package httpapi

import (
	"errors"
	"net/http"

	"github.com/L4C99/dota2-arcade-platform/internal/platform/store"
	"github.com/jackc/pgx/v5"
)

func (a *api) playerUser(w http.ResponseWriter, r *http.Request) (string, bool) {
	cookie, err := r.Cookie(a.userCookieName())
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return "", false
	}
	id, err := a.store.UserForToken(r.Context(), cookie.Value)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return "", false
	}
	return id, true
}

func (a *api) playerCatalog(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.playerUser(w, r); !ok {
		return
	}
	c, err := a.store.PlayerCatalog(r.Context())
	if err != nil {
		http.Error(w, "catalog unavailable", http.StatusServiceUnavailable)
		return
	}
	writeJSON(w, http.StatusOK, c)
}

func (a *api) playerNodes(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.playerUser(w, r); !ok {
		return
	}
	gameID, presetID := r.URL.Query().Get("arcadeGameId"), r.URL.Query().Get("gamePresetId")
	if !nodeIDPattern.MatchString(gameID) || !nodeIDPattern.MatchString(presetID) {
		http.Error(w, "invalid selection", http.StatusBadRequest)
		return
	}
	nodes, err := a.store.PlayerNodeChoices(r.Context(), gameID, presetID)
	if errors.Is(err, store.ErrInvalidSelection) {
		http.Error(w, "game or preset unavailable", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "nodes unavailable", http.StatusServiceUnavailable)
		return
	}
	writeJSON(w, http.StatusOK, nodes)
}

func (a *api) createPlayerRequest(w http.ResponseWriter, r *http.Request) {
	if !a.checkOrigin(w, r) {
		return
	}
	userID, ok := a.playerUser(w, r)
	if !ok {
		return
	}
	var input struct {
		ArcadeGameID      string `json:"arcadeGameId"`
		GamePresetID      string `json:"gamePresetId"`
		NodeSelectionMode string `json:"nodeSelectionMode"`
		ManualNodeID      string `json:"manualNodeId"`
	}
	if err := decodeJSON(r, &input); err != nil || !nodeIDPattern.MatchString(input.ArcadeGameID) || !nodeIDPattern.MatchString(input.GamePresetID) {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	if input.NodeSelectionMode == "" {
		input.NodeSelectionMode = "auto"
	}
	if input.ManualNodeID != "" && !nodeIDPattern.MatchString(input.ManualNodeID) {
		http.Error(w, "invalid node selection", http.StatusBadRequest)
		return
	}
	request, created, err := a.store.CreateUserRequestSelected(r.Context(), userID,
		input.ArcadeGameID, input.GamePresetID, input.NodeSelectionMode, input.ManualNodeID)
	var maintenance *store.MaintenanceError
	switch {
	case errors.As(err, &maintenance):
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"code": "maintenance", "scope": maintenance.Scope, "message": maintenance.Message})
	case errors.Is(err, store.ErrInvalidSelection):
		http.Error(w, "game or preset unavailable", http.StatusNotFound)
	case errors.Is(err, store.ErrInvalidNodeSelection):
		http.Error(w, "invalid node selection", http.StatusBadRequest)
	case errors.Is(err, store.ErrPartyForbidden):
		writeJSON(w, http.StatusForbidden, map[string]string{"code": "leader_required", "message": "只有队长可以为队伍申请服务器。"})
	case errors.Is(err, store.ErrPresetPartyTooLarge):
		writeJSON(w, http.StatusConflict, map[string]string{"code": "party_exceeds_preset", "message": "队伍人数超过该玩法的人数上限，请选择其他玩法。"})
	case err != nil:
		http.Error(w, "request unavailable", http.StatusServiceUnavailable)
	default:
		status := http.StatusOK
		if created {
			status = http.StatusCreated
		}
		writeJSON(w, status, request)
	}
}

func (a *api) cancelPlayerRequest(w http.ResponseWriter, r *http.Request) {
	if !a.checkOrigin(w, r) {
		return
	}
	userID, ok := a.playerUser(w, r)
	if !ok {
		return
	}
	id := r.PathValue("id")
	if !nodeIDPattern.MatchString(id) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	request, err := a.store.CancelUserRequest(r.Context(), userID, id)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		http.Error(w, "not found", http.StatusNotFound)
	case errors.Is(err, store.ErrPartyForbidden):
		writeJSON(w, http.StatusForbidden, map[string]string{"code": "leader_required", "message": "只有队长可以取消队伍申请。"})
	case errors.Is(err, store.ErrJobConflict):
		writeJSON(w, http.StatusConflict, map[string]string{"code": "cancel_unsafe", "message": "已开始分配或存在历史分配，不能通过取消释放资源。"})
	case err != nil:
		http.Error(w, "cancel unavailable", http.StatusServiceUnavailable)
	default:
		writeJSON(w, http.StatusOK, request)
	}
}

func (a *api) currentPlayerRequest(w http.ResponseWriter, r *http.Request) {
	userID, ok := a.playerUser(w, r)
	if !ok {
		return
	}
	request, err := a.store.CurrentUserRequest(r.Context(), userID)
	if err != nil {
		http.Error(w, "request unavailable", http.StatusServiceUnavailable)
		return
	}
	writeJSON(w, http.StatusOK, request)
}

func (a *api) playerRequest(w http.ResponseWriter, r *http.Request) {
	userID, ok := a.playerUser(w, r)
	if !ok {
		return
	}
	id := r.PathValue("id")
	if !nodeIDPattern.MatchString(id) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	request, err := a.store.UserRequest(r.Context(), userID, id)
	if errors.Is(err, pgx.ErrNoRows) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "request unavailable", http.StatusServiceUnavailable)
		return
	}
	writeJSON(w, http.StatusOK, request)
}

func (a *api) stopPlayerRequest(w http.ResponseWriter, r *http.Request) {
	if !a.checkOrigin(w, r) {
		return
	}
	userID, ok := a.playerUser(w, r)
	if !ok {
		return
	}
	id := r.PathValue("id")
	if !nodeIDPattern.MatchString(id) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	request, err := a.store.StopUserRequest(r.Context(), userID, id)
	if errors.Is(err, pgx.ErrNoRows) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if errors.Is(err, store.ErrPartyForbidden) {
		writeJSON(w, http.StatusForbidden, map[string]string{"code": "leader_required", "message": "只有队长可以结束队伍服务器。"})
		return
	}
	if errors.Is(err, store.ErrJobConflict) {
		http.Error(w, "request cannot be stopped in its current state", http.StatusConflict)
		return
	}
	if err != nil {
		http.Error(w, "stop unavailable", http.StatusServiceUnavailable)
		return
	}
	writeJSON(w, http.StatusOK, request)
}

func (a *api) abandonPlayerRequest(w http.ResponseWriter, r *http.Request) {
	if !a.checkOrigin(w, r) {
		return
	}
	userID, ok := a.playerUser(w, r)
	if !ok {
		return
	}
	id := r.PathValue("id")
	if !nodeIDPattern.MatchString(id) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	var input struct {
		Confirm bool `json:"confirm"`
	}
	if err := decodeJSON(r, &input); err != nil || !input.Confirm {
		writeJSON(w, http.StatusBadRequest, map[string]string{"code": "confirmation_required"})
		return
	}
	request, err := a.store.AbandonQuarantinedUserRequest(r.Context(), userID, id)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		http.Error(w, "not found", http.StatusNotFound)
	case errors.Is(err, store.ErrPartyForbidden):
		writeJSON(w, http.StatusForbidden, map[string]string{"code": "leader_required", "message": "只有队长可以放弃异常服务器。"})
	case errors.Is(err, store.ErrJobConflict):
		writeJSON(w, http.StatusConflict, map[string]string{"code": "not_quarantined", "message": "只有确认进入异常隔离的服务器才能放弃。"})
	case err != nil:
		http.Error(w, "abandon unavailable", http.StatusServiceUnavailable)
	default:
		writeJSON(w, http.StatusOK, request)
	}
}

func (a *api) nextGamePlayerRequest(w http.ResponseWriter, r *http.Request) {
	if !a.checkOrigin(w, r) {
		return
	}
	userID, ok := a.playerUser(w, r)
	if !ok {
		return
	}
	id := r.PathValue("id")
	if !nodeIDPattern.MatchString(id) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	request, err := a.store.NextGameUserRequest(r.Context(), userID, id)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		http.Error(w, "not found", http.StatusNotFound)
	case errors.Is(err, store.ErrPartyForbidden):
		writeJSON(w, http.StatusForbidden, map[string]string{"code": "leader_required", "message": "只有队长可以开始下一局。"})
	case errors.Is(err, store.ErrJobConflict):
		writeJSON(w, http.StatusConflict, map[string]string{"code": "next_game_unavailable", "message": "当前服务器状态无法开始下一局。"})
	case err != nil:
		http.Error(w, "next game unavailable", http.StatusServiceUnavailable)
	default:
		writeJSON(w, http.StatusOK, request)
	}
}

func (a *api) playerNextGameIntent(w http.ResponseWriter, r *http.Request) {
	userID, ok := a.playerUser(w, r)
	if !ok {
		return
	}
	id := r.PathValue("id")
	if !nodeIDPattern.MatchString(id) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	intent, err := a.store.UserNextGameIntent(r.Context(), userID, id)
	if errors.Is(err, pgx.ErrNoRows) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "next game unavailable", http.StatusServiceUnavailable)
		return
	}
	writeJSON(w, http.StatusOK, intent)
}

func (a *api) playerAllocation(w http.ResponseWriter, r *http.Request) {
	userID, ok := a.playerUser(w, r)
	if !ok {
		return
	}
	id := r.PathValue("id")
	if !nodeIDPattern.MatchString(id) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if _, err := a.store.UserRequest(r.Context(), userID, id); errors.Is(err, pgx.ErrNoRows) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	} else if err != nil {
		http.Error(w, "allocation unavailable", http.StatusServiceUnavailable)
		return
	}
	allocation, err := a.store.UserRequestAllocation(r.Context(), userID, id)
	if err != nil {
		http.Error(w, "allocation unavailable", http.StatusServiceUnavailable)
		return
	}
	writeJSON(w, http.StatusOK, allocation)
}
