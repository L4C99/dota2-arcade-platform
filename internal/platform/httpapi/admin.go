package httpapi

import (
	"errors"
	"net/http"

	"github.com/L4C99/dota2-arcade-platform/internal/platform/store"
	"github.com/jackc/pgx/v5"
)

func (a *api) authenticatedAdmin(w http.ResponseWriter, r *http.Request) (string, bool) {
	cookie, err := r.Cookie(a.adminCookieName())
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return "", false
	}
	id, err := a.store.AdminForToken(r.Context(), cookie.Value)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return "", false
	}
	return id, true
}

func (a *api) adminOverview(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.authenticatedAdmin(w, r); !ok {
		return
	}
	overview, err := a.store.AdminOverview(r.Context())
	if err != nil {
		http.Error(w, "overview unavailable", http.StatusServiceUnavailable)
		return
	}
	writeJSON(w, http.StatusOK, overview)
}

func (a *api) adminAction(w http.ResponseWriter, r *http.Request) {
	if !a.checkOrigin(w, r) {
		return
	}
	adminID, ok := a.authenticatedAdmin(w, r)
	if !ok {
		return
	}
	var action store.AdminAction
	if err := decodeJSON(r, &action); err != nil {
		http.Error(w, "invalid action JSON", http.StatusBadRequest)
		return
	}
	if action.TargetID != "" && !nodeIDPattern.MatchString(action.TargetID) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"code": "invalid_target"})
		return
	}
	if action.GameID != "" && !nodeIDPattern.MatchString(action.GameID) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"code": "invalid_game"})
		return
	}
	err := a.store.ApplyAdminAction(r.Context(), adminID, action)
	switch {
	case errors.Is(err, store.ErrInvalidAdminAction):
		writeJSON(w, http.StatusBadRequest, map[string]string{"code": "invalid_action"})
	case errors.Is(err, store.ErrInvalidDesiredCapacity), errors.Is(err, store.ErrJobConflict):
		writeJSON(w, http.StatusConflict, map[string]string{"code": "action_conflict"})
	case errors.Is(err, pgx.ErrNoRows):
		http.Error(w, "target not found", http.StatusNotFound)
	case errors.Is(err, store.ErrInvalidCredentials):
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	case err != nil:
		http.Error(w, "action unavailable", http.StatusServiceUnavailable)
	default:
		writeJSON(w, http.StatusOK, map[string]string{"result": "succeeded"})
	}
}
