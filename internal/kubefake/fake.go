// Package kubefake is an in-memory stand-in for the Kubernetes API used in tests.
package kubefake

import (
	"context"
	"encoding/json"
	"strings"
	"sync"

	"nectar_apps/internal/kube"
)

// Fake is an in-memory Kubernetes API for tests (implements deploy.Kube).
type Fake struct {
	mu      sync.Mutex
	objects map[string]kube.Object // kind/ns/name
	// Applied and Deleted record calls as "Kind/name".
	Applied []string
	Deleted []string
	Patched []string
	// behaviour
	// RolloutOK makes deployments report all replicas available.
	RolloutOK bool
	// PodIssue is a waiting reason injected into synthesized pods (e.g. "CrashLoopBackOff").
	PodIssue string
	// JobOK makes jobs succeed (otherwise they fail).
	JobOK bool
	// Nodes returned by ListNodes.
	Nodes []kube.Node
}

// New returns an empty fake cluster where rollouts and jobs succeed.
func New() *Fake {
	return &Fake{objects: map[string]kube.Object{}, RolloutOK: true, JobOK: true,
		Nodes: []kube.Node{{Name: "n1", Ready: true, InternalIP: "10.0.0.5", Allocatable: kube.Resources{CPUMillicores: 4000, MemoryBytes: 8 << 30}}}}
}

func key(kind, ns, name string) string { return kind + "/" + ns + "/" + name }

func (f *Fake) Apply(_ context.Context, o kube.Object) (kube.Object, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	b, _ := json.Marshal(o)
	var cp kube.Object
	json.Unmarshal(b, &cp)
	kind, ns, name := kube.Meta(cp)
	md := cp["metadata"].(map[string]any)
	if old, ok := f.objects[key(kind, ns, name)]; ok {
		md["generation"] = float64(kube.Int(old, "metadata", "generation") + 1)
	} else {
		md["generation"] = float64(1)
	}
	f.objects[key(kind, ns, name)] = cp
	f.Applied = append(f.Applied, kind+"/"+name)
	return cp, nil
}

func (f *Fake) GetObject(_ context.Context, kind, ns, name string, out any) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	o, ok := f.objects[key(kind, ns, name)]
	if !ok {
		return &kube.StatusError{Code: 404, Message: "not found"}
	}
	switch kind {
	case "Deployment":
		replicas := kube.Int(o, "spec", "replicas")
		st := map[string]any{"observedGeneration": kube.Int(o, "metadata", "generation"), "replicas": replicas}
		if f.RolloutOK {
			st["updatedReplicas"], st["availableReplicas"], st["readyReplicas"] = replicas, replicas, replicas
		}
		o["status"] = st
	case "Job":
		if f.JobOK {
			o["status"] = map[string]any{"succeeded": 1}
		} else {
			o["status"] = map[string]any{"failed": 1}
		}
	}
	b, _ := json.Marshal(o)
	return json.Unmarshal(b, out)
}

// Patch applies a JSON merge patch (404 if the object is missing).
func (f *Fake) Patch(_ context.Context, kind, ns, name string, patch any) (kube.Object, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	o, ok := f.objects[key(kind, ns, name)]
	if !ok {
		return nil, &kube.StatusError{Code: 404, Message: "not found"}
	}
	b, _ := json.Marshal(patch)
	var p map[string]any
	json.Unmarshal(b, &p)
	mergePatch(o, p)
	f.Patched = append(f.Patched, kind+"/"+name)
	return o, nil
}

func mergePatch(dst, patch map[string]any) {
	for k, v := range patch {
		if v == nil {
			delete(dst, k)
			continue
		}
		if pm, ok := v.(map[string]any); ok {
			if dm, ok := dst[k].(map[string]any); ok {
				mergePatch(dm, pm)
				continue
			}
			nm := map[string]any{}
			mergePatch(nm, pm)
			dst[k] = nm
			continue
		}
		dst[k] = v
	}
}

func (f *Fake) Delete(_ context.Context, kind, ns, name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.objects, key(kind, ns, name))
	f.Deleted = append(f.Deleted, kind+"/"+name)
	return nil
}

func (f *Fake) List(_ context.Context, kind, ns, sel string) ([]kube.Object, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []kube.Object
	if kind == "Pod" {
		// synthesize one pod per deployment for the newest release, with an optional issue
		for k, o := range f.objects {
			if !strings.HasPrefix(k, "Deployment/") {
				continue
			}
			labels, _ := kube.Field(o, "spec", "template", "metadata", "labels").(map[string]any)
			pod := kube.Object{"kind": "Pod", "metadata": map[string]any{"name": kube.Str(o, "metadata", "name") + "-pod", "namespace": kube.Str(o, "metadata", "namespace"), "labels": labels},
				"spec":   kube.Field(o, "spec", "template", "spec"),
				"status": map[string]any{"phase": "Running"}}
			if f.PodIssue != "" {
				pod["status"] = map[string]any{"phase": "Pending", "containerStatuses": []any{map[string]any{"name": "app",
					"state": map[string]any{"waiting": map[string]any{"reason": f.PodIssue, "message": "boom"}}}}}
			}
			if matches(pod, ns, sel) {
				out = append(out, pod)
			}
		}
		// plus pods stored directly (finished job pods in tests)
		for k, o := range f.objects {
			if strings.HasPrefix(k, "Pod/") && matches(o, ns, sel) {
				out = append(out, o)
			}
		}
		return out, nil
	}
	for k, o := range f.objects {
		if strings.HasPrefix(k, kind+"/") && matches(o, ns, sel) {
			out = append(out, o)
		}
	}
	return out, nil
}

func matches(o kube.Object, ns, sel string) bool {
	_, ons, _ := kube.Meta(o)
	if ns != "" && ons != ns {
		return false
	}
	for _, term := range strings.Split(sel, ",") {
		if term == "" {
			continue
		}
		k, v, _ := strings.Cut(term, "=")
		if kube.Str(o, "metadata", "labels", k) != v {
			return false
		}
	}
	return true
}

func (f *Fake) ListNodes(context.Context) ([]kube.Node, error) {
	return f.Nodes, nil
}

func (f *Fake) PodLogs(context.Context, string, string, string, int, bool) (string, error) {
	return "Exception in thread main", nil
}

// Get returns a stored object (nil if missing).
func (f *Fake) Get(kind, ns, name string) kube.Object {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.objects[key(kind, ns, name)]
}

// Has reports whether an object exists.
func (f *Fake) Has(kind, ns, name string) bool { return f.Get(kind, ns, name) != nil }
