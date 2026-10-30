package build

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"nectar_apps/internal/deploy"
	"nectar_apps/internal/kube"
	"nectar_apps/internal/registry"
	"nectar_apps/internal/secretbox"
	"nectar_apps/internal/store"
)

// Builds run in their own namespace: they are not limited by project quotas and never share a network with apps.
const (
	DefaultNamespace = "nectar-builds"
	KanikoImage      = "gcr.io/kaniko-project/executor:v1.23.2"
	keepImages       = 5 // build images kept per app (plus the ones live releases use)
)

// Kube is the part of the Kubernetes client builds need.
type Kube interface {
	Apply(ctx context.Context, o kube.Object) (kube.Object, error)
	GetObject(ctx context.Context, kind, namespace, name string, out any) error
	Delete(ctx context.Context, kind, namespace, name string) error
	List(ctx context.Context, kind, namespace, labelSelector string) ([]kube.Object, error)
	PodLogs(ctx context.Context, namespace, pod, container string, tailLines int, previous bool) (string, error)
}

// Builder starts and tracks builds.
type Builder struct {
	Store     *store.Store
	Kube      Kube
	Registry  *registry.Registry
	Deployer  *deploy.Deployer // nil in tests that do not deploy
	Box       *secretbox.Box   // source tokens and webhook secrets
	Creds     func(ctx context.Context) ([]registry.Credential, error)
	Remote    Remote
	Log       *slog.Logger
	Namespace string        // default nectar-builds
	Image     string        // Kaniko executor image, default KanikoImage
	Poll      time.Duration // default 3 s
	Timeout   time.Duration // default 1 h

	mu      sync.Mutex
	running sync.WaitGroup
	cancel  map[int64]bool
}

func (b *Builder) image() string {
	if b.Image != "" {
		return b.Image
	}
	return KanikoImage
}

func (b *Builder) ns() string {
	if b.Namespace != "" {
		return b.Namespace
	}
	return DefaultNamespace
}

func (b *Builder) poll() time.Duration {
	if b.Poll > 0 {
		return b.Poll
	}
	return 3 * time.Second
}

func (b *Builder) timeout() time.Duration {
	if b.Timeout > 0 {
		return b.Timeout
	}
	return time.Hour
}

// Wait waits for tracking goroutines (tests).
func (b *Builder) Wait() { b.running.Wait() }

// JobName is the Job of a build.
func JobName(id int64) string { return "build-" + strconv.FormatInt(id, 10) }

// Repo is the registry repository of an app.
func Repo(p store.Project, a store.App) string { return p.Namespace + "/" + a.Slug }

// Request starts a build.
type Request struct {
	App     store.App
	Project store.Project
	Trigger string // manual | webhook | api
	Commit  string // "" = the head of the branch
	Message string
	Deploy  bool
	By      *int64
}

// Start resolves the commit, records the build and starts the Kaniko Job.
func (b *Builder) Start(ctx context.Context, req Request) (store.Build, error) {
	if !b.Registry.Enabled() {
		return store.Build{}, registry.ErrDisabled
	}
	src, err := b.Store.GetAppSource(ctx, req.App.ID)
	if errors.Is(err, store.ErrNotFound) {
		return store.Build{}, errors.New("this app is not built from a Git repository")
	}
	if err != nil {
		return store.Build{}, err
	}
	token, err := b.open(src.Token)
	if err != nil {
		return store.Build{}, err
	}
	commit := req.Commit
	if commit == "" {
		rctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		commit, err = b.Remote.Head(rctx, src.RepoURL, src.Branch, token)
		cancel()
		if err != nil {
			return store.Build{}, err
		}
	}
	bl, err := b.Store.CreateBuild(ctx, store.Build{AppID: req.App.ID, Trigger: req.Trigger, Branch: src.Branch, CommitSHA: commit,
		CommitMsg: req.Message, Deploy: req.Deploy, CreatedBy: req.By})
	if err != nil {
		return bl, err
	}
	tag := "build-" + strconv.Itoa(bl.Number)
	bl.Image = b.Registry.PullRef(Repo(req.Project, req.App), tag)
	_ = b.Store.SetBuildImage(ctx, bl.ID, bl.Image)
	if err := b.createJob(ctx, bl, req, src, token, tag); err != nil {
		_ = b.Store.FinishBuild(ctx, bl.ID, store.BuildFailed, "could not start the build: "+err.Error(), "", nil)
		bl.Status, bl.Message = store.BuildFailed, err.Error()
		return bl, err
	}
	b.track(bl.ID)
	return bl, nil
}

func (b *Builder) open(sealed string) (string, error) {
	if sealed == "" {
		return "", nil
	}
	return b.Box.Open(sealed)
}

// gitContext is Kaniko's Git build context: git://host/path#refs/heads/<branch>#<commit>.
func gitContext(repoURL, branch, commit string) (string, bool) {
	u, err := url.Parse(repoURL)
	if err != nil {
		return "", false
	}
	return "git://" + u.Host + u.Path + "#refs/heads/" + branch + "#" + commit, u.Scheme == "http"
}

func (b *Builder) ensureNamespace(ctx context.Context) error {
	ns := b.ns()
	labels := map[string]any{"app.kubernetes.io/part-of": "nectar", "app.kubernetes.io/managed-by": "nectar-apps",
		// Kaniko runs as root but needs no privileges: baseline is enough
		"pod-security.kubernetes.io/enforce": "baseline"}
	if _, err := b.Kube.Apply(ctx, kube.Object{"apiVersion": "v1", "kind": "Namespace", "metadata": map[string]any{"name": ns, "labels": labels}}); err != nil {
		return err
	}
	// nothing may connect to build pods
	_, err := b.Kube.Apply(ctx, kube.Object{"apiVersion": "networking.k8s.io/v1", "kind": "NetworkPolicy",
		"metadata": map[string]any{"name": "deny-ingress", "namespace": ns, "labels": map[string]any{"app.kubernetes.io/part-of": "nectar"}},
		"spec":     map[string]any{"podSelector": map[string]any{}, "policyTypes": []any{"Ingress"}}})
	return err
}

func (b *Builder) createJob(ctx context.Context, bl store.Build, req Request, src store.AppSource, token, tag string) error {
	if err := b.ensureNamespace(ctx); err != nil {
		return err
	}
	creds := []registry.Credential{b.Registry.BuildCredential()}
	if b.Creds != nil {
		extra, err := b.Creds(ctx)
		if err != nil {
			return err
		}
		creds = append(creds, extra...) // base images from private registries
	}
	ns, name := b.ns(), JobName(bl.ID)
	labels := map[string]any{"app.kubernetes.io/part-of": "nectar", "nectar.dev/build": strconv.FormatInt(bl.ID, 10), "nectar.dev/app": strconv.FormatInt(req.App.ID, 10)}
	secretData := map[string]any{"config.json": base64.StdEncoding.EncodeToString(registry.DockerConfig(creds))}
	if token != "" {
		secretData["git-password"] = base64.StdEncoding.EncodeToString([]byte(token))
	}
	if _, err := b.Kube.Apply(ctx, kube.Object{"apiVersion": "v1", "kind": "Secret", "type": "Opaque",
		"metadata": map[string]any{"name": name, "namespace": ns, "labels": labels}, "data": secretData}); err != nil {
		return err
	}
	gitCtx, plainHTTP := gitContext(src.RepoURL, src.Branch, bl.CommitSHA)
	repo := Repo(req.Project, req.App)
	push := b.Registry.PushHost()
	args := []any{
		"--context=" + gitCtx,
		"--dockerfile=" + src.Dockerfile,
		"--destination=" + b.Registry.PushRef(repo, tag),
		"--insecure-registry=" + push, // the built-in registry speaks plain HTTP inside the cluster
		"--cache=true", "--cache-repo=" + push + "/" + repo + "/cache",
		"--snapshot-mode=redo", "--use-new-run", "--push-retry=2",
		"--label=org.opencontainers.image.revision=" + bl.CommitSHA,
		"--label=org.opencontainers.image.source=" + src.RepoURL,
	}
	if src.ContextDir != "" {
		args = append(args, "--context-sub-path="+src.ContextDir)
	}
	env := []any{map[string]any{"name": "DOCKER_CONFIG", "value": "/kaniko/.docker"}}
	if token != "" {
		env = append(env,
			map[string]any{"name": "GIT_USERNAME", "value": GitUser},
			map[string]any{"name": "GIT_PASSWORD", "valueFrom": map[string]any{"secretKeyRef": map[string]any{"name": name, "key": "git-password"}}})
	}
	if plainHTTP {
		env = append(env, map[string]any{"name": "GIT_PULL_METHOD", "value": "http"})
	}
	cpu, mem := src.BuildCPU, src.BuildMemory
	if cpu <= 0 {
		cpu = 2000
	}
	if mem <= 0 {
		mem = 2048
	}
	job := kube.Object{"apiVersion": "batch/v1", "kind": "Job",
		"metadata": map[string]any{"name": name, "namespace": ns, "labels": labels},
		"spec": map[string]any{
			"backoffLimit":            0,
			"activeDeadlineSeconds":   int64(b.timeout() / time.Second),
			"ttlSecondsAfterFinished": 86400,
			"template": map[string]any{
				"metadata": map[string]any{"labels": labels},
				"spec": map[string]any{
					"restartPolicy":                "Never",
					"automountServiceAccountToken": false,
					"containers": []any{map[string]any{
						"name": "kaniko", "image": b.image(), "args": args, "env": env,
						"volumeMounts": []any{map[string]any{"name": "docker", "mountPath": "/kaniko/.docker", "readOnly": true}},
						"resources": map[string]any{
							"requests": map[string]any{"cpu": "250m", "memory": "512Mi"},
							"limits":   map[string]any{"cpu": fmt.Sprintf("%dm", cpu), "memory": fmt.Sprintf("%dMi", mem)},
						},
					}},
					"volumes": []any{map[string]any{"name": "docker", "secret": map[string]any{"secretName": name,
						"items": []any{map[string]any{"key": "config.json", "path": "config.json"}}}}},
				},
			},
		}}
	_, err := b.Kube.Apply(ctx, job)
	return err
}

// track waits for a build's Job in the background.
func (b *Builder) track(id int64) {
	b.running.Add(1)
	go func() {
		defer b.running.Done()
		b.wait(context.Background(), id)
	}()
}

// Recover resumes tracking builds that were running when the panel stopped.
func (b *Builder) Recover(ctx context.Context) {
	list, err := b.Store.RunningBuilds(ctx)
	if err != nil {
		return
	}
	for _, bl := range list {
		b.track(bl.ID)
	}
}

func (b *Builder) wait(ctx context.Context, id int64) {
	ns, name := b.ns(), JobName(id)
	deadline := time.Now().Add(b.timeout() + 5*time.Minute)
	missing := 0
	for {
		b.mu.Lock()
		canceled := b.cancel[id]
		b.mu.Unlock()
		if canceled {
			logs := b.logs(ctx, id)
			_ = b.Kube.Delete(ctx, "Job", ns, name)
			_ = b.Kube.Delete(ctx, "Secret", ns, name)
			_ = b.Store.FinishBuild(ctx, id, store.BuildCanceled, "canceled", logs, nil)
			return
		}
		var job kube.Object
		err := b.Kube.GetObject(ctx, "Job", ns, name, &job)
		if kube.IsNotFound(err) {
			if missing++; missing > 10 {
				_ = b.Store.FinishBuild(ctx, id, store.BuildFailed, "the build job disappeared", "", nil)
				return
			}
		} else if err == nil {
			missing = 0
			ok, failed := kube.Int(job, "status", "succeeded") > 0, kube.Int(job, "status", "failed") > 0
			for _, c := range kube.Slice(job, "status", "conditions") {
				if kube.Str(c, "type") == "Failed" && kube.Str(c, "status") == "True" {
					failed = true
				}
			}
			if ok || failed {
				b.finish(ctx, id, ok)
				return
			}
			if msg := b.stuck(ctx, id); msg != "" {
				logs := b.logs(ctx, id)
				_ = b.Kube.Delete(ctx, "Job", ns, name)
				_ = b.Store.FinishBuild(ctx, id, store.BuildFailed, msg, logs, nil)
				return
			}
		}
		if time.Now().After(deadline) {
			_ = b.Store.FinishBuild(ctx, id, store.BuildFailed, "the build did not finish in time", b.logs(ctx, id), nil)
			return
		}
		time.Sleep(b.poll())
	}
}

// stuck explains a build pod that cannot start (e.g. the Kaniko image cannot be pulled).
func (b *Builder) stuck(ctx context.Context, id int64) string {
	pods, err := b.Kube.List(ctx, "Pod", b.ns(), "job-name="+JobName(id))
	if err != nil {
		return ""
	}
	for _, p := range pods {
		for _, cs := range kube.Slice(p, "status", "containerStatuses") {
			switch reason := kube.Str(cs, "state", "waiting", "reason"); reason {
			case "ImagePullBackOff", "ErrImagePull", "CreateContainerConfigError", "InvalidImageName":
				return reason + ": " + kube.Str(cs, "state", "waiting", "message")
			}
		}
	}
	return ""
}

// logs returns the build output (the last 5000 lines).
func (b *Builder) logs(ctx context.Context, id int64) string {
	pods, err := b.Kube.List(ctx, "Pod", b.ns(), "job-name="+JobName(id))
	if err != nil || len(pods) == 0 {
		return ""
	}
	_, _, pod := kube.Meta(pods[0])
	out, err := b.Kube.PodLogs(ctx, b.ns(), pod, "kaniko", 5000, false)
	if err != nil {
		return ""
	}
	return out
}

// Logs returns the output of a build: live while it runs, stored afterwards.
func (b *Builder) Logs(ctx context.Context, bl store.Build) string {
	if bl.Status == store.BuildRunning {
		return b.logs(ctx, bl.ID)
	}
	return bl.Log
}

// Cancel stops a running build.
func (b *Builder) Cancel(id int64) {
	b.mu.Lock()
	if b.cancel == nil {
		b.cancel = map[int64]bool{}
	}
	b.cancel[id] = true
	b.mu.Unlock()
}

// finish records the outcome, switches the app to the new image and deploys it.
func (b *Builder) finish(ctx context.Context, id int64, ok bool) {
	logs := b.logs(ctx, id)
	ns, name := b.ns(), JobName(id)
	_ = b.Kube.Delete(ctx, "Secret", ns, name) // the Job (and its logs) stay a day for kubectl debugging
	bl, err := b.Store.GetBuild(ctx, id)
	if err != nil {
		return
	}
	if !ok {
		_ = b.Store.FinishBuild(ctx, id, store.BuildFailed, failureReason(logs), logs, nil)
		return
	}
	app, err := b.Store.GetApp(ctx, bl.AppID)
	if err != nil {
		_ = b.Store.FinishBuild(ctx, id, store.BuildFailed, "the app was deleted", logs, nil)
		return
	}
	msg := "image " + bl.Image
	var release *int64
	if bl.Deploy {
		app.Config.Image = bl.Image
		if app, err = b.Store.SaveApp(ctx, app); err != nil {
			_ = b.Store.FinishBuild(ctx, id, store.BuildFailed, err.Error(), logs, nil)
			return
		}
		if b.Deployer != nil {
			rel, err := b.Deployer.Deploy(ctx, deploy.DeployRequest{App: app, By: bl.CreatedBy})
			if err != nil {
				msg += "; the deploy did not start: " + err.Error()
			} else {
				n := int64(rel.Number)
				release = &n
				msg += fmt.Sprintf("; deploying release #%d", rel.Number)
			}
		}
	}
	_ = b.Store.FinishBuild(ctx, id, store.BuildSucceeded, msg, logs, release)
	b.prune(ctx, app)
}

// failureReason picks the most useful line of a failed Kaniko log.
func failureReason(logs string) string {
	lines := strings.Split(strings.TrimSpace(logs), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		l := lines[i]
		if strings.Contains(l, "error") || strings.Contains(l, "Error") || strings.Contains(l, "failed") {
			if len(l) > 400 {
				l = l[:400]
			}
			return strings.TrimSpace(l)
		}
	}
	if len(lines) > 0 && lines[len(lines)-1] != "" {
		return lines[len(lines)-1]
	}
	return "the build failed"
}

// prune deletes old build images of an app (keeps the newest ones and those releases still use).
func (b *Builder) prune(ctx context.Context, app store.App) {
	p, err := b.Store.GetProject(ctx, app.ProjectID)
	if err != nil {
		return
	}
	inUse := map[string]bool{}
	if i := strings.LastIndexByte(app.Config.Image, ':'); i > 0 {
		inUse[app.Config.Image[i+1:]] = true
	}
	if rels, err := b.Store.ListReleases(ctx, app.ID, 20); err == nil {
		for _, r := range rels {
			if i := strings.LastIndexByte(r.Image, ':'); i > 0 && b.Registry.Owns(r.Image) {
				inUse[r.Image[i+1:]] = true
			}
		}
	}
	pctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	if n, err := b.Registry.Prune(pctx, Repo(p, app), keepImages, inUse); err != nil && b.Log != nil {
		b.Log.Warn("registry prune", "app", app.Slug, "err", err)
	} else if n > 0 && b.Log != nil {
		b.Log.Info("registry prune", "app", app.Slug, "deleted", n)
	}
}
