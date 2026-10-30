// Package rekey rotates the panel master key and keeps the artifact signing key.
//
// The master key seals every secret at rest (secretbox, one derived key per purpose). Rotation
// happens while the panel starts, before anything else uses the database, so no running process
// keeps writing with the old key:
//
//  1. `nectar-apps admin rotate-master-key` stores a new key as master-key-next in Secret
//     nectar-secrets and restarts the panel (Recreate strategy: one pod at a time).
//  2. At startup Startup sees master-key-next, re-seals every value from the current key to the new
//     one in a single transaction and records the key id in the database, then makes the new key
//     master-key in the Secret. A crash in between is safe: the key id tells which key the database
//     uses, and the next start finishes the switch.
package rekey

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"time"

	"nectar_apps/internal/deploy"
	"nectar_apps/internal/kube"
	"nectar_apps/internal/secretbox"
	"nectar_apps/internal/store"
)

// SecretName and the keys of the panel Secret (written by install.sh).
const (
	SecretName = "nectar-secrets"
	KeyCurrent = "master-key"
	KeyNext    = "master-key-next"
)

// settingKeyID records which master key the database is sealed with.
const settingKeyID = "master_key_id"

// settingArtifactKey holds the artifact signing key, sealed with the "env" box.
const settingArtifactKey = "artifact_signing_key"

// ErrWrongKey: the database is sealed with another master key than the one the panel got.
var ErrWrongKey = errors.New("the database is sealed with a different master key")

// KeyID identifies a master key without revealing it.
func KeyID(key []byte) string {
	m := hmac.New(sha256.New, key)
	m.Write([]byte("nectar-apps/master-key-id"))
	return hex.EncodeToString(m.Sum(nil))[:16]
}

// NewKey returns a random master key (base64, as install.sh writes it).
func NewKey() (raw []byte, encoded string) {
	raw = make([]byte, 32)
	_, _ = rand.Read(raw)
	return raw, base64.StdEncoding.EncodeToString(raw)
}

// column is one sealed column; where selects the rows that hold a sealed value.
type column struct {
	purpose, table, col, where string
}

// columns are every value sealed with a box derived from the master key. A new sealed column must be
// added here (the rekey test fills each of them).
var columns = []column{
	{"env", "env_vars", "value_enc", ""},
	{"env", "app_files", "content_enc", ""},
	{"env", "addons", "credentials", ""},
	{"totp", "users", "totp_secret", "totp_secret != ''"},
	{"alerts", "alert_channels", "config", ""},
	{"backups", "backup_targets", "config", ""},
	{"registries", "registry_credentials", "password", ""},
	{"builds", "app_sources", "token", "token != ''"},
	{"builds", "app_sources", "webhook_secret", "webhook_secret != ''"},
}

// sealedSettings are settings rows whose JSON has sealed fields: key → purpose, field ("" = the whole
// value is a sealed string).
var sealedSettings = []struct{ key, purpose, field string }{
	{"smtp", "smtp", "password"},
	{settingArtifactKey, "env", ""},
}

// Purposes lists every secretbox purpose in use.
func Purposes() []string {
	seen := map[string]bool{}
	var out []string
	for _, c := range columns {
		if !seen[c.purpose] {
			seen[c.purpose] = true
			out = append(out, c.purpose)
		}
	}
	for _, s := range sealedSettings {
		if !seen[s.purpose] {
			seen[s.purpose] = true
			out = append(out, s.purpose)
		}
	}
	return out
}

type boxes struct{ old, new map[string]*secretbox.Box }

func newBoxes(oldKey, newKey []byte) (boxes, error) {
	b := boxes{old: map[string]*secretbox.Box{}, new: map[string]*secretbox.Box{}}
	for _, p := range Purposes() {
		o, err := secretbox.New(oldKey, p)
		if err != nil {
			return b, err
		}
		n, err := secretbox.New(newKey, p)
		if err != nil {
			return b, err
		}
		b.old[p], b.new[p] = o, n
	}
	return b, nil
}

func (b boxes) reseal(purpose, v string) (string, error) {
	pt, err := b.old[purpose].Open(v)
	if err != nil {
		return "", err
	}
	return b.new[purpose].Seal(pt)
}

// Report counts the re-sealed values.
type Report struct {
	Values int
	Tables map[string]int
}

// Reseal moves every sealed value from oldKey to newKey and records newKey's id, in one transaction.
func Reseal(ctx context.Context, st *store.Store, oldKey, newKey []byte) (Report, error) {
	rep := Report{Tables: map[string]int{}}
	bx, err := newBoxes(oldKey, newKey)
	if err != nil {
		return rep, err
	}
	tx, err := st.DB().BeginTx(ctx, nil)
	if err != nil {
		return rep, err
	}
	defer tx.Rollback()
	for _, c := range columns {
		q := "SELECT rowid, " + c.col + " FROM " + c.table
		if c.where != "" {
			q += " WHERE " + c.where
		}
		rows, err := tx.QueryContext(ctx, q)
		if err != nil {
			return rep, fmt.Errorf("%s.%s: %w", c.table, c.col, err)
		}
		type row struct {
			id int64
			v  string
		}
		var all []row
		for rows.Next() {
			var r row
			if err := rows.Scan(&r.id, &r.v); err != nil {
				rows.Close()
				return rep, err
			}
			all = append(all, r)
		}
		rows.Close()
		for _, r := range all {
			nv, err := bx.reseal(c.purpose, r.v)
			if err != nil {
				return rep, fmt.Errorf("%s.%s row %d: %w", c.table, c.col, r.id, err)
			}
			if _, err := tx.ExecContext(ctx, "UPDATE "+c.table+" SET "+c.col+" = ? WHERE rowid = ?", nv, r.id); err != nil {
				return rep, err
			}
			rep.Values++
			rep.Tables[c.table+"."+c.col]++
		}
	}
	for _, ss := range sealedSettings {
		var raw string
		err := tx.QueryRowContext(ctx, "SELECT value FROM settings WHERE key = ?", ss.key).Scan(&raw)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return rep, fmt.Errorf("setting %s: %w", ss.key, err)
		}
		var out string
		if ss.field == "" {
			var sealed string
			if err := json.Unmarshal([]byte(raw), &sealed); err != nil {
				return rep, fmt.Errorf("setting %s: %w", ss.key, err)
			}
			nv, err := bx.reseal(ss.purpose, sealed)
			if err != nil {
				return rep, fmt.Errorf("setting %s: %w", ss.key, err)
			}
			b, _ := json.Marshal(nv)
			out = string(b)
		} else {
			var m map[string]any
			if err := json.Unmarshal([]byte(raw), &m); err != nil {
				return rep, fmt.Errorf("setting %s: %w", ss.key, err)
			}
			v, _ := m[ss.field].(string)
			if v == "" {
				continue
			}
			nv, err := bx.reseal(ss.purpose, v)
			if err != nil {
				return rep, fmt.Errorf("setting %s.%s: %w", ss.key, ss.field, err)
			}
			m[ss.field] = nv
			b, _ := json.Marshal(m)
			out = string(b)
		}
		if _, err := tx.ExecContext(ctx, "UPDATE settings SET value = ? WHERE key = ?", out, ss.key); err != nil {
			return rep, err
		}
		rep.Values++
		rep.Tables["settings."+ss.key]++
	}
	id, _ := json.Marshal(KeyID(newKey))
	if _, err := tx.ExecContext(ctx, `INSERT INTO settings (key, value, updated_at) VALUES (?, ?, ?)
		ON CONFLICT (key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`, settingKeyID, string(id), time.Now().Unix()); err != nil {
		return rep, err
	}
	return rep, tx.Commit()
}

// dbKeyID returns the recorded key id ("" before the first start with this package).
func dbKeyID(ctx context.Context, st *store.Store) string {
	var id string
	if err := st.GetSetting(ctx, settingKeyID, &id); err != nil {
		return ""
	}
	return id
}

// Kube is the part of the Kubernetes client Startup needs.
type Kube interface {
	GetObject(ctx context.Context, kind, namespace, name string, out any) error
	Patch(ctx context.Context, kind, namespace, name string, patch any) (kube.Object, error)
	List(ctx context.Context, kind, namespace, labelSelector string) ([]kube.Object, error)
}

// Options for Startup.
type Options struct {
	Store     *store.Store
	Kube      Kube // nil: no Secret handling (development)
	Namespace string
	Current   []byte // NECTAR_MASTER_KEY
	Log       *slog.Logger
}

// Startup checks the master key against the database and finishes a pending rotation. It returns the
// key the panel must use from now on.
func Startup(ctx context.Context, o Options) ([]byte, error) {
	var next []byte
	if o.Kube != nil {
		var sec kube.Object
		err := o.Kube.GetObject(ctx, "Secret", o.Namespace, SecretName, &sec)
		if err != nil && !kube.IsNotFound(err) {
			return nil, fmt.Errorf("read Secret %s: %w", SecretName, err)
		}
		if enc := kube.Str(sec, "data", KeyNext); enc != "" {
			b, err := base64.StdEncoding.DecodeString(enc)
			if err == nil {
				next, err = base64.StdEncoding.DecodeString(strings.TrimSpace(string(b)))
			}
			if err != nil || len(next) < 32 {
				return nil, fmt.Errorf("Secret %s: %s is not a base64 key of at least 32 bytes", SecretName, KeyNext)
			}
		}
	}
	id := dbKeyID(ctx, o.Store)
	cur := KeyID(o.Current)
	switch {
	case next == nil && (id == "" || id == cur):
		if id == "" {
			b, _ := json.Marshal(cur)
			_ = o.Store.SetSetting(ctx, settingKeyID, json.RawMessage(b))
		}
		return o.Current, nil
	case next == nil:
		return nil, fmt.Errorf("%w (id %s, the panel got %s): start the panel with the key printed when this database was created or rotated", ErrWrongKey, id, cur)
	case id == KeyID(next):
		// re-sealed already, the Secret switch did not happen (crash): finish it
		o.Log.Info("master key rotation: database already uses the new key, updating the Secret")
	case id == "" || id == cur:
		rep, err := Reseal(ctx, o.Store, o.Current, next)
		if err != nil {
			return nil, fmt.Errorf("master key rotation: %w (nothing was changed)", err)
		}
		o.Log.Info("master key rotation: secrets re-sealed", "values", rep.Values)
		_ = o.Store.Audit(ctx, nil, "system", "master_key.rotated", "", "", map[string]any{"values": rep.Values, "key_id": KeyID(next)})
		if n, err := Resign(ctx, o.Kube, o.Store, o.Current, next); err != nil {
			o.Log.Warn("master key rotation: re-signing artifact URLs", "err", err)
		} else if n > 0 {
			o.Log.Info("master key rotation: artifact URLs re-signed", "workloads", n)
		}
	default:
		return nil, fmt.Errorf("%w (id %s; current %s, next %s)", ErrWrongKey, id, cur, KeyID(next))
	}
	enc := base64.StdEncoding.EncodeToString([]byte(base64.StdEncoding.EncodeToString(next)))
	if _, err := o.Kube.Patch(ctx, "Secret", o.Namespace, SecretName, map[string]any{"data": map[string]any{KeyCurrent: enc, KeyNext: nil}}); err != nil {
		return nil, fmt.Errorf("master key rotation: the database uses the new key but Secret %s could not be updated (%w): it is finished at the next start", SecretName, err)
	}
	o.Log.Info("master key rotation finished", "key_id", KeyID(next))
	return next, nil
}

// ArtifactKey returns the artifact signing key (random, stored sealed), creating it on first use.
// Artifact download URLs used to be signed with the master key itself; those keep working (see
// VerifyArtifact) until a rotation re-signs them.
func ArtifactKey(ctx context.Context, st *store.Store, env *secretbox.Box) ([]byte, error) {
	var sealed string
	err := st.GetSetting(ctx, settingArtifactKey, &sealed)
	if err == nil && sealed != "" {
		pt, err := env.Open(sealed)
		if err != nil {
			return nil, fmt.Errorf("artifact signing key: %w", err)
		}
		return base64.StdEncoding.DecodeString(pt)
	}
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return nil, err
	}
	raw, enc := NewKey()
	s, err := env.Seal(enc)
	if err != nil {
		return nil, err
	}
	if err := st.SetSetting(ctx, settingArtifactKey, s); err != nil {
		return nil, err
	}
	return raw, nil
}

// VerifyArtifact accepts URLs signed with the artifact key, and older ones signed with the master key.
func VerifyArtifact(artifactKey, masterKey []byte, sha, sig string) bool {
	return deploy.VerifyArtifact(artifactKey, sha, sig) || deploy.VerifyArtifact(masterKey, sha, sig)
}

// Resign rewrites artifact download URLs in the panel's workloads that were signed with the old master
// key (before the artifact key existed), so their pods can still fetch the artifact after a rotation.
func Resign(ctx context.Context, k Kube, st *store.Store, oldMaster, newMaster []byte) (int, error) {
	if k == nil {
		return 0, nil
	}
	env, err := secretbox.New(newMaster, "env")
	if err != nil {
		return 0, err
	}
	akey, err := ArtifactKey(ctx, st, env)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, kind := range []string{"Deployment", "CronJob"} {
		items, err := k.List(ctx, kind, "", deploy.LabelManagedBy+"="+deploy.ManagedBy)
		if err != nil {
			if kube.IsNotFound(err) {
				continue
			}
			return n, err
		}
		for _, it := range items {
			_, ns, name := kube.Meta(it)
			path := []string{"spec", "template", "spec"}
			if kind == "CronJob" {
				path = []string{"spec", "jobTemplate", "spec", "template", "spec"}
			}
			inits := kube.Slice(it, append(path, "initContainers")...)
			changed := false
			for _, ic := range inits {
				for _, e := range kube.Slice(ic, "env") {
					if kube.Str(e, "name") != "ARTIFACT_URL" {
						continue
					}
					u, err := url.Parse(kube.Str(e, "value"))
					if err != nil {
						continue
					}
					sha := u.Path[strings.LastIndex(u.Path, "/")+1:]
					sig := u.Query().Get("sig")
					if deploy.VerifyArtifact(oldMaster, sha, sig) {
						q := u.Query()
						q.Set("sig", deploy.SignArtifact(akey, sha))
						u.RawQuery = q.Encode()
						e["value"] = u.String()
						changed = true
					}
				}
			}
			if !changed {
				continue
			}
			patch := map[string]any{"initContainers": inits}
			for i := len(path) - 1; i >= 0; i-- {
				patch = map[string]any{path[i]: patch}
			}
			if _, err := k.Patch(ctx, kind, ns, name, patch); err != nil {
				return n, fmt.Errorf("%s %s/%s: %w", kind, ns, name, err)
			}
			n++
		}
	}
	return n, nil
}
