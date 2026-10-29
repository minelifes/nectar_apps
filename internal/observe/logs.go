package observe

import (
	"bufio"
	"context"
	"io"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"nectar_apps/internal/kube"
)

// LogKube is the part of the Kubernetes client used for logs and events.
type LogKube interface {
	List(ctx context.Context, kind, namespace, labelSelector string) ([]kube.Object, error)
	Stream(ctx context.Context, path string) (io.ReadCloser, error)
}

// LogLine is one line of container output.
type LogLine struct {
	Pod  string    `json:"pod"`
	Time time.Time `json:"t"`
	Line string    `json:"line"`
}

const maxLineBytes = 16 << 10

// Pod is a pod of an app with what the log viewer needs.
type Pod struct {
	Name      string    `json:"name"`
	Phase     string    `json:"phase"`
	Restarts  int64     `json:"restarts"`
	Role      string    `json:"role"`
	StartedAt time.Time `json:"started_at"`
}

// AppPods lists the pods of an app, oldest first.
func AppPods(ctx context.Context, k LogKube, namespace, slug string) ([]Pod, error) {
	objs, err := k.List(ctx, "Pod", namespace, "nectar.dev/app="+slug)
	if err != nil {
		return nil, err
	}
	out := make([]Pod, 0, len(objs))
	for _, o := range objs {
		_, _, name := kube.Meta(o)
		p := Pod{Name: name, Phase: kube.Str(o, "status", "phase"), Role: kube.Str(o, "metadata", "labels", "nectar.dev/role")}
		for _, cs := range kube.Slice(o, "status", "containerStatuses") {
			p.Restarts += kube.Int(cs, "restartCount")
		}
		if t, err := time.Parse(time.RFC3339, kube.Str(o, "metadata", "creationTimestamp")); err == nil {
			p.StartedAt = t
		}
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].StartedAt.Before(out[j].StartedAt) })
	return out, nil
}

// LogPath is the API path of a pod's log. tail <= 0 = everything the kubelet still has (follow only).
func LogPath(namespace, pod string, follow, previous bool, tail int, since time.Time) string {
	q := url.Values{"timestamps": {"true"}}
	if follow {
		q.Set("follow", "true")
	}
	if previous {
		q.Set("previous", "true")
	}
	if tail > 0 {
		q.Set("tailLines", strconv.Itoa(tail))
	}
	if !since.IsZero() {
		q.Set("sinceTime", since.UTC().Format(time.RFC3339))
	}
	return "/api/v1/namespaces/" + url.PathEscape(namespace) + "/pods/" + url.PathEscape(pod) + "/log?" + q.Encode()
}

// ParseLine splits "2026-10-28T00:47:35.123456789Z message" (timestamps=true).
func ParseLine(pod, s string) LogLine {
	s = strings.TrimRight(s, "\r\n")
	if ts, rest, ok := strings.Cut(s, " "); ok {
		if t, err := time.Parse(time.RFC3339Nano, ts); err == nil {
			return LogLine{Pod: pod, Time: t, Line: rest}
		}
	}
	return LogLine{Pod: pod, Line: s}
}

// ReadLines reads log lines from r until EOF or ctx ends, calling emit for each.
func ReadLines(ctx context.Context, r io.Reader, pod string, emit func(LogLine)) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	for sc.Scan() {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		line := sc.Text()
		if len(line) > maxLineBytes {
			line = line[:maxLineBytes] + "… (truncated)"
		}
		emit(ParseLine(pod, line))
	}
	return sc.Err()
}

// TailLogs returns the last tail lines of each pod, merged by time (oldest first) and cut to tail overall.
func TailLogs(ctx context.Context, k LogKube, namespace string, pods []string, tail int, previous bool) ([]LogLine, error) {
	if tail <= 0 || tail > 10000 {
		tail = 500
	}
	var all []LogLine
	var firstErr error
	for _, p := range pods {
		rc, err := k.Stream(ctx, LogPath(namespace, p, false, previous, tail, time.Time{}))
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		_ = ReadLines(ctx, rc, p, func(l LogLine) { all = append(all, l) })
		rc.Close()
	}
	if len(all) == 0 && firstErr != nil {
		return nil, firstErr
	}
	sort.SliceStable(all, func(i, j int) bool { return all[i].Time.Before(all[j].Time) })
	if len(all) > tail {
		all = all[len(all)-tail:]
	}
	if all == nil {
		all = []LogLine{}
	}
	return all, nil
}
