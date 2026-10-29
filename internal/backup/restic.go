// Package backup runs restic backups of app volumes, database dumps and the panel's own data as Kubernetes
// Jobs, keeps the history, restores, and brings a whole server back after a disaster (PLAN.md §7).
package backup

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"path"
	"strconv"
	"strings"

	"nectar_apps/internal/kube"
	"nectar_apps/internal/secretbox"
)

// ResticImage runs every backup and restore.
var ResticImage = "restic/restic:0.17.3"

// Target kinds.
const (
	KindS3    = "s3" // AWS S3, Backblaze B2 (S3 API), Cloudflare R2, MinIO, Wasabi, …
	KindSFTP  = "sftp"
	KindLocal = "local" // a folder on the server, served by a restic rest-server in the system namespace
)

// TargetConfig is the sealed configuration of a target.
type TargetConfig struct {
	// s3
	Endpoint  string `json:"endpoint,omitempty"` // https://s3.eu-central-1.amazonaws.com, https://s3.us-west-004.backblazeb2.com, …
	Bucket    string `json:"bucket,omitempty"`
	Prefix    string `json:"prefix,omitempty"` // folder in the bucket
	Region    string `json:"region,omitempty"`
	AccessKey string `json:"access_key,omitempty"`
	SecretKey string `json:"secret_key,omitempty"`

	// sftp
	Host       string `json:"host,omitempty"`
	Port       int    `json:"port,omitempty"`
	User       string `json:"user,omitempty"`
	PrivateKey string `json:"private_key,omitempty"` // OpenSSH private key (no passphrase)

	// sftp + local
	Path string `json:"path,omitempty"`

	// local: the rest-server credentials (Jobs in project namespaces cannot mount host folders)
	RestUser     string `json:"rest_user,omitempty"`
	RestPassword string `json:"rest_password,omitempty"`

	// Password encrypts the restic repository. Generated once; without it the backups cannot be read.
	Password string `json:"password,omitempty"`
}

// Masked stands for an unchanged secret in the API.
const Masked = "••••••••"

// Mask hides secrets.
func (c TargetConfig) Mask() TargetConfig {
	for _, p := range []*string{&c.SecretKey, &c.PrivateKey, &c.Password, &c.RestPassword} {
		if *p != "" {
			*p = Masked
		}
	}
	return c
}

// KeepSecrets copies secrets from old where the input still holds Masked. The repository password never
// changes once set (the existing backups are encrypted with it).
func (c TargetConfig) KeepSecrets(old TargetConfig) TargetConfig {
	if c.SecretKey == Masked {
		c.SecretKey = old.SecretKey
	}
	if c.PrivateKey == Masked {
		c.PrivateKey = old.PrivateKey
	}
	if old.Password != "" {
		c.Password = old.Password
	}
	if old.RestPassword != "" {
		c.RestUser, c.RestPassword = old.RestUser, old.RestPassword
	}
	return c
}

// Validate checks a config and fills in defaults (a new repository password included).
func (c *TargetConfig) Validate(kind string) error {
	switch kind {
	case KindS3:
		if c.Endpoint == "" {
			c.Endpoint = "https://s3.amazonaws.com"
		}
		u, err := url.Parse(c.Endpoint)
		if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
			return errors.New("endpoint must be an http(s) URL, e.g. https://s3.eu-central-1.amazonaws.com")
		}
		c.Endpoint = strings.TrimRight(c.Endpoint, "/")
		if c.Bucket == "" || strings.ContainsAny(c.Bucket, "/ ") {
			return errors.New("a bucket name is required")
		}
		c.Prefix = strings.Trim(c.Prefix, "/")
		if c.AccessKey == "" || c.SecretKey == "" {
			return errors.New("access key and secret key are required")
		}
	case KindSFTP:
		if c.Host == "" || c.User == "" {
			return errors.New("host and user are required")
		}
		if c.Port == 0 {
			c.Port = 22
		}
		if c.Path == "" {
			return errors.New("a folder on the server is required")
		}
		if !strings.Contains(c.PrivateKey, "PRIVATE KEY") {
			return errors.New("paste an OpenSSH private key without a passphrase")
		}
	case KindLocal:
		if !path.IsAbs(c.Path) || path.Clean(c.Path) != c.Path || c.Path == "/" || strings.HasPrefix(c.Path, "/var/lib/rancher") {
			return errors.New("an absolute folder on the server is required, e.g. /var/backups/nectar")
		}
	default:
		return fmt.Errorf("unknown target kind %q", kind)
	}
	if c.Password == "" {
		c.Password = randomToken()
	}
	if kind == KindLocal && c.RestPassword == "" {
		c.RestUser, c.RestPassword = "nectar", randomToken()
	}
	return nil
}

func randomToken() string {
	var b [24]byte
	_, _ = rand.Read(b[:])
	return base64.RawURLEncoding.EncodeToString(b[:])
}

// Repository is the restic repository string. Local targets go through the rest-server of the target
// (restServer = "host:port"); "" gives the folder itself (/repo, for Jobs that mount it).
func (c TargetConfig) Repository(kind, restServer string) string {
	switch kind {
	case KindS3:
		r := "s3:" + c.Endpoint + "/" + c.Bucket
		if c.Prefix != "" {
			r += "/" + c.Prefix
		}
		return r
	case KindSFTP:
		return "sftp:" + c.User + "@" + c.Host + ":" + c.Path
	default:
		if restServer == "" {
			return "/repo"
		}
		return "rest:http://" + c.RestUser + ":" + c.RestPassword + "@" + restServer + "/"
	}
}

// Env are the environment variables restic needs (they go into a Secret).
func (c TargetConfig) Env(kind, restServer string) map[string]string {
	env := map[string]string{"RESTIC_REPOSITORY": c.Repository(kind, restServer), "RESTIC_PASSWORD": c.Password}
	if kind == KindS3 {
		env["AWS_ACCESS_KEY_ID"], env["AWS_SECRET_ACCESS_KEY"] = c.AccessKey, c.SecretKey
		if c.Region != "" {
			env["AWS_DEFAULT_REGION"] = c.Region
		}
	}
	return env
}

// Display is the repository as shown in the UI (no credentials).
func (c TargetConfig) Display(kind string) string {
	if kind == KindLocal {
		return "folder " + c.Path + " on the server"
	}
	return c.Repository(kind, "")
}

// Seal and Open store configs encrypted.
func Seal(box *secretbox.Box, c TargetConfig) (string, error) {
	b, _ := json.Marshal(c)
	return box.Seal(string(b))
}

// Open decrypts a stored config.
func Open(box *secretbox.Box, sealed string) (TargetConfig, error) {
	var c TargetConfig
	pt, err := box.Open(sealed)
	if err != nil {
		return c, err
	}
	err = json.Unmarshal([]byte(pt), &c)
	return c, err
}

// RecoveryKit is everything install.sh --restore needs besides the master key.
type RecoveryKit struct {
	Kind       string            `json:"kind"`
	Repository string            `json:"repository"`
	Env        map[string]string `json:"env"`
	SSHKey     string            `json:"ssh_key,omitempty"`
	SSHPort    int               `json:"ssh_port,omitempty"`
	LocalPath  string            `json:"local_path,omitempty"`
}

// Kit returns the recovery kit of a target.
func (c TargetConfig) Kit(kind string) RecoveryKit {
	k := RecoveryKit{Kind: kind, Repository: c.Repository(kind, ""), Env: c.Env(kind, "")}
	if kind == KindSFTP {
		k.SSHKey, k.SSHPort = c.PrivateKey, c.Port
	}
	if kind == KindLocal {
		k.LocalPath = c.Path
	}
	return k
}

// ------------------------------------------------------------------ jobs --

// secretName is the per-namespace copy of a target's credentials.
func secretName(targetID int64) string { return "nectar-backup-t" + strconv.FormatInt(targetID, 10) }

// targetSecret holds the restic env (and the SSH key) in the Job's namespace.
func targetSecret(ns string, targetID int64, kind string, c TargetConfig, restServer string) kube.Object {
	data := map[string]any{}
	for k, v := range c.Env(kind, restServer) {
		data[k] = base64.StdEncoding.EncodeToString([]byte(v))
	}
	if kind == KindSFTP {
		data["ssh-key"] = base64.StdEncoding.EncodeToString([]byte(strings.TrimSpace(c.PrivateKey) + "\n"))
	}
	return kube.Object{
		"apiVersion": "v1", "kind": "Secret", "type": "Opaque",
		"metadata": map[string]any{"name": secretName(targetID), "namespace": ns, "labels": map[string]any{labelManagedBy: managedBy, labelBackup: "credentials"}},
		"data":     data,
	}
}

const (
	labelManagedBy = "app.kubernetes.io/managed-by"
	managedBy      = "nectar-apps"
	labelBackup    = "nectar.dev/backup"
)

// jobSpec describes a restic Job.
type jobSpec struct {
	Namespace string
	Name      string
	TargetID  int64
	Kind      string // target kind
	Config    TargetConfig
	Script    string            // runs in the restic container (sh)
	Env       map[string]string // extra env for the restic container
	// Init runs before restic (database dumps), Main after it (database restores); both share /work.
	Init, After []map[string]any
	Volumes     []map[string]any
	Mounts      []map[string]any // restic container mounts (besides /work and the repository)
	RunAsUser   *int64
	Deadline    int
	// HostRepo mounts a local target's folder at /repo (only the system namespace may mount host folders).
	HostRepo bool
}

// sshCommand makes restic use the mounted key (and accept the host key on first use).
func sshCommand(c TargetConfig) string {
	return fmt.Sprintf("ssh -i /ssh/key -p %d -o StrictHostKeyChecking=accept-new -o UserKnownHostsFile=/tmp/known_hosts %s@%s -s sftp", c.Port, c.User, c.Host)
}

// resticPreamble opens (or creates) the repository.
const resticPreamble = `set -eu
export RESTIC_CACHE_DIR=/tmp/restic-cache HOME=/tmp
restic cat config >/dev/null 2>&1 || restic init
`

func (j jobSpec) object() kube.Object {
	env := []any{}
	for k, v := range j.Env {
		env = append(env, map[string]any{"name": k, "value": v})
	}
	script := j.Script
	if j.Kind == KindSFTP {
		// restic reads extra options from the command line only: wrap it in a function
		script = "restic() { command restic -o \"sftp.command=" + sshCommand(j.Config) + "\" \"$@\"; }\n" + script
	}
	mounts := append([]any{map[string]any{"name": "work", "mountPath": "/work"}}, toAny(j.Mounts)...)
	volumes := append([]any{map[string]any{"name": "work", "emptyDir": map[string]any{}}}, toAny(j.Volumes)...)
	switch j.Kind {
	case KindSFTP:
		mounts = append(mounts, map[string]any{"name": "ssh", "mountPath": "/ssh", "readOnly": true})
		volumes = append(volumes, map[string]any{"name": "ssh", "secret": map[string]any{"secretName": secretName(j.TargetID),
			"items": []any{map[string]any{"key": "ssh-key", "path": "key"}}, "defaultMode": 0o400}})
	case KindLocal:
		if !j.HostRepo {
			break
		}
		mounts = append(mounts, map[string]any{"name": "repo", "mountPath": "/repo"})
		volumes = append(volumes, map[string]any{"name": "repo", "hostPath": map[string]any{"path": j.Config.Path, "type": "DirectoryOrCreate"}})
	}
	restic := map[string]any{
		"name":            "restic",
		"image":           ResticImage,
		"imagePullPolicy": "IfNotPresent",
		"command":         []any{"sh", "-c", script},
		"envFrom":         []any{map[string]any{"secretRef": map[string]any{"name": secretName(j.TargetID)}}},
		"env":             env,
		"volumeMounts":    mounts,
		"resources":       map[string]any{"requests": map[string]any{"cpu": "100m", "memory": "128Mi"}, "limits": map[string]any{"memory": "1Gi"}},
	}
	// restic runs first for restores (it fetches the dump), last for backups (it uploads the dump)
	var inits, containers []any
	inits = append(inits, toAny(j.Init)...)
	if len(j.After) > 0 {
		inits = append(inits, restic)
		containers = toAny(j.After)
	} else {
		containers = []any{restic}
	}
	pod := map[string]any{
		"restartPolicy":                "Never",
		"automountServiceAccountToken": false,
		"enableServiceLinks":           false,
		"containers":                   containers,
		"volumes":                      volumes,
	}
	if len(inits) > 0 {
		pod["initContainers"] = inits
	}
	if j.RunAsUser != nil {
		pod["securityContext"] = map[string]any{"runAsUser": *j.RunAsUser, "runAsGroup": *j.RunAsUser, "fsGroup": *j.RunAsUser}
	}
	deadline := j.Deadline
	if deadline == 0 {
		deadline = 6 * 3600
	}
	labels := map[string]any{labelManagedBy: managedBy, labelBackup: "job"}
	return kube.Object{
		"apiVersion": "batch/v1", "kind": "Job",
		"metadata": map[string]any{"name": j.Name, "namespace": j.Namespace, "labels": labels},
		"spec": map[string]any{
			"backoffLimit":            0,
			"activeDeadlineSeconds":   deadline,
			"ttlSecondsAfterFinished": 7 * 86400,
			"template":                map[string]any{"metadata": map[string]any{"labels": labels}, "spec": pod},
		},
	}
}

func toAny[T any](in []T) []any {
	out := make([]any, len(in))
	for i, v := range in {
		out[i] = v
	}
	return out
}

// shellQuote quotes s for sh.
func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

// RestServerName is the Deployment and Service of a local target's rest-server.
func RestServerName(targetID int64) string {
	return "nectar-backup-repo-" + strconv.FormatInt(targetID, 10)
}

// RestServerImage serves local targets.
var RestServerImage = "restic/rest-server:0.13.0"

// restServerObjects returns the Secret (htpasswd), Deployment and Service of a local target.
func restServerObjects(ns string, targetID int64, c TargetConfig, htpasswd string) []kube.Object {
	name := RestServerName(targetID)
	labels := map[string]any{labelManagedBy: managedBy, labelBackup: "repository", "app.kubernetes.io/name": name}
	return []kube.Object{
		{"apiVersion": "v1", "kind": "Secret", "type": "Opaque",
			"metadata": map[string]any{"name": name, "namespace": ns, "labels": labels},
			"data":     map[string]any{"htpasswd": base64.StdEncoding.EncodeToString([]byte(htpasswd + "\n"))}},
		{"apiVersion": "apps/v1", "kind": "Deployment",
			"metadata": map[string]any{"name": name, "namespace": ns, "labels": labels},
			"spec": map[string]any{
				"replicas": 1,
				"strategy": map[string]any{"type": "Recreate"},
				"selector": map[string]any{"matchLabels": map[string]any{"app.kubernetes.io/name": name}},
				"template": map[string]any{
					"metadata": map[string]any{"labels": labels},
					"spec": map[string]any{
						"automountServiceAccountToken": false,
						"containers": []any{map[string]any{
							"name":            "rest-server",
							"image":           RestServerImage,
							"imagePullPolicy": "IfNotPresent",
							"command":         []any{"rest-server"},
							"args":            []any{"--path", "/data", "--listen", ":8000", "--htpasswd-file", "/auth/htpasswd"},
							"ports":           []any{map[string]any{"name": "rest", "containerPort": 8000}},
							"readinessProbe":  map[string]any{"tcpSocket": map[string]any{"port": 8000}, "periodSeconds": 5},
							"resources":       map[string]any{"requests": map[string]any{"cpu": "20m", "memory": "32Mi"}, "limits": map[string]any{"memory": "256Mi"}},
							"volumeMounts": []any{
								map[string]any{"name": "data", "mountPath": "/data"},
								map[string]any{"name": "auth", "mountPath": "/auth", "readOnly": true},
							},
						}},
						"volumes": []any{
							map[string]any{"name": "data", "hostPath": map[string]any{"path": c.Path, "type": "DirectoryOrCreate"}},
							map[string]any{"name": "auth", "secret": map[string]any{"secretName": name}},
						},
					},
				},
			}},
		{"apiVersion": "v1", "kind": "Service",
			"metadata": map[string]any{"name": name, "namespace": ns, "labels": labels},
			"spec": map[string]any{"selector": map[string]any{"app.kubernetes.io/name": name},
				"ports": []any{map[string]any{"name": "rest", "port": 8000, "targetPort": "rest"}}}},
	}
}
