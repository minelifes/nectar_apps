package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

// User is a panel account.
type User struct {
	ID           int64     `json:"id"`
	Email        string    `json:"email"`
	Name         string    `json:"name"`
	PasswordHash string    `json:"-"`
	IsOwner      bool      `json:"is_owner"`
	Disabled     bool      `json:"disabled"`
	MFAEnabled   bool      `json:"mfa_enabled"`
	CreatedAt    time.Time `json:"created_at"`
}

// ErrLastOwner is returned when an operation would leave the installation without an active Owner.
var ErrLastOwner = errors.New("there must be at least one active owner")

const userCols = `id, email, name, password_hash, is_owner, disabled, created_at, mfa_enabled`

type scanner interface{ Scan(...any) error }

func scanUser(r scanner) (User, error) {
	var u User
	var created int64
	err := r.Scan(&u.ID, &u.Email, &u.Name, &u.PasswordHash, &u.IsOwner, &u.Disabled, &created, &u.MFAEnabled)
	if errors.Is(err, sql.ErrNoRows) {
		return u, ErrNotFound
	}
	u.CreatedAt = ts(created)
	return u, err
}

// NormalizeEmail lower-cases and trims an email address.
func NormalizeEmail(e string) string { return strings.ToLower(strings.TrimSpace(e)) }

// CountUsers returns the number of users.
func (s *Store) CountUsers(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&n)
	return n, err
}

// CreateUser creates a user. Returns ErrConflict if the email is taken.
func (s *Store) CreateUser(ctx context.Context, email, name, passwordHash string, owner bool) (User, error) {
	now := s.unix()
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO users (email, name, password_hash, is_owner, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
		NormalizeEmail(email), strings.TrimSpace(name), passwordHash, b2i(owner), now, now)
	if isUnique(err) {
		return User{}, ErrConflict
	}
	if err != nil {
		return User{}, err
	}
	id, _ := res.LastInsertId()
	return s.GetUser(ctx, id)
}

// CreateFirstOwner creates an Owner only if no users exist yet (atomic). Returns ErrConflict otherwise.
func (s *Store) CreateFirstOwner(ctx context.Context, email, name, passwordHash string) (User, error) {
	var id int64
	err := s.tx(ctx, func(tx *sql.Tx) error {
		var n int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&n); err != nil {
			return err
		}
		if n > 0 {
			return ErrConflict
		}
		now := s.unix()
		res, err := tx.ExecContext(ctx,
			`INSERT INTO users (email, name, password_hash, is_owner, created_at, updated_at) VALUES (?, ?, ?, 1, ?, ?)`,
			NormalizeEmail(email), strings.TrimSpace(name), passwordHash, now, now)
		if err != nil {
			return err
		}
		id, err = res.LastInsertId()
		return err
	})
	if err != nil {
		return User{}, err
	}
	return s.GetUser(ctx, id)
}

// GetUser returns a user by id.
func (s *Store) GetUser(ctx context.Context, id int64) (User, error) {
	return scanUser(s.db.QueryRowContext(ctx, `SELECT `+userCols+` FROM users WHERE id = ?`, id))
}

// GetUserByEmail returns a user by email (case-insensitive).
func (s *Store) GetUserByEmail(ctx context.Context, email string) (User, error) {
	return scanUser(s.db.QueryRowContext(ctx, `SELECT `+userCols+` FROM users WHERE email = ?`, NormalizeEmail(email)))
}

// ListUsers returns all users ordered by id.
func (s *Store) ListUsers(ctx context.Context) ([]User, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+userCols+` FROM users ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []User
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// UserUpdate holds optional changes; nil fields are left unchanged.
type UserUpdate struct {
	Name         *string
	Disabled     *bool
	PasswordHash *string
}

// UpdateUser applies changes. Disabling the last active owner returns ErrLastOwner.
func (s *Store) UpdateUser(ctx context.Context, id int64, upd UserUpdate) (User, error) {
	err := s.tx(ctx, func(tx *sql.Tx) error {
		u, err := scanUser(tx.QueryRowContext(ctx, `SELECT `+userCols+` FROM users WHERE id = ?`, id))
		if err != nil {
			return err
		}
		if upd.Name != nil {
			u.Name = strings.TrimSpace(*upd.Name)
		}
		if upd.PasswordHash != nil {
			u.PasswordHash = *upd.PasswordHash
		}
		if upd.Disabled != nil {
			if *upd.Disabled && !u.Disabled && u.IsOwner {
				if err := ensureOtherOwner(ctx, tx, id); err != nil {
					return err
				}
			}
			u.Disabled = *upd.Disabled
		}
		_, err = tx.ExecContext(ctx, `UPDATE users SET name = ?, password_hash = ?, disabled = ?, updated_at = ? WHERE id = ?`,
			u.Name, u.PasswordHash, b2i(u.Disabled), s.unix(), id)
		return err
	})
	if err != nil {
		return User{}, err
	}
	return s.GetUser(ctx, id)
}

// SetOwner grants or revokes the Owner flag. Revoking the last active owner returns ErrLastOwner.
func (s *Store) SetOwner(ctx context.Context, id int64, owner bool) error {
	return s.tx(ctx, func(tx *sql.Tx) error {
		if !owner {
			if err := ensureOtherOwner(ctx, tx, id); err != nil {
				return err
			}
		}
		res, err := tx.ExecContext(ctx, `UPDATE users SET is_owner = ?, updated_at = ? WHERE id = ?`, b2i(owner), s.unix(), id)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return ErrNotFound
		}
		return nil
	})
}

// DeleteUser deletes a user (sessions, bindings and tokens cascade). The last owner cannot be deleted.
func (s *Store) DeleteUser(ctx context.Context, id int64) error {
	return s.tx(ctx, func(tx *sql.Tx) error {
		u, err := scanUser(tx.QueryRowContext(ctx, `SELECT `+userCols+` FROM users WHERE id = ?`, id))
		if err != nil {
			return err
		}
		if u.IsOwner && !u.Disabled {
			if err := ensureOtherOwner(ctx, tx, id); err != nil {
				return err
			}
		}
		_, err = tx.ExecContext(ctx, `DELETE FROM users WHERE id = ?`, id)
		return err
	})
}

func ensureOtherOwner(ctx context.Context, tx *sql.Tx, exceptID int64) error {
	var n int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM users WHERE is_owner = 1 AND disabled = 0 AND id <> ?`, exceptID).Scan(&n); err != nil {
		return err
	}
	if n == 0 {
		return ErrLastOwner
	}
	return nil
}

// ---------------------------------------------------------------------- mfa --

// MFA returns the sealed TOTP secret, whether 2FA is on, and the hashes of the unused recovery codes.
func (s *Store) MFA(ctx context.Context, userID int64) (secret string, enabled bool, codes []string, err error) {
	var raw string
	err = s.db.QueryRowContext(ctx, `SELECT totp_secret, mfa_enabled, recovery_codes FROM users WHERE id = ?`, userID).Scan(&secret, &enabled, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil, ErrNotFound
	}
	if err == nil {
		_ = json.Unmarshal([]byte(raw), &codes)
	}
	return secret, enabled, codes, err
}

// SetMFA stores the 2FA state ("" secret and enabled=false turns it off).
func (s *Store) SetMFA(ctx context.Context, userID int64, secret string, enabled bool, codes []string) error {
	if codes == nil {
		codes = []string{}
	}
	b, _ := json.Marshal(codes)
	res, err := s.db.ExecContext(ctx, `UPDATE users SET totp_secret = ?, mfa_enabled = ?, recovery_codes = ?, updated_at = ? WHERE id = ?`,
		secret, b2i(enabled), string(b), s.unix(), userID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// UseRecoveryCode consumes a recovery code (by hash). It reports whether the code was valid.
func (s *Store) UseRecoveryCode(ctx context.Context, userID int64, hash string) (bool, error) {
	used := false
	err := s.tx(ctx, func(tx *sql.Tx) error {
		var raw string
		if err := tx.QueryRowContext(ctx, `SELECT recovery_codes FROM users WHERE id = ?`, userID).Scan(&raw); err != nil {
			return err
		}
		var codes []string
		_ = json.Unmarshal([]byte(raw), &codes)
		rest := codes[:0]
		for _, c := range codes {
			if c == hash && !used {
				used = true
				continue
			}
			rest = append(rest, c)
		}
		if !used {
			return nil
		}
		b, _ := json.Marshal(rest)
		_, err := tx.ExecContext(ctx, `UPDATE users SET recovery_codes = ? WHERE id = ?`, string(b), userID)
		return err
	})
	return used, err
}

// ------------------------------------------------------------- user tokens --

// CreateUserToken stores a one-time token (reset or invite) by its hash; older tokens of the purpose go.
func (s *Store) CreateUserToken(ctx context.Context, userID int64, hash, purpose string, ttl time.Duration) error {
	return s.tx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `DELETE FROM user_tokens WHERE user_id = ? AND purpose = ?`, userID, purpose); err != nil {
			return err
		}
		now := s.now()
		_, err := tx.ExecContext(ctx, `INSERT INTO user_tokens (token_hash, user_id, purpose, expires_at, created_at) VALUES (?, ?, ?, ?, ?)`,
			hash, userID, purpose, now.Add(ttl).Unix(), now.Unix())
		return err
	})
}

// ConsumeUserToken deletes a valid token and returns its user (ErrNotFound when unknown or expired).
func (s *Store) ConsumeUserToken(ctx context.Context, hash string) (User, string, error) {
	var userID, exp int64
	var purpose string
	err := s.db.QueryRowContext(ctx, `SELECT user_id, purpose, expires_at FROM user_tokens WHERE token_hash = ?`, hash).Scan(&userID, &purpose, &exp)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, "", ErrNotFound
	}
	if err != nil {
		return User{}, "", err
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM user_tokens WHERE token_hash = ?`, hash); err != nil {
		return User{}, "", err
	}
	if exp < s.unix() {
		return User{}, "", ErrNotFound
	}
	u, err := s.GetUser(ctx, userID)
	return u, purpose, err
}

// PeekUserToken returns the user of a valid token without consuming it.
func (s *Store) PeekUserToken(ctx context.Context, hash string) (User, string, error) {
	var userID, exp int64
	var purpose string
	err := s.db.QueryRowContext(ctx, `SELECT user_id, purpose, expires_at FROM user_tokens WHERE token_hash = ?`, hash).Scan(&userID, &purpose, &exp)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && exp < s.unix()) {
		return User{}, "", ErrNotFound
	}
	if err != nil {
		return User{}, "", err
	}
	u, err := s.GetUser(ctx, userID)
	return u, purpose, err
}
