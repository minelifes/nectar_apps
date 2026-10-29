package observe

import (
	"context"
	"sort"
	"strings"
	"time"

	"nectar_apps/internal/kube"
)

// Event is a Kubernetes event in the panel's terms.
type Event struct {
	Type      string    `json:"type"` // Normal | Warning
	Reason    string    `json:"reason"`
	Message   string    `json:"message"`
	Object    string    `json:"object"` // Kind/name
	Namespace string    `json:"namespace,omitempty"`
	Count     int64     `json:"count"`
	First     time.Time `json:"first"`
	Last      time.Time `json:"last"`
}

// AppEvents returns the events of an app's objects (Deployment, ReplicaSets, Pods, Jobs named <slug>…),
// newest first.
func AppEvents(ctx context.Context, k LogKube, namespace, slug string, limit int) ([]Event, error) {
	objs, err := k.List(ctx, "Event", namespace, "")
	if err != nil {
		return nil, err
	}
	return toEvents(objs, limit, func(name string) bool { return name == slug || strings.HasPrefix(name, slug+"-") }), nil
}

// WarningEvents returns recent Warning events in all namespaces (server page), newest first.
func WarningEvents(ctx context.Context, k LogKube, limit int) ([]Event, error) {
	objs, err := k.List(ctx, "Event", "", "")
	if err != nil {
		return nil, err
	}
	var warn []kube.Object
	for _, o := range objs {
		if kube.Str(o, "type") == "Warning" {
			warn = append(warn, o)
		}
	}
	return toEvents(warn, limit, nil), nil
}

func toEvents(objs []kube.Object, limit int, match func(name string) bool) []Event {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	out := []Event{}
	for _, o := range objs {
		name := kube.Str(o, "involvedObject", "name")
		if match != nil && !match(name) {
			continue
		}
		e := Event{
			Type:      kube.Str(o, "type"),
			Reason:    kube.Str(o, "reason"),
			Message:   strings.TrimSpace(kube.Str(o, "message")),
			Object:    kube.Str(o, "involvedObject", "kind") + "/" + name,
			Namespace: kube.Str(o, "involvedObject", "namespace"),
			Count:     kube.Int(o, "count"),
		}
		e.First = eventTime(o, "firstTimestamp")
		e.Last = eventTime(o, "lastTimestamp")
		if e.Last.IsZero() {
			e.Last = eventTime(o, "eventTime")
		}
		if e.First.IsZero() {
			e.First = e.Last
		}
		if e.Count == 0 {
			e.Count = 1
		}
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Last.After(out[j].Last) })
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

func eventTime(o kube.Object, field string) time.Time {
	s := kube.Str(o, field)
	if s == "" {
		return time.Time{}
	}
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t
	}
	return time.Time{}
}
