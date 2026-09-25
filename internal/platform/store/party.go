package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/L4C99/dota2-arcade-platform/internal/platform/auth"
	"github.com/jackc/pgx/v5"
)

var (
	ErrAlreadyInParty     = errors.New("user already belongs to a party")
	ErrPartyFull          = errors.New("party is full")
	ErrPartySizeUnset     = errors.New("max_party_size is not configured")
	ErrPartyForbidden     = errors.New("party action forbidden")
	ErrPartyBusy          = errors.New("party has an active server request or allocation")
	ErrInvalidInvite      = errors.New("invalid party invite")
	ErrCannotRemoveLeader = errors.New("leader cannot be removed")
	ErrUserBusy           = errors.New("user has a blocking solo request")
)

type PartyMember struct {
	UserID   string    `json:"userId"`
	Role     string    `json:"role"`
	JoinedAt time.Time `json:"joinedAt"`
}

type Party struct {
	ID           string        `json:"id"`
	LeaderUserID string        `json:"leaderUserId"`
	CurrentRole  string        `json:"currentRole"`
	MaxSize      int           `json:"maxSize"`
	Members      []PartyMember `json:"members"`
	CreatedAt    time.Time     `json:"createdAt"`
}

type PartyInvite struct {
	PartyID string `json:"partyId"`
	Token   string `json:"token"`
}

// ConfigurePartySize records an explicit deployment choice. It is deliberately
// separate from migration and from every GamePreset's max_players.
func (s *Store) ConfigurePartySize(ctx context.Context, size int) error {
	if size <= 0 {
		return fmt.Errorf("PLATFORM_MAX_PARTY_SIZE must be a positive integer")
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `UPDATE platform_settings SET max_party_size=$1 WHERE singleton=true`, size); err != nil {
		return err
	}
	var largest int
	if err := tx.QueryRow(ctx, `SELECT COALESCE(MAX(member_count),0) FROM
		(SELECT count(*) AS member_count FROM party_members GROUP BY party_id) counts`).Scan(&largest); err != nil {
		return err
	}
	if largest > size {
		return fmt.Errorf("max_party_size cannot be less than a current Party's membership")
	}
	return tx.Commit(ctx)
}

func partySize(ctx context.Context, tx pgx.Tx) (int, error) {
	var size *int
	if err := tx.QueryRow(ctx, `SELECT max_party_size FROM platform_settings WHERE singleton=true FOR SHARE`).Scan(&size); err != nil {
		return 0, err
	}
	if size == nil {
		return 0, ErrPartySizeUnset
	}
	return *size, nil
}

func lockUser(ctx context.Context, tx pgx.Tx, userID string) error {
	var id string
	return tx.QueryRow(ctx, `SELECT id FROM users WHERE id=$1 FOR UPDATE`, userID).Scan(&id)
}

func memberParty(ctx context.Context, tx pgx.Tx, userID string) (string, error) {
	var id string
	err := tx.QueryRow(ctx, `SELECT party_id FROM party_members WHERE user_id=$1`, userID).Scan(&id)
	return id, err
}

func blockingSolo(ctx context.Context, tx pgx.Tx, userID string) (bool, error) {
	var busy bool
	err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM server_requests WHERE owner_user_id=$1
		AND state IN ('waiting','allocating','creating','running','stopping','failed_unreclaimed','quarantined'))`, userID).Scan(&busy)
	return busy, err
}

func (s *Store) CurrentParty(ctx context.Context, userID string) (*Party, error) {
	var p Party
	err := s.Pool.QueryRow(ctx, `SELECT p.id,p.leader_user_id,p.created_at,s.max_party_size
		FROM party_members m JOIN parties p ON p.id=m.party_id
		JOIN platform_settings s ON s.singleton=true
		WHERE m.user_id=$1 AND p.dissolved_at IS NULL`, userID).
		Scan(&p.ID, &p.LeaderUserID, &p.CreatedAt, &p.MaxSize)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if p.LeaderUserID == userID {
		p.CurrentRole = "leader"
	} else {
		p.CurrentRole = "member"
	}
	rows, err := s.Pool.Query(ctx, `SELECT user_id,joined_at FROM party_members WHERE party_id=$1 ORDER BY joined_at,user_id`, p.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	p.Members = []PartyMember{}
	for rows.Next() {
		var m PartyMember
		if err := rows.Scan(&m.UserID, &m.JoinedAt); err != nil {
			return nil, err
		}
		m.Role = "member"
		if m.UserID == p.LeaderUserID {
			m.Role = "leader"
		}
		p.Members = append(p.Members, m)
	}
	return &p, rows.Err()
}

func (s *Store) CreateParty(ctx context.Context, userID string) (string, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)
	if err := lockUser(ctx, tx, userID); err != nil {
		return "", err
	}
	if _, err := partySize(ctx, tx); err != nil {
		return "", err
	}
	if _, err := memberParty(ctx, tx, userID); err == nil {
		return "", ErrAlreadyInParty
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return "", err
	}
	if busy, err := blockingSolo(ctx, tx, userID); err != nil {
		return "", err
	} else if busy {
		return "", ErrUserBusy
	}
	id, err := NewID()
	if err != nil {
		return "", err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO parties(id,leader_user_id) VALUES($1,$2)`, id, userID); err != nil {
		return "", err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO party_members(party_id,user_id) VALUES($1,$2)`, id, userID); err != nil {
		return "", err
	}
	inviteID, err := NewID()
	if err != nil {
		return "", err
	}
	token, hash, err := auth.NewToken()
	if err != nil {
		return "", err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO party_invites(id,party_id,token_hash,token_value) VALUES($1,$2,$3,$4)`, inviteID, id, hash[:], token); err != nil {
		return "", err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", err
	}
	return id, nil
}

func (s *Store) CurrentInvite(ctx context.Context, userID string) (PartyInvite, error) {
	var invite PartyInvite
	err := s.Pool.QueryRow(ctx, `SELECT i.party_id,i.token_value FROM party_invites i
		JOIN parties p ON p.id=i.party_id JOIN party_members m ON m.party_id=p.id
		WHERE m.user_id=$1 AND p.leader_user_id=$1 AND p.dissolved_at IS NULL AND i.revoked_at IS NULL`, userID).
		Scan(&invite.PartyID, &invite.Token)
	return invite, err
}

// ResetInvite serializes with consumption on the Party row. Once this commits,
// any later consumer of the old token sees its revoked state.
func (s *Store) ResetInvite(ctx context.Context, userID string) (PartyInvite, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return PartyInvite{}, err
	}
	defer tx.Rollback(ctx)
	var partyID string
	err = tx.QueryRow(ctx, `SELECT p.id FROM parties p JOIN party_members m ON m.party_id=p.id
		WHERE m.user_id=$1 AND p.leader_user_id=$1 AND p.dissolved_at IS NULL FOR UPDATE OF p`, userID).Scan(&partyID)
	if errors.Is(err, pgx.ErrNoRows) {
		return PartyInvite{}, ErrPartyForbidden
	}
	if err != nil {
		return PartyInvite{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE party_invites SET revoked_at=now() WHERE party_id=$1 AND revoked_at IS NULL`, partyID); err != nil {
		return PartyInvite{}, err
	}
	id, err := NewID()
	if err != nil {
		return PartyInvite{}, err
	}
	token, hash, err := auth.NewToken()
	if err != nil {
		return PartyInvite{}, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO party_invites(id,party_id,token_hash,token_value) VALUES($1,$2,$3,$4)`, id, partyID, hash[:], token); err != nil {
		return PartyInvite{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return PartyInvite{}, err
	}
	return PartyInvite{PartyID: partyID, Token: token}, nil
}

func (s *Store) JoinParty(ctx context.Context, userID, token string) (string, error) {
	hash, valid := auth.TokenHash(token)
	if !valid {
		return "", ErrInvalidInvite
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)
	if err := lockUser(ctx, tx, userID); err != nil {
		return "", err
	}
	if _, err := memberParty(ctx, tx, userID); err == nil {
		return "", ErrAlreadyInParty
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return "", err
	}
	if busy, err := blockingSolo(ctx, tx, userID); err != nil {
		return "", err
	} else if busy {
		return "", ErrUserBusy
	}
	var partyID string
	err = tx.QueryRow(ctx, `SELECT party_id FROM party_invites WHERE token_hash=$1`, hash[:]).Scan(&partyID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrInvalidInvite
	}
	if err != nil {
		return "", err
	}
	var live bool
	err = tx.QueryRow(ctx, `SELECT dissolved_at IS NULL FROM parties WHERE id=$1 FOR UPDATE`, partyID).Scan(&live)
	if err != nil {
		return "", err
	}
	if !live {
		return "", ErrInvalidInvite
	}
	// Recheck after the Party lock: a concurrent reset may have invalidated it.
	var active bool
	err = tx.QueryRow(ctx, `SELECT revoked_at IS NULL FROM party_invites WHERE token_hash=$1 AND party_id=$2`, hash[:], partyID).Scan(&active)
	if err != nil {
		return "", err
	}
	if !active {
		return "", ErrInvalidInvite
	}
	size, err := partySize(ctx, tx)
	if err != nil {
		return "", err
	}
	var count int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM party_members WHERE party_id=$1`, partyID).Scan(&count); err != nil {
		return "", err
	}
	if count >= size {
		return "", ErrPartyFull
	}
	if _, err := tx.Exec(ctx, `INSERT INTO party_members(party_id,user_id) VALUES($1,$2)`, partyID, userID); err != nil {
		return "", err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", err
	}
	return partyID, nil
}

func partyBusy(ctx context.Context, tx pgx.Tx, partyID string) (bool, error) {
	var busy bool
	err := tx.QueryRow(ctx, `SELECT EXISTS (
		SELECT 1 FROM server_requests r WHERE r.owner_party_id=$1 AND
		(r.state IN ('waiting','allocating','creating','running','stopping','failed_unreclaimed','quarantined')
		 OR EXISTS (SELECT 1 FROM allocations a WHERE a.server_request_id=r.id
		 AND a.state NOT IN ('reclaimed','released_no_effect'))))`, partyID).Scan(&busy)
	return busy, err
}

func (s *Store) LeaveParty(ctx context.Context, userID string) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := lockUser(ctx, tx, userID); err != nil {
		return err
	}
	partyID, err := memberParty(ctx, tx, userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrPartyForbidden
	}
	if err != nil {
		return err
	}
	var leaderID string
	if err := tx.QueryRow(ctx, `SELECT leader_user_id FROM parties WHERE id=$1 AND dissolved_at IS NULL FOR UPDATE`, partyID).Scan(&leaderID); err != nil {
		return err
	}
	if leaderID == userID {
		return ErrPartyForbidden
	} // V1 has no leader transfer; leader uses idle disband.
	if _, err := tx.Exec(ctx, `DELETE FROM party_members WHERE party_id=$1 AND user_id=$2`, partyID, userID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) RemovePartyMember(ctx context.Context, leaderID, targetID string) error {
	if leaderID == targetID {
		return ErrCannotRemoveLeader
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := lockUser(ctx, tx, targetID); err != nil {
		return err
	}
	var partyID string
	err = tx.QueryRow(ctx, `SELECT p.id FROM parties p JOIN party_members m ON m.party_id=p.id
		WHERE m.user_id=$1 AND p.leader_user_id=$1 AND p.dissolved_at IS NULL FOR UPDATE OF p`, leaderID).Scan(&partyID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrPartyForbidden
	}
	if err != nil {
		return err
	}
	result, err := tx.Exec(ctx, `DELETE FROM party_members WHERE party_id=$1 AND user_id=$2`, partyID, targetID)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return ErrPartyForbidden
	}
	return tx.Commit(ctx)
}

func (s *Store) DisbandParty(ctx context.Context, userID string) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := lockUser(ctx, tx, userID); err != nil {
		return err
	}
	var partyID string
	err = tx.QueryRow(ctx, `SELECT p.id FROM parties p JOIN party_members m ON m.party_id=p.id
		WHERE m.user_id=$1 AND p.leader_user_id=$1 AND p.dissolved_at IS NULL FOR UPDATE OF p`, userID).Scan(&partyID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrPartyForbidden
	}
	if err != nil {
		return err
	}
	busy, err := partyBusy(ctx, tx, partyID)
	if err != nil {
		return err
	}
	if busy {
		return ErrPartyBusy
	}
	if _, err := tx.Exec(ctx, `UPDATE party_invites SET revoked_at=now() WHERE party_id=$1 AND revoked_at IS NULL`, partyID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE parties SET leader_user_id=NULL,dissolved_at=now() WHERE id=$1`, partyID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM party_members WHERE party_id=$1`, partyID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
