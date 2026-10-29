package deploy

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"time"

	"nectar_apps/internal/kube"
	"nectar_apps/internal/runtimes"
	"nectar_apps/internal/store"
)

// CapacityError explains why an app does not fit.
type CapacityError struct {
	Reason string
}

func (e *CapacityError) Error() string { return e.Reason }

// CheckCapacity verifies that the app's requests fit on the server and within the project quota.
// Pods of the same app are counted as freed (they are replaced by the new release).
func (d *Deployer) CheckCapacity(ctx context.Context, a store.App) error {
	replicas := int64(a.Config.Replicas)
	if a.Kind == runtimes.KindCron || a.Kind == runtimes.KindTask {
		replicas = 1
	}
	if replicas <= 0 {
		return nil
	}
	needCPU := a.Config.Resources.CPUMillicores * replicas
	needMem := a.Config.Resources.MemoryMB * replicas * 1 << 20

	nodes, err := d.kube.ListNodes(ctx)
	if err != nil || len(nodes) == 0 {
		return nil // can't tell; let Kubernetes decide
	}
	var allocCPU, allocMem int64
	for _, n := range nodes {
		if n.Ready {
			allocCPU += n.Allocatable.CPUMillicores
			allocMem += n.Allocatable.MemoryBytes
		}
	}
	pods, err := d.kube.List(ctx, "Pod", "", "")
	if err != nil {
		return nil
	}
	p, _ := d.store.GetProject(ctx, a.ProjectID)
	var usedCPU, usedMem, nsCPU, nsMem int64
	for _, pod := range pods {
		phase := kube.Str(pod, "status", "phase")
		if phase == "Succeeded" || phase == "Failed" {
			continue
		}
		_, ns, _ := kube.Meta(pod)
		if ns == p.Namespace && kube.Str(pod, "metadata", "labels", LabelApp) == a.Slug && kube.Str(pod, "metadata", "labels", LabelRole) == "workload" {
			continue // replaced by this deploy
		}
		c, m := podRequests(pod)
		usedCPU += c
		usedMem += m
		if ns == p.Namespace {
			nsCPU += c
			nsMem += m
		}
	}
	freeCPU, freeMem := allocCPU-usedCPU, allocMem-usedMem
	if needCPU > freeCPU {
		return &CapacityError{fmt.Sprintf("not enough CPU on the server: needs %s, %s free of %s",
			cores(needCPU), cores(max(freeCPU, 0)), cores(allocCPU))}
	}
	if needMem > freeMem {
		return &CapacityError{fmt.Sprintf("not enough memory on the server: needs %s, %s free of %s",
			mib(needMem), mib(max(freeMem, 0)), mib(allocMem))}
	}
	if q := p.Quota; q != nil {
		if q.CPUMillicores > 0 && nsCPU+needCPU > q.CPUMillicores {
			return &CapacityError{fmt.Sprintf("project CPU quota exceeded: %s used + %s needed > %s",
				cores(nsCPU), cores(needCPU), cores(q.CPUMillicores))}
		}
		if q.MemoryMB > 0 && nsMem+needMem > q.MemoryMB<<20 {
			return &CapacityError{fmt.Sprintf("project memory quota exceeded: %s used + %s needed > %s",
				mib(nsMem), mib(needMem), mib(q.MemoryMB<<20))}
		}
	}
	return nil
}

// podRequests sums the CPU (millicores) and memory (bytes) requests of a pod's containers.
func podRequests(pod kube.Object) (cpu, mem int64) {
	for _, c := range kube.Slice(pod, "spec", "containers") {
		if v := kube.Str(c, "resources", "requests", "cpu"); v != "" {
			if q, err := kube.ParseQuantity(v); err == nil {
				cpu += int64(math.Round(q * 1000))
			}
		}
		if v := kube.Str(c, "resources", "requests", "memory"); v != "" {
			if q, err := kube.ParseQuantity(v); err == nil {
				mem += int64(q)
			}
		}
	}
	return
}

func cores(m int64) string { return strconv.FormatFloat(float64(m)/1000, 'f', -1, 64) + " CPU" }

func mib(b int64) string {
	if b >= 1<<30 {
		return strconv.FormatFloat(float64(b)/(1<<30), 'f', 1, 64) + " GiB"
	}
	return strconv.FormatInt(b>>20, 10) + " MiB"
}

// ------------------------------------------------------------------ status --

// PodStatus is the live state of one pod.
type PodStatus struct {
	Name      string     `json:"name"`
	Phase     string     `json:"phase"`
	Ready     bool       `json:"ready"`
	Restarts  int64      `json:"restarts"`
	Release   string     `json:"release"`
	Role      string     `json:"role"`
	Node      string     `json:"node,omitempty"`
	Problem   string     `json:"problem,omitempty"`
	StartedAt *time.Time `json:"started_at,omitempty"`
}

// LiveStatus is the live state of an app in the cluster.
type LiveStatus struct {
	Replicas  int64        `json:"replicas"`
	Ready     int64        `json:"ready"`
	Updated   int64        `json:"updated"`
	Pods      []PodStatus  `json:"pods"`
	NodePorts []ExposeInfo `json:"node_ports,omitempty"`
}

// ExposeInfo is an exposed port with the server port assigned by Kubernetes.
type ExposeInfo struct {
	Port     int64  `json:"port"`
	Protocol string `json:"protocol"`
	NodePort int64  `json:"node_port"`
}

// Status reads the live state of an app.
func (d *Deployer) Status(ctx context.Context, a store.App, p store.Project) (LiveStatus, error) {
	st := LiveStatus{Pods: []PodStatus{}}
	if a.Kind == runtimes.KindWeb || a.Kind == runtimes.KindWorker {
		var dep kube.Object
		if err := d.kube.GetObject(ctx, "Deployment", p.Namespace, a.Slug, &dep); err == nil {
			st.Replicas = kube.Int(dep, "spec", "replicas")
			st.Ready = kube.Int(dep, "status", "readyReplicas")
			st.Updated = kube.Int(dep, "status", "updatedReplicas")
		} else if !kube.IsNotFound(err) {
			return st, err
		}
	}
	pods, err := d.kube.List(ctx, "Pod", p.Namespace, LabelApp+"="+a.Slug)
	if err != nil && !kube.IsNotFound(err) {
		return st, err
	}
	for _, pod := range pods {
		_, _, name := kube.Meta(pod)
		ps := PodStatus{
			Name:    name,
			Phase:   kube.Str(pod, "status", "phase"),
			Release: kube.Str(pod, "metadata", "labels", LabelRelease),
			Role:    kube.Str(pod, "metadata", "labels", LabelRole),
			Node:    kube.Str(pod, "spec", "nodeName"),
			Problem: podProblem(pod),
		}
		for _, cs := range kube.Slice(pod, "status", "containerStatuses") {
			ps.Restarts += kube.Int(cs, "restartCount")
			if kube.Str(cs, "name") == "app" {
				ps.Ready, _ = kube.Field(cs, "ready").(bool)
			}
		}
		if t, err := time.Parse(time.RFC3339, kube.Str(pod, "status", "startTime")); err == nil {
			ps.StartedAt = &t
		}
		st.Pods = append(st.Pods, ps)
	}
	if len(a.Config.Expose) > 0 {
		var svc kube.Object
		if err := d.kube.GetObject(ctx, "Service", p.Namespace, a.Slug+"-ports", &svc); err == nil {
			for _, port := range kube.Slice(svc, "spec", "ports") {
				st.NodePorts = append(st.NodePorts, ExposeInfo{Port: kube.Int(port, "port"), Protocol: kube.Str(port, "protocol"), NodePort: kube.Int(port, "nodePort")})
			}
		}
	}
	return st, nil
}
