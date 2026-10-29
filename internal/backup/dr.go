package backup

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"nectar_apps/internal/deploy"
	"nectar_apps/internal/kube"
	"nectar_apps/internal/store"
)

// RestoreSecret is created by install.sh --restore: key "kit" holds a RecoveryKit (JSON).
const RestoreSecret = "nectar-restore"

// restoredMarker tells the panel, after it opened the restored database, to bring every app back.
const restoredMarker = ".restore-pending"

// RestorePanel runs before the panel opens its database: when install.sh left a recovery kit and the data
// directory has no database yet, the newest panel snapshot is restored into the panel volume. It returns
// true when it restored.
func RestorePanel(ctx context.Context, k Kube, ns, dataDir, claim string, uid int64, log *slog.Logger) (bool, error) {
	var sec kube.Object
	if err := k.GetObject(ctx, "Secret", ns, RestoreSecret, &sec); err != nil {
		if kube.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}
	raw, _ := base64.StdEncoding.DecodeString(kube.Str(sec, "data", "kit"))
	var kit RecoveryKit
	if err := json.Unmarshal(raw, &kit); err != nil || kit.Repository == "" {
		return false, fmt.Errorf("%s: invalid recovery kit", RestoreSecret)
	}
	if _, err := os.Stat(filepath.Join(dataDir, "nectar.db")); err == nil {
		log.Warn("a recovery kit is present but the panel already has a database: not restoring", "secret", RestoreSecret)
		return false, nil
	}
	log.Info("restoring the panel from its backup", "repository", kit.Repository)
	cfg := TargetConfig{Password: kit.Env["RESTIC_PASSWORD"], PrivateKey: kit.SSHKey, Port: kit.SSHPort, Path: kit.LocalPath}
	// the Job reads the repository settings from the same kind of Secret as every backup Job
	data := map[string]any{}
	for name, v := range kit.Env {
		data[name] = base64.StdEncoding.EncodeToString([]byte(v))
	}
	if kit.SSHKey != "" {
		data["ssh-key"] = base64.StdEncoding.EncodeToString([]byte(kit.SSHKey + "\n"))
	}
	const tid = 0
	if _, err := k.Apply(ctx, kube.Object{"apiVersion": "v1", "kind": "Secret", "type": "Opaque",
		"metadata": map[string]any{"name": secretName(tid), "namespace": ns, "labels": map[string]any{labelManagedBy: managedBy, labelBackup: "credentials"}},
		"data":     data}); err != nil {
		return false, err
	}
	if kit.Kind == KindSFTP {
		// sshCommand needs host and user: take them from "sftp:user@host:/path"
		if u, h, ok := splitSFTP(kit.Repository); ok {
			cfg.User, cfg.Host = u, h
		}
	}
	j := jobSpec{Namespace: ns, Name: fmt.Sprintf("nectar-restore-panel-%d", time.Now().Unix()), TargetID: tid, Kind: kit.Kind, Config: cfg,
		RunAsUser: &uid, HostRepo: true,
		Volumes: []map[string]any{{"name": "panel", "persistentVolumeClaim": map[string]any{"claimName": claim}}},
		Mounts:  []map[string]any{{"name": "panel", "mountPath": "/data"}},
		Script: `set -eu
export RESTIC_CACHE_DIR=/tmp/restic-cache HOME=/tmp
restic restore latest --host nectar --tag panel --target /
mv /data/backup/nectar.db /data/nectar.db
echo restored`}
	if _, err := k.Apply(ctx, j.object()); err != nil {
		return false, err
	}
	m := &Manager{Kube: k, Log: log}
	ok, logs, err := m.waitJob(ctx, ns, j.Name)
	if !ok {
		return false, fmt.Errorf("restoring the panel failed: %v\n%s", err, lastLines(logs, 10))
	}
	if err := os.WriteFile(filepath.Join(dataDir, restoredMarker), []byte(time.Now().UTC().Format(time.RFC3339)), 0o600); err != nil {
		return true, err
	}
	_ = k.Delete(ctx, "Secret", ns, RestoreSecret)
	log.Info("panel data restored; apps and databases are brought back after the start")
	return true, nil
}

func splitSFTP(repo string) (user, host string, ok bool) {
	rest, found := strings.CutPrefix(repo, "sftp:")
	if !found {
		return "", "", false
	}
	user, rest, found = strings.Cut(rest, "@")
	if !found {
		return "", "", false
	}
	host, _, found = strings.Cut(rest, ":")
	return user, host, found
}

// RestorePending reports whether the panel was just restored (RestoreAll still has to run).
func RestorePending(dataDir string) bool {
	_, err := os.Stat(filepath.Join(dataDir, restoredMarker))
	return err == nil
}

// RestoreAll brings a restored panel's workloads back: databases are created and get their newest dump,
// apps are redeployed and get their newest volume snapshots. It runs once after RestorePanel.
func (m *Manager) RestoreAll(ctx context.Context) error {
	defer os.Remove(filepath.Join(m.DataDir, restoredMarker))
	plans, err := m.Store.ListBackupPlans(ctx)
	if err != nil {
		return err
	}
	planFor := map[string]store.BackupPlan{}
	for _, p := range plans {
		planFor[Tag(p.Scope, p.SubjectID)] = p
	}
	var errs []error
	addons, err := m.Store.ListAddons(ctx, 0)
	if err != nil {
		return err
	}
	for _, a := range addons {
		if err := m.Deployer.ApplyAddon(ctx, a); err != nil {
			errs = append(errs, fmt.Errorf("database %s: %w", a.Slug, err))
			continue
		}
		if p, ok := planFor[Tag(ScopeAddon, a.ID)]; ok {
			if err := m.waitAddonReady(ctx, a); err != nil {
				errs = append(errs, fmt.Errorf("database %s: %w", a.Slug, err))
				continue
			}
			errs = append(errs, m.restoreLatest(ctx, p))
		}
	}
	projects, err := m.Store.ListProjects(ctx)
	if err != nil {
		return err
	}
	for _, proj := range projects {
		apps, err := m.Store.ListApps(ctx, proj.ID)
		if err != nil {
			return err
		}
		for _, a := range apps {
			if _, err := m.Store.LiveRelease(ctx, a.ID); err != nil {
				continue // never deployed
			}
			if _, err := m.Deployer.Deploy(ctx, deploy.DeployRequest{App: a, Force: true}); err != nil {
				errs = append(errs, fmt.Errorf("app %s/%s: %w", proj.Slug, a.Slug, err))
				continue
			}
			m.Deployer.Wait()
			if p, ok := planFor[Tag(ScopeApp, a.ID)]; ok {
				errs = append(errs, m.restoreLatest(ctx, p))
			}
		}
	}
	m.Wait()
	err = errors.Join(errs...)
	if err != nil {
		m.Log.Warn("disaster recovery finished with errors", "err", err)
	} else {
		m.Log.Info("disaster recovery finished: every app and database is back")
	}
	return err
}

func (m *Manager) restoreLatest(ctx context.Context, p store.BackupPlan) error {
	snaps, err := m.Snapshots(ctx, p)
	if err != nil {
		return err
	}
	if len(snaps) == 0 {
		return nil
	}
	if _, err := m.StartRestore(ctx, p, snaps[0].ID); err != nil {
		return err
	}
	m.Wait()
	return nil
}

func (m *Manager) waitAddonReady(ctx context.Context, a store.Addon) error {
	p, err := m.Store.GetProject(ctx, a.ProjectID)
	if err != nil {
		return err
	}
	deadline := m.now().Add(10 * time.Minute)
	for {
		live, err := m.Deployer.AddonStatus(ctx, a, p)
		if err == nil && live.Ready {
			return nil
		}
		if m.now().After(deadline) {
			return errors.New("not ready after 10 minutes")
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(m.poll()):
		}
	}
}
