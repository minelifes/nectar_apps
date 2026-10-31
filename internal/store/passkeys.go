package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

// Passkey is a WebAuthn credential of a user.
type Passkey struct {
	ID             int64      `json:"id"`
	UserID         int64      `json:"user_id"`
	CredentialID   []byte     `json:"-"`
	PublicKey      []byte     `json:"-"`
	SignCount      uint32     `json:"-"`
	AAGUID         []byte     `json:"-"`
	RPID           string     `json:"rp_id"`
	Name           string     `json:"name"`
	BackupEligible bool       `json:"synced"`
	Transports     []string   `json:"transports"`
	CreatedAt      time.Time  `json:"created_at"`
	LastUsedAt     *time.Time `json:"last_used_at,omitempty"`
}

const passkeyCols = `id, user_id, credential_id, public_key, sign_count, aaguid, rp_id, name, backup_eligible, transports, created_at, last_used_at`

func scanPasskey(sc interface{ Scan(...any) error }) (Passkey, error) {
	var p Passkey
	var tr string
	var created int64
	var used sql.NullInt64
	err := sc.Scan(&p.ID, &p.UserID, &p.CredentialID, &p.PublicKey, &p.SignCount, &p.AAGUID, &p.RPID, &p.Name, &p.BackupEligible, &tr, &created, &used)
	p.Transports = []string{}
	if tr != "" {
		p.Transports = strings.Split(tr, ",")
	}
	p.CreatedAt = ts(created)
	if used.Valid {
		t := ts(used.Int64)
		p.LastUsedAt = &t
	}
	return p, err
}

// AddPasskey stores a new passkey.
func (s *Store) AddPasskey(ctx context.Context, p Passkey) (Passkey, error) {
	res, err := s.db.ExecContext(ctx, `INSERT INTO passkeys (user_id, credential_id, public_key, sign_count, aaguid, rp_id, name, backup_eligible, transports, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, p.UserID, p.CredentialID, p.PublicKey, p.SignCount, p.AAGUID, p.RPID, p.Name, b2i(p.BackupEligible),
		strings.Join(p.Transports, ","), s.unix())
	if isUnique(err) {
		return p, ErrConflict
	}
	if err != nil {
		return p, err
	}
	id, _ := res.LastInsertId()
	return s.getPasskey(ctx, `id = ?`, id)
}

func (s *Store) getPasskey(ctx context.Context, where string, arg any) (Passkey, error) {
	p, err := scanPasskey(s.db.QueryRowContext(ctx, `SELECT `+passkeyCols+` FROM passkeys WHERE `+where, arg))
	if errors.Is(err, sql.ErrNoRows) {
		return p, ErrNotFound
	}
	return p, err
}

// PasskeyByCredential finds a passkey by its credential id.
func (s *Store) PasskeyByCredential(ctx context.Context, credID []byte) (Passkey, error) {
	return s.getPasskey(ctx, `credential_id = ?`, credID)
}

// ListPasskeys returns a user's passkeys, oldest first.
func (s *Store) ListPasskeys(ctx context.Context, userID int64) ([]Passkey, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+passkeyCols+` FROM passkeys WHERE user_id = ? ORDER BY id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Passkey{}
	for rows.Next() {
		p, err := scanPasskey(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// TouchPasskey records a sign-in with the new signature counter.
func (s *Store) TouchPasskey(ctx context.Context, id int64, count uint32) error {
	_, err := s.db.ExecContext(ctx, `UPDATE passkeys SET sign_count = ?, last_used_at = ? WHERE id = ?`, count, s.unix(), id)
	return err
}

// RenamePasskey changes the label of a user's passkey.
func (s *Store) RenamePasskey(ctx context.Context, userID, id int64, name string) error {
	res, err := s.db.ExecContext(ctx, `UPDATE passkeys SET name = ? WHERE id = ? AND user_id = ?`, name, id, userID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeletePasskeys removes one passkey of a user (id 0 = all of them, e.g. a lost phone).
func (s *Store) DeletePasskeys(ctx context.Context, userID, id int64) (int64, error) {
	q, args := `DELETE FROM passkeys WHERE user_id = ?`, []any{userID}
	if id != 0 {
		q, args = q+` AND id = ?`, append(args, id)
	}
	res, err := s.db.ExecContext(ctx, q, args...)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
