package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

var ErrInvalidSelection = errors.New("invalid game or preset selection")

type MaintenanceError struct {
	Scope   string
	Message string
}

func (e *MaintenanceError) Error() string { return e.Scope + " is not accepting requests" }

type ArcadeGame struct {
	ID                   string `json:"id"`
	WorkshopID           string `json:"workshopId"`
	DisplayName          string `json:"displayName"`
	Enabled              bool   `json:"enabled"`
	AcceptingNewRequests bool   `json:"acceptingNewRequests"`
	MaintenanceMessage   string `json:"maintenanceMessage"`
}

type GamePreset struct {
	ID                   string `json:"id"`
	ArcadeGameID         string `json:"arcadeGameId"`
	DisplayName          string `json:"displayName"`
	Enabled              bool   `json:"enabled"`
	AcceptingNewRequests bool   `json:"acceptingNewRequests"`
	MaintenanceMessage   string `json:"maintenanceMessage"`
	MaxPlayers           int    `json:"maxPlayers"`
}

type Catalog struct {
	GlobalAcceptingNewRequests bool         `json:"globalAcceptingNewRequests"`
	GlobalMaintenanceMessage   string       `json:"globalMaintenanceMessage"`
	Games                      []ArcadeGame `json:"games"`
	Presets                    []GamePreset `json:"presets"`
}

func (s *Store) PlayerCatalog(ctx context.Context) (Catalog, error) {
	c := Catalog{Games: []ArcadeGame{}, Presets: []GamePreset{}}
	if err := s.Pool.QueryRow(ctx, `SELECT accepting_new_requests,maintenance_message
		FROM platform_settings WHERE singleton=true`).Scan(&c.GlobalAcceptingNewRequests, &c.GlobalMaintenanceMessage); err != nil {
		return Catalog{}, err
	}
	rows, err := s.Pool.Query(ctx, `SELECT id,workshop_id,display_name,enabled,accepting_new_requests,maintenance_message
		FROM arcade_games WHERE enabled ORDER BY created_at,id`)
	if err != nil {
		return Catalog{}, err
	}
	for rows.Next() {
		var g ArcadeGame
		if err := rows.Scan(&g.ID, &g.WorkshopID, &g.DisplayName, &g.Enabled, &g.AcceptingNewRequests, &g.MaintenanceMessage); err != nil {
			rows.Close()
			return Catalog{}, err
		}
		c.Games = append(c.Games, g)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return Catalog{}, err
	}
	rows.Close()
	rows, err = s.Pool.Query(ctx, `SELECT p.id,p.arcade_game_id,p.display_name,p.enabled,p.accepting_new_requests,p.maintenance_message,p.max_players
		FROM game_presets p JOIN arcade_games g ON g.id=p.arcade_game_id
		WHERE p.enabled AND g.enabled ORDER BY p.created_at,p.id`)
	if err != nil {
		return Catalog{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var p GamePreset
		if err := rows.Scan(&p.ID, &p.ArcadeGameID, &p.DisplayName, &p.Enabled, &p.AcceptingNewRequests, &p.MaintenanceMessage, &p.MaxPlayers); err != nil {
			return Catalog{}, err
		}
		c.Presets = append(c.Presets, p)
	}
	return c, rows.Err()
}

type ServerRequest struct {
	ID           string    `json:"id"`
	ArcadeGameID string    `json:"arcadeGameId"`
	GamePresetID string    `json:"gamePresetId"`
	State        string    `json:"state"`
	RequestedAt  time.Time `json:"requestedAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

type requestScanner interface{ Scan(...any) error }

func scanServerRequest(row requestScanner) (ServerRequest, error) {
	var r ServerRequest
	err := row.Scan(&r.ID, &r.ArcadeGameID, &r.GamePresetID, &r.State, &r.RequestedAt, &r.UpdatedAt)
	return r, err
}

const currentRequestSQL = `SELECT id,arcade_game_id,game_preset_id,state,requested_at,updated_at
	FROM server_requests WHERE owner_user_id=$1 AND state IN ('waiting','allocating','creating','running','stopping','failed_unreclaimed','quarantined')`

func (s *Store) CurrentUserRequest(ctx context.Context, userID string) (*ServerRequest, error) {
	r, err := scanServerRequest(s.Pool.QueryRow(ctx, currentRequestSQL, userID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &r, nil
}

func (s *Store) UserRequest(ctx context.Context, userID, requestID string) (ServerRequest, error) {
	return scanServerRequest(s.Pool.QueryRow(ctx, `SELECT id,arcade_game_id,game_preset_id,state,requested_at,updated_at
		FROM server_requests WHERE id=$1 AND owner_user_id=$2`, requestID, userID))
}

// The user row serializes submissions from every Session of the same owner.
// The partial unique index is a second, database-level guard against duplicates.
func (s *Store) CreateUserRequest(ctx context.Context, userID, gameID, presetID string) (ServerRequest, bool, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return ServerRequest{}, false, err
	}
	defer tx.Rollback(ctx)
	var lockedID string
	if err := tx.QueryRow(ctx, `SELECT id FROM users WHERE id=$1 FOR UPDATE`, userID).Scan(&lockedID); err != nil {
		return ServerRequest{}, false, err
	}
	if r, err := scanServerRequest(tx.QueryRow(ctx, currentRequestSQL, userID)); err == nil {
		return r, false, tx.Commit(ctx)
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return ServerRequest{}, false, err
	}
	var globalAccept, gameEnabled, gameAccept, presetEnabled, presetAccept bool
	var globalMessage, gameMessage, presetMessage string
	err = tx.QueryRow(ctx, `SELECT s.accepting_new_requests,s.maintenance_message,
		g.enabled,g.accepting_new_requests,g.maintenance_message,
		p.enabled,p.accepting_new_requests,p.maintenance_message
		FROM platform_settings s JOIN arcade_games g ON g.id=$1
		JOIN game_presets p ON p.id=$2 AND p.arcade_game_id=g.id
		WHERE s.singleton=true FOR SHARE OF s,g,p`, gameID, presetID).
		Scan(&globalAccept, &globalMessage, &gameEnabled, &gameAccept, &gameMessage, &presetEnabled, &presetAccept, &presetMessage)
	if errors.Is(err, pgx.ErrNoRows) {
		return ServerRequest{}, false, ErrInvalidSelection
	}
	if err != nil {
		return ServerRequest{}, false, err
	}
	if !globalAccept {
		return ServerRequest{}, false, &MaintenanceError{Scope: "global", Message: globalMessage}
	}
	if !gameEnabled || !gameAccept {
		return ServerRequest{}, false, &MaintenanceError{Scope: "arcadeGame", Message: gameMessage}
	}
	if !presetEnabled || !presetAccept {
		return ServerRequest{}, false, &MaintenanceError{Scope: "gamePreset", Message: presetMessage}
	}
	id, err := NewID()
	if err != nil {
		return ServerRequest{}, false, err
	}
	requestedAt := time.Now().UTC()
	r, err := scanServerRequest(tx.QueryRow(ctx, `INSERT INTO server_requests(id,owner_user_id,arcade_game_id,game_preset_id,requested_at,updated_at)
		VALUES($1,$2,$3,$4,$5,$5) RETURNING id,arcade_game_id,game_preset_id,state,requested_at,updated_at`, id, userID, gameID, presetID, requestedAt))
	if err != nil {
		return ServerRequest{}, false, err
	}
	return r, true, tx.Commit(ctx)
}
