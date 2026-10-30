// Package volfiles browses and edits the files of a volume (PVC) through a short-lived helper pod.
//
// The helper pod ("nectar-files-<claim>") mounts the volume at /data and runs busybox; the panel runs
// busybox commands in it over the exec API. It works for any app image (distroless ones too) and while
// the app is stopped. The pod runs as root so it can read every file; written files get the owner of
// their folder, so the app can still use them. It stops by itself after an hour and is deleted by the
// housekeeping (and before the volume is deleted).
package volfiles

import (
	"context"
	"errors"
	"fmt"
	"path"
	"strconv"
	"strings"
	"time"

	"nectar_apps/internal/kube"
)

// Root is where the helper pod mounts the volume.
const Root = "/data"

// Label marks helper pods (value: the claim name).
const Label = "nectar.dev/files"

// Lifetime of a helper pod.
const Lifetime = time.Hour

// MaxRead is the largest file that can be downloaded through the panel.
const MaxRead = 64 << 20

// Kube is the part of the Kubernetes client used here.
type Kube interface {
	Apply(ctx context.Context, o kube.Object) (kube.Object, error)
	GetObject(ctx context.Context, kind, namespace, name string, out any) error
	Delete(ctx context.Context, kind, namespace, name string) error
	ExecRun(ctx context.Context, namespace, pod, container string, command []string, stdin []byte, maxOut int) (kube.ExecResult, error)
}

// Browser opens helper pods.
type Browser struct {
	Kube  Kube
	Image string // busybox (deploy.InitImage)
	Poll  time.Duration
}

// Entry is a file or folder.
type Entry struct {
	Name     string    `json:"name"`
	Type     string    `json:"type"` // file | dir | link | other
	Size     int64     `json:"size"`
	Modified time.Time `json:"modified"`
	Mode     string    `json:"mode"`  // octal permissions, e.g. 644
	Owner    string    `json:"owner"` // uid:gid
}

// ErrNotFound: the path does not exist.
var ErrNotFound = errors.New("no such file or folder")

// PodName returns the helper pod of a claim (≤ 63 characters).
func PodName(claim string) string {
	n := "nectar-files-" + claim
	if len(n) > 63 {
		n = strings.TrimRight(n[:63], "-")
	}
	return n
}

// Clean turns a user path ("", "/", "logs/a.txt", "/logs/../x") into an absolute path under Root, or "" when
// it leaves the volume.
func Clean(p string) string {
	if strings.ContainsAny(p, "\x00\n\r") {
		return ""
	}
	c := path.Clean("/" + p)
	if strings.Contains(c, "/../") || strings.HasSuffix(c, "/..") {
		return ""
	}
	if c == "/" {
		return Root
	}
	return Root + c
}

// Session is an open helper pod.
type Session struct {
	b         *Browser
	namespace string
	pod       string
}

// Open starts (or reuses) the helper pod of a claim and waits until it runs.
func (b *Browser) Open(ctx context.Context, namespace, claim string) (*Session, error) {
	name := PodName(claim)
	var cur kube.Object
	err := b.Kube.GetObject(ctx, "Pod", namespace, name, &cur)
	switch {
	case err == nil && kube.Str(cur, "status", "phase") == "Running":
		return &Session{b: b, namespace: namespace, pod: name}, nil
	case err == nil && kube.Str(cur, "metadata", "deletionTimestamp") == "":
		if phase := kube.Str(cur, "status", "phase"); phase == "Succeeded" || phase == "Failed" {
			if err := b.Kube.Delete(ctx, "Pod", namespace, name); err != nil && !kube.IsNotFound(err) {
				return nil, err
			}
			if err := b.waitGone(ctx, namespace, name); err != nil {
				return nil, err
			}
			if _, err := b.Kube.Apply(ctx, helperPod(namespace, name, claim, b.Image)); err != nil {
				return nil, err
			}
		}
	case err == nil: // terminating
		if err := b.waitGone(ctx, namespace, name); err != nil {
			return nil, err
		}
		if _, err := b.Kube.Apply(ctx, helperPod(namespace, name, claim, b.Image)); err != nil {
			return nil, err
		}
	case kube.IsNotFound(err):
		if _, err := b.Kube.Apply(ctx, helperPod(namespace, name, claim, b.Image)); err != nil {
			return nil, err
		}
	default:
		return nil, err
	}
	if err := b.waitRunning(ctx, namespace, name); err != nil {
		return nil, err
	}
	return &Session{b: b, namespace: namespace, pod: name}, nil
}

// Close deletes the helper pod of a claim (before the volume is deleted).
func (b *Browser) Close(ctx context.Context, namespace, claim string) error {
	err := b.Kube.Delete(ctx, "Pod", namespace, PodName(claim))
	if kube.IsNotFound(err) {
		return nil
	}
	return err
}

func (b *Browser) poll() time.Duration {
	if b.Poll > 0 {
		return b.Poll
	}
	return time.Second
}

func (b *Browser) waitRunning(ctx context.Context, namespace, name string) error {
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	for {
		var p kube.Object
		if err := b.Kube.GetObject(ctx, "Pod", namespace, name, &p); err == nil {
			switch kube.Str(p, "status", "phase") {
			case "Running":
				return nil
			case "Failed", "Succeeded":
				return fmt.Errorf("the file browser pod stopped: %s", kube.Str(p, "status", "message"))
			}
			for _, cs := range kube.Slice(p, "status", "containerStatuses") {
				if r := kube.Str(cs, "state", "waiting", "reason"); r == "ErrImagePull" || r == "ImagePullBackOff" {
					return fmt.Errorf("the file browser pod cannot pull %s: %s", kube.Str(cs, "image"), kube.Str(cs, "state", "waiting", "message"))
				}
			}
		}
		select {
		case <-ctx.Done():
			return errors.New("the file browser pod did not start in time (is the volume attached to a node that is down?)")
		case <-time.After(b.poll()):
		}
	}
}

func (b *Browser) waitGone(ctx context.Context, namespace, name string) error {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	for {
		var p kube.Object
		if err := b.Kube.GetObject(ctx, "Pod", namespace, name, &p); kube.IsNotFound(err) {
			return nil
		}
		select {
		case <-ctx.Done():
			return errors.New("the old file browser pod is still terminating")
		case <-time.After(b.poll()):
		}
	}
}

func helperPod(namespace, name, claim, image string) kube.Object {
	return kube.Object{
		"apiVersion": "v1", "kind": "Pod",
		"metadata": map[string]any{
			"name": name, "namespace": namespace,
			"labels": map[string]any{Label: claim, "app.kubernetes.io/managed-by": "nectar-apps"},
		},
		"spec": map[string]any{
			"restartPolicy":                 "Never",
			"activeDeadlineSeconds":         int(Lifetime.Seconds()),
			"automountServiceAccountToken":  false,
			"enableServiceLinks":            false,
			"terminationGracePeriodSeconds": 1,
			"securityContext":               map[string]any{"seccompProfile": map[string]any{"type": "RuntimeDefault"}},
			"containers": []any{map[string]any{
				"name": "files", "image": image, "imagePullPolicy": "IfNotPresent",
				"command": []any{"sleep", strconv.Itoa(int(Lifetime.Seconds()))},
				// root without extra capabilities: reads and writes every file of the volume
				"securityContext": map[string]any{"allowPrivilegeEscalation": false,
					"capabilities": map[string]any{"drop": []any{"ALL"}, "add": []any{"CHOWN", "DAC_OVERRIDE", "FOWNER", "DAC_READ_SEARCH"}}},
				"resources":    map[string]any{"requests": map[string]any{"cpu": "10m", "memory": "16Mi"}, "limits": map[string]any{"memory": "128Mi"}},
				"volumeMounts": []any{map[string]any{"name": "data", "mountPath": Root}},
			}},
			"volumes": []any{map[string]any{"name": "data", "persistentVolumeClaim": map[string]any{"claimName": claim}}},
		},
	}
}

// run executes a shell script with arguments ($1 …) in the helper pod.
func (s *Session) run(ctx context.Context, script string, stdin []byte, maxOut int, args ...string) (kube.ExecResult, error) {
	cmd := append([]string{"sh", "-c", script, "sh"}, args...)
	res, err := s.b.Kube.ExecRun(ctx, s.namespace, s.pod, "files", cmd, stdin, maxOut)
	if err != nil {
		return res, err
	}
	switch res.ExitCode {
	case 0:
		return res, nil
	case 44:
		return res, ErrNotFound
	}
	msg := strings.TrimSpace(res.Stderr)
	if msg == "" {
		msg = "exit code " + strconv.Itoa(res.ExitCode)
	}
	return res, errors.New(msg)
}

// List returns the entries of a folder (folders first, then by name).
func (s *Session) List(ctx context.Context, dir string) ([]Entry, error) {
	// one line per entry: type|size|mtime|mode|uid:gid|name (busybox stat)
	res, err := s.run(ctx, `[ -d "$1" ] || exit 44; cd "$1" || exit 3
for f in .* *; do
  case "$f" in .|..) continue ;; esac
  [ -e "$f" ] || [ -L "$f" ] || continue
  stat -c '%F|%s|%Y|%a|%u:%g|%n' "./$f"
done
true`, nil, 4<<20, dir)
	if err != nil {
		return nil, err
	}
	return parseList(string(res.Stdout)), nil
}

func parseList(out string) []Entry {
	var dirs, files []Entry
	for _, line := range strings.Split(out, "\n") {
		parts := strings.SplitN(line, "|", 6)
		if len(parts) != 6 || parts[5] == "" {
			continue
		}
		e := Entry{Name: strings.TrimPrefix(parts[5], "./"), Mode: parts[3], Owner: parts[4]}
		e.Size, _ = strconv.ParseInt(parts[1], 10, 64)
		if t, err := strconv.ParseInt(parts[2], 10, 64); err == nil {
			e.Modified = time.Unix(t, 0).UTC()
		}
		switch parts[0] {
		case "directory":
			e.Type = "dir"
			dirs = append(dirs, e)
			continue
		case "regular file", "regular empty file":
			e.Type = "file"
		case "symbolic link":
			e.Type = "link"
		default:
			e.Type = "other"
		}
		files = append(files, e)
	}
	return append(dirs, files...)
}

// Read returns a file's content (at most max bytes; ErrTooLarge above).
func (s *Session) Read(ctx context.Context, file string, max int) ([]byte, error) {
	res, err := s.run(ctx, `[ -f "$1" ] || exit 44; [ "$(stat -c %s "$1")" -le "$2" ] || { echo "the file is larger than $2 bytes" >&2; exit 3; }; cat "$1"`,
		nil, max+1, file, strconv.Itoa(max))
	if errors.Is(err, kube.ErrOutputTooLarge) {
		return nil, fmt.Errorf("the file is larger than %d MB", max>>20)
	}
	return res.Stdout, err
}

// Write replaces (or creates) a file atomically; a new file gets the owner of its folder.
func (s *Session) Write(ctx context.Context, file string, data []byte) error {
	_, err := s.run(ctx, `d=$(dirname "$1"); [ -d "$d" ] || exit 44; [ ! -d "$1" ] || { echo "a folder has this name" >&2; exit 3; }
t="$d/.nectar-upload-$$"; head -c "$2" > "$t" || { rm -f "$t"; exit 3; }
if [ -e "$1" ]; then chown "$(stat -c %u:%g "$1")" "$t"; chmod "$(stat -c %a "$1")" "$t"; else chown "$(stat -c %u:%g "$d")" "$t"; fi
mv -f "$t" "$1"`, data, 4096, file, strconv.Itoa(len(data)))
	return err
}

// Mkdir creates a folder (and its parents), owned like its parent.
func (s *Session) Mkdir(ctx context.Context, dir string) error {
	_, err := s.run(ctx, `[ ! -e "$1" ] || { echo "already exists" >&2; exit 3; }; p=$(dirname "$1"); while [ ! -d "$p" ]; do p=$(dirname "$p"); done
mkdir -p "$1" && chown -R "$(stat -c %u:%g "$p")" "$1"`, nil, 4096, dir)
	return err
}

// Rename moves a file or folder inside the volume.
func (s *Session) Rename(ctx context.Context, from, to string) error {
	_, err := s.run(ctx, `[ -e "$1" ] || [ -L "$1" ] || exit 44; [ ! -e "$2" ] || { echo "the target already exists" >&2; exit 3; }; mv "$1" "$2"`, nil, 4096, from, to)
	return err
}

// Remove deletes a file or a folder with everything in it.
func (s *Session) Remove(ctx context.Context, p string) error {
	if p == Root {
		return errors.New("the volume itself cannot be deleted here")
	}
	_, err := s.run(ctx, `[ -e "$1" ] || [ -L "$1" ] || exit 44; rm -rf -- "$1"`, nil, 4096, p)
	return err
}
