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

func (a *api) createPlayerRequest(w http.ResponseWriter, r *http.Request) {
	if !a.checkOrigin(w, r) {
		return
	}
	userID, ok := a.playerUser(w, r)
	if !ok {
		return
	}
	var input struct {
		ArcadeGameID string `json:"arcadeGameId"`
		GamePresetID string `json:"gamePresetId"`
	}
	if err := decodeJSON(r, &input); err != nil || !nodeIDPattern.MatchString(input.ArcadeGameID) || !nodeIDPattern.MatchString(input.GamePresetID) {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	request, created, err := a.store.CreateUserRequest(r.Context(), userID, input.ArcadeGameID, input.GamePresetID)
	var maintenance *store.MaintenanceError
	switch {
	case errors.As(err, &maintenance):
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"code": "maintenance", "scope": maintenance.Scope, "message": maintenance.Message})
	case errors.Is(err, store.ErrInvalidSelection):
		http.Error(w, "game or preset unavailable", http.StatusNotFound)
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
