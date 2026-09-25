package httpapi

import (
	"errors"
	"net/http"

	"github.com/L4C99/dota2-arcade-platform/internal/platform/store"
	"github.com/jackc/pgx/v5"
)

func partyError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrAlreadyInParty):
		writeJSON(w, http.StatusConflict, map[string]string{"code": "already_in_party", "message": "你已经在一个队伍中。"})
	case errors.Is(err, store.ErrPartyFull):
		writeJSON(w, http.StatusConflict, map[string]string{"code": "party_full", "message": "队伍人数已达平台上限。"})
	case errors.Is(err, store.ErrPartySizeUnset):
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"code": "party_size_unset", "message": "队伍功能尚未配置人数上限。"})
	case errors.Is(err, store.ErrInvalidInvite):
		writeJSON(w, http.StatusNotFound, map[string]string{"code": "invalid_invite", "message": "邀请链接无效或已重置。"})
	case errors.Is(err, store.ErrPartyBusy):
		writeJSON(w, http.StatusConflict, map[string]string{"code": "party_busy", "message": "队伍当前有活动申请或服务器，队长需等完整回收后才能解散。"})
	case errors.Is(err, store.ErrUserBusy):
		writeJSON(w, http.StatusConflict, map[string]string{"code": "solo_request_busy", "message": "请先结束当前单人服务器，再加入或创建队伍。"})
	case errors.Is(err, store.ErrCannotRemoveLeader):
		writeJSON(w, http.StatusConflict, map[string]string{"code": "leader_cannot_leave", "message": "V1 不支持队长转让；队长只能在空闲时解散队伍。"})
	case errors.Is(err, store.ErrPartyForbidden), errors.Is(err, pgx.ErrNoRows):
		writeJSON(w, http.StatusForbidden, map[string]string{"code": "forbidden", "message": "你没有执行此队伍操作的权限。"})
	default:
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"code": "party_unavailable", "message": "队伍操作暂时不可用。"})
	}
}

func (a *api) currentParty(w http.ResponseWriter, r *http.Request) {
	userID, ok := a.playerUser(w, r)
	if !ok {
		return
	}
	party, err := a.store.CurrentParty(r.Context(), userID)
	if err != nil {
		partyError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, party)
}

func (a *api) createParty(w http.ResponseWriter, r *http.Request) {
	if !a.checkOrigin(w, r) {
		return
	}
	userID, ok := a.playerUser(w, r)
	if !ok {
		return
	}
	if _, err := a.store.CreateParty(r.Context(), userID); err != nil {
		partyError(w, err)
		return
	}
	party, err := a.store.CurrentParty(r.Context(), userID)
	if err != nil {
		partyError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, party)
}

func (a *api) currentPartyInvite(w http.ResponseWriter, r *http.Request) {
	userID, ok := a.playerUser(w, r)
	if !ok {
		return
	}
	invite, err := a.store.CurrentInvite(r.Context(), userID)
	if err != nil {
		partyError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, invite)
}

func (a *api) resetPartyInvite(w http.ResponseWriter, r *http.Request) {
	if !a.checkOrigin(w, r) {
		return
	}
	userID, ok := a.playerUser(w, r)
	if !ok {
		return
	}
	invite, err := a.store.ResetInvite(r.Context(), userID)
	if err != nil {
		partyError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, invite)
}

func (a *api) joinParty(w http.ResponseWriter, r *http.Request) {
	if !a.checkOrigin(w, r) {
		return
	}
	userID, ok := a.playerUser(w, r)
	if !ok {
		return
	}
	var input struct {
		Token string `json:"token"`
	}
	if err := decodeJSON(r, &input); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	if _, err := a.store.JoinParty(r.Context(), userID, input.Token); err != nil {
		partyError(w, err)
		return
	}
	party, err := a.store.CurrentParty(r.Context(), userID)
	if err != nil {
		partyError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, party)
}

func (a *api) leaveParty(w http.ResponseWriter, r *http.Request) {
	if !a.checkOrigin(w, r) {
		return
	}
	userID, ok := a.playerUser(w, r)
	if !ok {
		return
	}
	if err := a.store.LeaveParty(r.Context(), userID); err != nil {
		partyError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *api) removePartyMember(w http.ResponseWriter, r *http.Request) {
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
	if err := a.store.RemovePartyMember(r.Context(), userID, id); err != nil {
		partyError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *api) disbandParty(w http.ResponseWriter, r *http.Request) {
	if !a.checkOrigin(w, r) {
		return
	}
	userID, ok := a.playerUser(w, r)
	if !ok {
		return
	}
	if err := a.store.DisbandParty(r.Context(), userID); err != nil {
		partyError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
