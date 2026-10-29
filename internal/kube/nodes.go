package kube

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

// Resources is a resource vector in base units.
type Resources struct {
	CPUMillicores         int64 `json:"cpu_millicores"`
	MemoryBytes           int64 `json:"memory_bytes"`
	EphemeralStorageBytes int64 `json:"ephemeral_storage_bytes,omitempty"`
	Pods                  int64 `json:"pods,omitempty"`
}

// Node is the panel's view of a cluster node.
type Node struct {
	Name           string     `json:"name"`
	Ready          bool       `json:"ready"`
	Roles          []string   `json:"roles"`
	InternalIP     string     `json:"internal_ip,omitempty"`
	ExternalIP     string     `json:"external_ip,omitempty"`
	OSImage        string     `json:"os_image,omitempty"`
	Kernel         string     `json:"kernel,omitempty"`
	Arch           string     `json:"arch,omitempty"`
	KubeletVersion string     `json:"kubelet_version,omitempty"`
	Capacity       Resources  `json:"capacity"`
	Allocatable    Resources  `json:"allocatable"`
	Usage          *Resources `json:"usage,omitempty"` // nil when metrics-server is unavailable
	// Unschedulable is set for cordoned nodes; Edge marks the node DNS points to (runs nectar-gateway).
	Unschedulable bool `json:"unschedulable,omitempty"`
	Edge          bool `json:"edge,omitempty"`
}

type rawNodeList struct {
	Items []struct {
		Metadata struct {
			Name   string            `json:"name"`
			Labels map[string]string `json:"labels"`
		} `json:"metadata"`
		Spec struct {
			Unschedulable bool `json:"unschedulable"`
		} `json:"spec"`
		Status struct {
			Capacity    map[string]string `json:"capacity"`
			Allocatable map[string]string `json:"allocatable"`
			Conditions  []struct {
				Type   string `json:"type"`
				Status string `json:"status"`
			} `json:"conditions"`
			Addresses []struct {
				Type    string `json:"type"`
				Address string `json:"address"`
			} `json:"addresses"`
			NodeInfo struct {
				OSImage        string `json:"osImage"`
				KernelVersion  string `json:"kernelVersion"`
				Architecture   string `json:"architecture"`
				KubeletVersion string `json:"kubeletVersion"`
			} `json:"nodeInfo"`
		} `json:"status"`
	} `json:"items"`
}

type rawNodeMetricsList struct {
	Items []struct {
		Metadata struct {
			Name string `json:"name"`
		} `json:"metadata"`
		Usage map[string]string `json:"usage"`
	} `json:"items"`
}

// ListNodes returns all nodes with capacity, allocatable and (if metrics-server is installed) usage.
func (c *Client) ListNodes(ctx context.Context) ([]Node, error) {
	var raw rawNodeList
	if err := c.Get(ctx, "/api/v1/nodes", &raw); err != nil {
		return nil, err
	}
	usage := map[string]Resources{}
	var m rawNodeMetricsList
	if err := c.Get(ctx, "/apis/metrics.k8s.io/v1beta1/nodes", &m); err == nil {
		for _, it := range m.Items {
			usage[it.Metadata.Name] = toResources(it.Usage)
		}
	} else {
		var se *StatusError
		if !errors.As(err, &se) { // network errors are real errors; 404/503 just means no metrics-server
			return nil, err
		}
		// no metrics-server (often disabled on adopted clusters): the kubelet summary has the same numbers
		for _, it := range raw.Items {
			if sum, err := c.Summary(ctx, it.Metadata.Name); err == nil && sum.Node.CPU.UsageNanoCores != nil && sum.Node.Memory.WorkingSetBytes != nil {
				usage[it.Metadata.Name] = Resources{CPUMillicores: nanoToMilli(*sum.Node.CPU.UsageNanoCores), MemoryBytes: clamp(*sum.Node.Memory.WorkingSetBytes)}
			}
		}
	}

	nodes := make([]Node, 0, len(raw.Items))
	for _, it := range raw.Items {
		n := Node{
			Name:           it.Metadata.Name,
			OSImage:        it.Status.NodeInfo.OSImage,
			Kernel:         it.Status.NodeInfo.KernelVersion,
			Arch:           it.Status.NodeInfo.Architecture,
			KubeletVersion: it.Status.NodeInfo.KubeletVersion,
			Capacity:       toResources(it.Status.Capacity),
			Allocatable:    toResources(it.Status.Allocatable),
			Roles:          []string{},
			Unschedulable:  it.Spec.Unschedulable,
			Edge:           it.Metadata.Labels["nectar.dev/edge"] == "true",
		}
		for _, cond := range it.Status.Conditions {
			if cond.Type == "Ready" {
				n.Ready = cond.Status == "True"
			}
		}
		for _, a := range it.Status.Addresses {
			if a.Type == "InternalIP" && n.InternalIP == "" {
				n.InternalIP = a.Address
			}
			if a.Type == "ExternalIP" && n.ExternalIP == "" {
				n.ExternalIP = a.Address
			}
		}
		for l := range it.Metadata.Labels {
			if r, ok := strings.CutPrefix(l, "node-role.kubernetes.io/"); ok && r != "" {
				n.Roles = append(n.Roles, r)
			}
		}
		sort.Strings(n.Roles)
		if u, ok := usage[n.Name]; ok {
			n.Usage = &u
		}
		nodes = append(nodes, n)
	}
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].Name < nodes[j].Name })
	return nodes, nil
}

func toResources(m map[string]string) Resources {
	var r Resources
	if v, ok := m["cpu"]; ok {
		if q, err := ParseQuantity(v); err == nil {
			r.CPUMillicores = int64(math.Round(q * 1000))
		}
	}
	if v, ok := m["memory"]; ok {
		if q, err := ParseQuantity(v); err == nil {
			r.MemoryBytes = int64(math.Round(q))
		}
	}
	if v, ok := m["ephemeral-storage"]; ok {
		if q, err := ParseQuantity(v); err == nil {
			r.EphemeralStorageBytes = int64(math.Round(q))
		}
	}
	if v, ok := m["pods"]; ok {
		if q, err := ParseQuantity(v); err == nil {
			r.Pods = int64(q)
		}
	}
	return r
}

var suffixes = []struct {
	s string
	m float64
}{
	// binary SI (longest first)
	{"Ki", 1 << 10}, {"Mi", 1 << 20}, {"Gi", 1 << 30}, {"Ti", 1 << 40}, {"Pi", 1 << 50}, {"Ei", 1 << 60},
	// decimal SI
	{"n", 1e-9}, {"u", 1e-6}, {"m", 1e-3}, {"k", 1e3}, {"M", 1e6}, {"G", 1e9}, {"T", 1e12}, {"P", 1e15}, {"E", 1e18},
}

// ParseQuantity parses a Kubernetes resource quantity ("250m", "1.5Gi", "128974848", "1e3", "123456n")
// into a float in base units (cores for CPU, bytes for memory).
func ParseQuantity(s string) (float64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, errors.New("empty quantity")
	}
	for _, suf := range suffixes {
		if num, ok := strings.CutSuffix(s, suf.s); ok && num != "" && !strings.ContainsAny(num[len(num)-1:], "eE") {
			v, err := strconv.ParseFloat(num, 64)
			if err != nil {
				return 0, fmt.Errorf("bad quantity %q", s)
			}
			return v * suf.m, nil
		}
	}
	v, err := strconv.ParseFloat(s, 64) // plain number or exponent form "1e3"
	if err != nil {
		return 0, fmt.Errorf("bad quantity %q", s)
	}
	return v, nil
}

// Cordon marks a node (un)schedulable.
func (c *Client) Cordon(ctx context.Context, node string, on bool) error {
	_, err := c.Patch(ctx, "Node", "", node, map[string]any{"spec": map[string]any{"unschedulable": on}})
	return err
}

// Evict asks the API server to evict a pod (respects PodDisruptionBudgets: 429 = not now).
func (c *Client) Evict(ctx context.Context, namespace, pod string) error {
	body := fmt.Sprintf(`{"apiVersion":"policy/v1","kind":"Eviction","metadata":{"name":%q,"namespace":%q}}`, pod, namespace)
	return c.Do(ctx, "POST", "/api/v1/namespaces/"+namespace+"/pods/"+pod+"/eviction", "application/json", strings.NewReader(body), nil)
}

// Summary is a node's kubelet stats summary (/stats/summary through the API server proxy).
type Summary struct {
	Node struct {
		CPU     SummaryCPU    `json:"cpu"`
		Memory  SummaryMemory `json:"memory"`
		Fs      SummaryFs     `json:"fs"`
		Network struct {
			RxBytes *uint64 `json:"rxBytes"`
			TxBytes *uint64 `json:"txBytes"`
		} `json:"network"`
	} `json:"node"`
	Pods []struct {
		PodRef struct {
			Name      string `json:"name"`
			Namespace string `json:"namespace"`
		} `json:"podRef"`
		CPU    SummaryCPU    `json:"cpu"`
		Memory SummaryMemory `json:"memory"`
	} `json:"pods"`
}

type SummaryCPU struct {
	UsageNanoCores *uint64 `json:"usageNanoCores"`
}

type SummaryMemory struct {
	WorkingSetBytes *uint64 `json:"workingSetBytes"`
}

type SummaryFs struct {
	CapacityBytes *uint64 `json:"capacityBytes"`
	UsedBytes     *uint64 `json:"usedBytes"`
}

func (c *Client) Summary(ctx context.Context, node string) (Summary, error) {
	var sum Summary
	err := c.Get(ctx, "/api/v1/nodes/"+url.PathEscape(node)+"/proxy/stats/summary", &sum)
	return sum, err
}

// PodUsage is one pod's CPU and memory from a kubelet summary.
type PodUsage struct {
	Namespace, Name string
	Resources
}

// PodUsage returns the usage of every pod on the node that reports it.
func (s Summary) PodUsage() []PodUsage {
	out := make([]PodUsage, 0, len(s.Pods))
	for _, p := range s.Pods {
		if p.CPU.UsageNanoCores == nil || p.Memory.WorkingSetBytes == nil {
			continue
		}
		out = append(out, PodUsage{Namespace: p.PodRef.Namespace, Name: p.PodRef.Name,
			Resources: Resources{CPUMillicores: nanoToMilli(*p.CPU.UsageNanoCores), MemoryBytes: clamp(*p.Memory.WorkingSetBytes)}})
	}
	return out
}

func nanoToMilli(n uint64) int64 { return clamp((n + 500_000) / 1_000_000) }

func clamp(v uint64) int64 {
	if v > math.MaxInt64 {
		return math.MaxInt64
	}
	return int64(v)
}
