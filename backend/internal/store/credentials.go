package store

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"time"
)

var ErrCredentialNotFound = errors.New("batch credential not found")
var callerIDPattern = regexp.MustCompile(`^[a-z][a-z0-9._-]{0,63}$`)

type BatchCredential struct {
	ID         int64  `json:"id"`
	CallerID   string `json:"caller_id"`
	Label      string `json:"label"`
	TokenHint  string `json:"token_hint"`
	CreatedAt  int64  `json:"created_at"`
	LastUsedAt *int64 `json:"last_used_at"`
	RevokedAt  *int64 `json:"revoked_at"`
}

func ValidBatchCallerID(value string) bool { return callerIDPattern.MatchString(value) }

func (s Store) CreateBatchCredential(ctx context.Context, callerID, label string) (BatchCredential, string, error) {
	if !ValidBatchCallerID(callerID) || label == "" || len([]rune(label)) > 100 {
		return BatchCredential{}, "", errors.New("invalid caller ID or label")
	}
	var random [32]byte
	if _, err := rand.Read(random[:]); err != nil {
		return BatchCredential{}, "", err
	}
	token := "ncc_live_" + base64.RawURLEncoding.EncodeToString(random[:])
	sum := sha256.Sum256([]byte(token))
	hash := hex.EncodeToString(sum[:])
	hint := token[:17] + "…" + token[len(token)-6:]
	now := time.Now().UnixMilli()
	result, err := s.DB.ExecContext(ctx, `INSERT INTO batch_credentials
		(caller_id,label,token_hash,token_hint,created_at) VALUES (?,?,?,?,?)`, callerID, label, hash, hint, now)
	if err != nil {
		return BatchCredential{}, "", err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return BatchCredential{}, "", err
	}
	return BatchCredential{ID: id, CallerID: callerID, Label: label, TokenHint: hint, CreatedAt: now}, token, nil
}

func (s Store) ListBatchCredentials(ctx context.Context) ([]BatchCredential, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id,caller_id,label,token_hint,created_at,last_used_at,revoked_at
		FROM batch_credentials ORDER BY created_at DESC,id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]BatchCredential, 0)
	for rows.Next() {
		var item BatchCredential
		if err := rows.Scan(&item.ID, &item.CallerID, &item.Label, &item.TokenHint, &item.CreatedAt, &item.LastUsedAt, &item.RevokedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s Store) RevokeBatchCredential(ctx context.Context, id int64) error {
	result, err := s.DB.ExecContext(ctx, "UPDATE batch_credentials SET revoked_at=? WHERE id=? AND revoked_at IS NULL", time.Now().UnixMilli(), id)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return ErrCredentialNotFound
	}
	return nil
}

func (s Store) BatchCredentialsEnabled(ctx context.Context) (bool, error) {
	if s.DB == nil {
		return false, nil
	}
	var enabled bool
	err := s.DB.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM batch_credentials WHERE revoked_at IS NULL)").Scan(&enabled)
	return enabled, err
}

func (s Store) ResolveBatchCredential(ctx context.Context, token string) (string, bool, error) {
	if s.DB == nil {
		return "", false, errors.New("batch credential storage unavailable")
	}
	sum := sha256.Sum256([]byte(token))
	hash := hex.EncodeToString(sum[:])
	var caller string
	err := s.DB.QueryRowContext(ctx, `SELECT caller_id FROM batch_credentials WHERE token_hash=? AND revoked_at IS NULL`, hash).Scan(&caller)
	if err == nil {
		_, updateErr := s.DB.ExecContext(ctx, `UPDATE batch_credentials SET last_used_at=? WHERE token_hash=? AND revoked_at IS NULL AND (last_used_at IS NULL OR last_used_at<?)`, time.Now().UnixMilli(), hash, time.Now().Add(-time.Minute).UnixMilli())
		return caller, true, updateErr
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", false, fmt.Errorf("resolve batch credential: %w", err)
	}
	enabled, countErr := s.BatchCredentialsEnabled(ctx)
	return "", enabled, countErr
}
