package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/L4C99/dota2-arcade-platform/internal/platform/auth"
	"github.com/jackc/pgx/v5"
)

var ErrInvalidCredentials = errors.New("invalid credentials")

const (
	userSessionLifetime  = 90 * 24 * time.Hour
	adminSessionLifetime = 12 * time.Hour
)

func (s *Store) CreateUserSession(ctx context.Context) (userID, token string, err error) {
	userID, err = NewID()
	if err != nil {
		return "", "", err
	}
	sessionID, err := NewID()
	if err != nil {
		return "", "", err
	}
	token, tokenHash, err := auth.NewToken()
	if err != nil {
		return "", "", err
	}
	displayName, err := newDisplayName()
	if err != nil {
		return "", "", err
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return "", "", err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, "INSERT INTO users(id,display_name) VALUES($1,$2)", userID, displayName); err != nil {
		return "", "", err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO user_sessions(id,user_id,token_hash,expires_at)
        VALUES($1,$2,$3,now()+$4::interval)`, sessionID, userID, tokenHash[:], fmt.Sprintf("%d seconds", int64(userSessionLifetime.Seconds()))); err != nil {
		return "", "", err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", "", err
	}
	return userID, token, nil
}

func (s *Store) UserForToken(ctx context.Context, token string) (string, error) {
	hash, ok := auth.TokenHash(token)
	if !ok {
		return "", ErrInvalidCredentials
	}
	var userID string
	err := s.Pool.QueryRow(ctx, `SELECT user_id FROM user_sessions
        WHERE token_hash=$1 AND revoked_at IS NULL AND expires_at>now()`, hash[:]).Scan(&userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrInvalidCredentials
	}
	return userID, err
}

func (s *Store) RevokeUserToken(ctx context.Context, token string) error {
	hash, ok := auth.TokenHash(token)
	if !ok {
		return ErrInvalidCredentials
	}
	_, err := s.Pool.Exec(ctx, "UPDATE user_sessions SET revoked_at=now() WHERE token_hash=$1 AND revoked_at IS NULL", hash[:])
	return err
}

func (s *Store) CreateAdmin(ctx context.Context, username, password string) (string, error) {
	username = strings.ToLower(strings.TrimSpace(username))
	if username == "" || len(username) > 128 {
		return "", fmt.Errorf("invalid admin username")
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		return "", err
	}
	id, err := NewID()
	if err != nil {
		return "", err
	}
	_, err = s.Pool.Exec(ctx, "INSERT INTO admin_users(id,username,password_hash) VALUES($1,$2,$3)", id, username, hash)
	return id, err
}

func (s *Store) ResetAdminPassword(ctx context.Context, username, password string) error {
	hash, err := auth.HashPassword(password)
	if err != nil {
		return err
	}
	result, err := s.Pool.Exec(ctx, `UPDATE admin_users SET password_hash=$2,credential_version=credential_version+1,
        updated_at=now() WHERE username=$1`, strings.ToLower(strings.TrimSpace(username)), hash)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (s *Store) LoginAdmin(ctx context.Context, username, password string) (adminID, token string, err error) {
	var hash string
	var enabled bool
	var version int64
	username = strings.ToLower(strings.TrimSpace(username))
	err = s.Pool.QueryRow(ctx, `SELECT id,password_hash,enabled,credential_version FROM admin_users WHERE username=$1`, username).
		Scan(&adminID, &hash, &enabled, &version)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", ErrInvalidCredentials
	}
	if err != nil {
		return "", "", err
	}
	if !auth.VerifyPassword(hash, password) || !enabled {
		return "", "", ErrInvalidCredentials
	}
	sessionID, err := NewID()
	if err != nil {
		return "", "", err
	}
	token, tokenHash, err := auth.NewToken()
	if err != nil {
		return "", "", err
	}
	result, err := s.Pool.Exec(ctx, `INSERT INTO admin_sessions(id,admin_user_id,token_hash,credential_version,expires_at)
        SELECT $1,id,$2,credential_version,now()+$3::interval FROM admin_users
        WHERE id=$4 AND enabled AND credential_version=$5`, sessionID, tokenHash[:],
		fmt.Sprintf("%d seconds", int64(adminSessionLifetime.Seconds())), adminID, version)
	if err != nil {
		return "", "", err
	}
	if result.RowsAffected() != 1 {
		return "", "", ErrInvalidCredentials
	}
	return adminID, token, nil
}

func (s *Store) AdminForToken(ctx context.Context, token string) (string, error) {
	hash, ok := auth.TokenHash(token)
	if !ok {
		return "", ErrInvalidCredentials
	}
	var adminID string
	err := s.Pool.QueryRow(ctx, `SELECT a.id FROM admin_sessions sess JOIN admin_users a ON a.id=sess.admin_user_id
        WHERE sess.token_hash=$1 AND sess.revoked_at IS NULL AND sess.expires_at>now()
        AND a.enabled AND a.credential_version=sess.credential_version`, hash[:]).Scan(&adminID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrInvalidCredentials
	}
	return adminID, err
}

func (s *Store) RevokeAdminToken(ctx context.Context, token string) error {
	hash, ok := auth.TokenHash(token)
	if !ok {
		return ErrInvalidCredentials
	}
	_, err := s.Pool.Exec(ctx, "UPDATE admin_sessions SET revoked_at=now() WHERE token_hash=$1 AND revoked_at IS NULL", hash[:])
	return err
}
