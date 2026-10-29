// Package registry runs the built-in image registry (registry:2 in the system namespace) that Git builds push to,
// and builds the Docker credentials for builds and for pulling from private registries.
//
// Nodes pull built images as <Host>/<repo>:<tag> (default registry.nectar.local/…): install.sh writes a k3s
// mirror (/etc/rancher/k3s/registries.yaml) that sends that name to the registry's NodePort on 127.0.0.1 with
// the same credentials, so the registry is never reachable without a password and needs no TLS.
package registry

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"nectar_apps/internal/auth"
	"nectar_apps/internal/kube"
	"nectar_apps/internal/secretbox"
	"nectar_apps/internal/store"
)

// Defaults.
const (
	Name         = "nectar-registry"
	User         = "nectar"
	DefaultHost  = "registry.nectar.local"
	DefaultPort  = 30500
	Image        = "registry:2.8.3"
	DefaultSizeG = 20
)

// Kube is the part of the Kubernetes client the registry needs.
type Kube interface {
	Apply(ctx context.Context, o kube.Object) (kube.Object, error)
	GetObject(ctx context.Context, kind, namespace, name string, out any) error
}

// Registry is the built-in registry.
type Registry struct {
	Kube      Kube
	Namespace string
	Password  string // "" = disabled (install.sh --registry sets it up)
	Host      string // name nodes pull from (k3s mirror), default registry.nectar.local
	NodePort  int    // default 30500
	SizeGB    int    // volume size, default 20
	HTTP      *http.Client
	// APIBase overrides the in-cluster address for the registry API (tests).
	APIBase string
}

// Enabled reports whether the built-in registry is configured.
func (r *Registry) Enabled() bool { return r != nil && r.Password != "" }

// ErrDisabled is returned when a feature needs the built-in registry.
var ErrDisabled = errors.New("the built-in registry is not set up: re-run install.sh with --registry (Git builds need it)")

func (r *Registry) host() string {
	if r.Host != "" {
		return r.Host
	}
	return DefaultHost
}

func (r *Registry) nodePort() int {
	if r.NodePort > 0 {
		return r.NodePort
	}
	return DefaultPort
}

// PullHost is the registry name pods pull from.
func (r *Registry) PullHost() string { return r.host() }

// PushHost is the in-cluster address builds push to.
func (r *Registry) PushHost() string { return Name + "." + r.Namespace + ".svc:5000" }

// PullRef is how pods refer to an image of the registry.
func (r *Registry) PullRef(repo, tag string) string { return r.host() + "/" + repo + ":" + tag }

// PushRef is where a build pushes the image.
func (r *Registry) PushRef(repo, tag string) string { return r.PushHost() + "/" + repo + ":" + tag }

// Owns reports whether an image reference points to the built-in registry.
func (r *Registry) Owns(image string) bool { return strings.HasPrefix(image, r.host()+"/") }

// Credential is a login for a registry.
type Credential struct {
	Host     string `json:"host"`
	Username string `json:"username"`
	Password string `json:"password"`
}

// DockerConfig returns a ~/.docker/config.json with the given logins.
func DockerConfig(creds []Credential) []byte {
	auths := map[string]any{}
	for _, c := range creds {
		host := c.Host
		if host == "docker.io" || host == "index.docker.io" {
			host = "https://index.docker.io/v1/"
		}
		auths[host] = map[string]string{
			"username": c.Username, "password": c.Password,
			"auth": base64.StdEncoding.EncodeToString([]byte(c.Username + ":" + c.Password)),
		}
	}
	b, _ := json.Marshal(map[string]any{"auths": auths})
	return b
}

// BuildCredential is the login builds use to push (and pull base images from) the built-in registry.
func (r *Registry) BuildCredential() Credential {
	return Credential{Host: r.PushHost(), Username: User, Password: r.Password}
}

// Ensure creates the registry (volume, credentials, Deployment, Service, weekly garbage collection).
func (r *Registry) Ensure(ctx context.Context) error {
	if !r.Enabled() {
		return ErrDisabled
	}
	htpasswd, err := r.htpasswd(ctx)
	if err != nil {
		return err
	}
	for _, o := range r.objects(htpasswd) {
		if _, err := r.Kube.Apply(ctx, o); err != nil {
			return fmt.Errorf("registry: %w", err)
		}
	}
	return nil
}

// htpasswd keeps the existing hash when it still matches the password (no needless restarts).
func (r *Registry) htpasswd(ctx context.Context) (string, error) {
	var sec kube.Object
	if err := r.Kube.GetObject(ctx, "Secret", r.Namespace, Name+"-auth", &sec); err == nil {
		if b, err := base64.StdEncoding.DecodeString(kube.Str(sec, "data", "htpasswd")); err == nil {
			if u, h, ok := strings.Cut(strings.TrimSpace(string(b)), ":"); ok && u == User && bcrypt.CompareHashAndPassword([]byte(h), []byte(r.Password)) == nil {
				return string(b), nil
			}
		}
	}
	return auth.Htpasswd([]auth.BasicAuthUser{{User: User, Password: r.Password}}, "")
}

func (r *Registry) objects(htpasswd string) []kube.Object {
	ns := r.Namespace
	labels := map[string]any{"app.kubernetes.io/name": Name, "app.kubernetes.io/part-of": "nectar", "app.kubernetes.io/managed-by": "nectar-apps"}
	meta := func(name string) map[string]any {
		return map[string]any{"name": name, "namespace": ns, "labels": labels}
	}
	size := r.SizeGB
	if size <= 0 {
		size = DefaultSizeG
	}
	sel := map[string]any{"app.kubernetes.io/name": Name}
	volumes := []any{
		map[string]any{"name": "data", "persistentVolumeClaim": map[string]any{"claimName": Name + "-data"}},
		map[string]any{"name": "auth", "secret": map[string]any{"secretName": Name + "-auth"}},
	}
	env := []any{
		map[string]any{"name": "REGISTRY_AUTH", "value": "htpasswd"},
		map[string]any{"name": "REGISTRY_AUTH_HTPASSWD_REALM", "value": "Nectar registry"},
		map[string]any{"name": "REGISTRY_AUTH_HTPASSWD_PATH", "value": "/auth/htpasswd"},
		map[string]any{"name": "REGISTRY_STORAGE_DELETE_ENABLED", "value": "true"},
		map[string]any{"name": "REGISTRY_HTTP_SECRET", "value": base64.RawURLEncoding.EncodeToString([]byte("nectar:" + r.Password))},
	}
	mounts := []any{
		map[string]any{"name": "data", "mountPath": "/var/lib/registry"},
		map[string]any{"name": "auth", "mountPath": "/auth", "readOnly": true},
	}
	sc := map[string]any{"allowPrivilegeEscalation": false, "capabilities": map[string]any{"drop": []any{"ALL"}}}
	return []kube.Object{
		{"apiVersion": "v1", "kind": "PersistentVolumeClaim", "metadata": meta(Name + "-data"),
			"spec": map[string]any{"accessModes": []any{"ReadWriteOnce"}, "resources": map[string]any{"requests": map[string]any{"storage": fmt.Sprintf("%dGi", size)}}}},
		{"apiVersion": "v1", "kind": "Secret", "metadata": meta(Name + "-auth"), "type": "Opaque",
			"stringData": map[string]any{"htpasswd": htpasswd}},
		{"apiVersion": "apps/v1", "kind": "Deployment", "metadata": meta(Name),
			"spec": map[string]any{
				"replicas": 1, "strategy": map[string]any{"type": "Recreate"},
				"selector": map[string]any{"matchLabels": sel},
				"template": map[string]any{
					"metadata": map[string]any{"labels": labels},
					"spec": map[string]any{
						"automountServiceAccountToken": false,
						"containers": []any{map[string]any{
							"name": "registry", "image": Image, "env": env, "volumeMounts": mounts, "securityContext": sc,
							"ports":          []any{map[string]any{"name": "http", "containerPort": 5000}},
							"readinessProbe": map[string]any{"tcpSocket": map[string]any{"port": 5000}, "periodSeconds": 5},
							"livenessProbe":  map[string]any{"tcpSocket": map[string]any{"port": 5000}, "periodSeconds": 20},
							"resources":      map[string]any{"requests": map[string]any{"cpu": "20m", "memory": "32Mi"}, "limits": map[string]any{"memory": "512Mi"}},
						}},
						"volumes": volumes,
					},
				},
			}},
		{"apiVersion": "v1", "kind": "Service", "metadata": meta(Name),
			"spec": map[string]any{"type": "NodePort", "selector": sel,
				"ports": []any{map[string]any{"name": "http", "port": 5000, "targetPort": "http", "nodePort": r.nodePort()}}}},
		// layers of deleted tags are only freed by garbage collection (weekly, Sunday 04:30 UTC)
		{"apiVersion": "batch/v1", "kind": "CronJob", "metadata": meta(Name + "-gc"),
			"spec": map[string]any{
				"schedule": "30 4 * * 0", "concurrencyPolicy": "Forbid", "successfulJobsHistoryLimit": 1, "failedJobsHistoryLimit": 2,
				"jobTemplate": map[string]any{"spec": map[string]any{"backoffLimit": 0, "template": map[string]any{
					"metadata": map[string]any{"labels": map[string]any{"app.kubernetes.io/name": Name + "-gc", "app.kubernetes.io/part-of": "nectar"}},
					"spec": map[string]any{
						"restartPolicy": "Never", "automountServiceAccountToken": false,
						"containers": []any{map[string]any{
							"name": "gc", "image": Image, "securityContext": sc,
							"command":      []any{"registry", "garbage-collect", "--delete-untagged", "/etc/docker/registry/config.yml"},
							"volumeMounts": []any{map[string]any{"name": "data", "mountPath": "/var/lib/registry"}},
						}},
						"volumes": []any{volumes[0]},
					},
				}}},
			}},
	}
}

func (r *Registry) api() string {
	if r.APIBase != "" {
		return strings.TrimRight(r.APIBase, "/")
	}
	return "http://" + r.PushHost()
}

func (r *Registry) client() *http.Client {
	if r.HTTP != nil {
		return r.HTTP
	}
	return &http.Client{Timeout: 30 * time.Second}
}

func (r *Registry) do(ctx context.Context, method, path string, hdr map[string]string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, r.api()+path, nil)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(User, r.Password)
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	return r.client().Do(req)
}

// Tags lists the tags of a repository (empty when it does not exist).
func (r *Registry) Tags(ctx context.Context, repo string) ([]string, error) {
	resp, err := r.do(ctx, http.MethodGet, "/v2/"+repo+"/tags/list", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("registry: tags of %s: HTTP %d", repo, resp.StatusCode)
	}
	var out struct {
		Tags []string `json:"tags"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&out); err != nil {
		return nil, err
	}
	sort.Strings(out.Tags)
	return out.Tags, nil
}

// DeleteTag removes a tag (its manifest); the layers go with the next garbage collection.
func (r *Registry) DeleteTag(ctx context.Context, repo, tag string) error {
	accept := "application/vnd.docker.distribution.manifest.v2+json, application/vnd.oci.image.manifest.v1+json, " +
		"application/vnd.oci.image.index.v1+json, application/vnd.docker.distribution.manifest.list.v2+json"
	resp, err := r.do(ctx, http.MethodHead, "/v2/"+repo+"/manifests/"+tag, map[string]string{"Accept": accept})
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil
	}
	digest := resp.Header.Get("Docker-Content-Digest")
	if resp.StatusCode != http.StatusOK || digest == "" {
		return fmt.Errorf("registry: manifest %s:%s: HTTP %d", repo, tag, resp.StatusCode)
	}
	resp, err = r.do(ctx, http.MethodDelete, "/v2/"+repo+"/manifests/"+digest, nil)
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusNotFound {
		return fmt.Errorf("registry: delete %s:%s: HTTP %d", repo, tag, resp.StatusCode)
	}
	return nil
}

// Prune deletes the build tags of a repository except the newest keep ones and those in use.
func (r *Registry) Prune(ctx context.Context, repo string, keep int, inUse map[string]bool) (int, error) {
	tags, err := r.Tags(ctx, repo)
	if err != nil {
		return 0, err
	}
	var builds []string
	for _, t := range tags {
		if strings.HasPrefix(t, "build-") {
			builds = append(builds, t)
		}
	}
	// build-<n>: newest first by number
	sort.Slice(builds, func(i, j int) bool { return buildNum(builds[i]) > buildNum(builds[j]) })
	n := 0
	for i, t := range builds {
		if i < keep || inUse[t] {
			continue
		}
		if err := r.DeleteTag(ctx, repo, t); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

func buildNum(tag string) int {
	n := 0
	for _, c := range strings.TrimPrefix(tag, "build-") {
		if c < '0' || c > '9' {
			break
		}
		n = n*10 + int(c-'0')
	}
	return n
}

// LoadCredentials returns the private registry logins (passwords opened).
func LoadCredentials(ctx context.Context, st *store.Store, box *secretbox.Box) ([]Credential, error) {
	list, err := st.ListRegistryCredentials(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Credential, 0, len(list))
	for _, c := range list {
		pw, err := box.Open(c.Password)
		if err != nil {
			return nil, fmt.Errorf("login for %s: %w", c.Host, err)
		}
		out = append(out, Credential{Host: c.Host, Username: c.Username, Password: pw})
	}
	return out, nil
}

// PullSecret is the Docker config.json for app pods (nil without logins).
func PullSecret(ctx context.Context, st *store.Store, box *secretbox.Box) ([]byte, error) {
	creds, err := LoadCredentials(ctx, st, box)
	if err != nil || len(creds) == 0 {
		return nil, err
	}
	return DockerConfig(creds), nil
}
