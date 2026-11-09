package cli

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"text/tabwriter"
)

// nodeInfo is a node from GET /api/server/nodes.
type nodeInfo struct {
	Name          string            `json:"name"`
	Ready         bool              `json:"ready"`
	Roles         []string          `json:"roles"`
	InternalIP    string            `json:"internal_ip"`
	Unschedulable bool              `json:"unschedulable"`
	Edge          bool              `json:"edge"`
	Labels        map[string]string `json:"labels"`
}

const nodeUsage = `usage:
  nectar node list
  nectar node label NODE KEY=VALUE… [KEY-]…
KEY=VALUE adds or changes a label, KEY- removes it. Apps pick nodes by labels (app Settings → Placement).`

// systemLabel mirrors kube.EditableLabel: labels of Kubernetes, k3s and Nectar are not listed or changed.
func systemLabel(key string) bool {
	prefix, _, ok := strings.Cut(key, "/")
	if !ok {
		return false
	}
	for _, sys := range []string{"kubernetes.io", "k8s.io", "k3s.io", "cattle.io", "nectar.dev", "longhorn.io"} {
		if prefix == sys || strings.HasSuffix(prefix, "."+sys) {
			return true
		}
	}
	return false
}

func userLabels(n nodeInfo) map[string]string {
	out := map[string]string{}
	for k, v := range n.Labels {
		if !systemLabel(k) {
			out[k] = v
		}
	}
	return out
}

func formatLabels(m map[string]string) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = k + "=" + m[k]
	}
	if len(parts) == 0 {
		return "-"
	}
	return strings.Join(parts, ",")
}

func nodeCommand(ctx context.Context, c *Client, args []string, env Env) error {
	if len(args) == 0 {
		return errors.New(nodeUsage)
	}
	var nodes []nodeInfo
	if err := c.getJSON(ctx, "/server/nodes", &nodes); err != nil {
		return err
	}
	switch args[0] {
	case "list", "ls":
		tw := tabwriter.NewWriter(env.Stdout, 0, 4, 2, ' ', 0)
		fmt.Fprintln(tw, "NAME\tSTATUS\tROLES\tIP\tLABELS")
		for _, n := range nodes {
			st := "NotReady"
			if n.Ready {
				st = "Ready"
			}
			if n.Unschedulable {
				st += ",cordoned"
			}
			roles := strings.Join(n.Roles, ",")
			if n.Edge {
				roles = strings.TrimPrefix(roles+",edge", ",")
			}
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", n.Name, st, roles, n.InternalIP, formatLabels(userLabels(n)))
		}
		return tw.Flush()
	case "label":
		if len(args) < 3 {
			return errors.New(nodeUsage)
		}
		var node *nodeInfo
		for i := range nodes {
			if nodes[i].Name == args[1] {
				node = &nodes[i]
			}
		}
		if node == nil {
			return fmt.Errorf("no node %s", args[1])
		}
		labels := userLabels(*node)
		for _, a := range args[2:] {
			if k, ok := strings.CutSuffix(a, "-"); ok && !strings.Contains(a, "=") {
				delete(labels, k)
				continue
			}
			k, v, ok := strings.Cut(a, "=")
			if !ok || k == "" {
				return fmt.Errorf("%q: use KEY=VALUE or KEY-", a)
			}
			labels[k] = v
		}
		if err := c.do(ctx, http.MethodPut, "/server/nodes/"+url.PathEscape(node.Name)+"/labels",
			jsonBody(map[string]any{"labels": labels}), "application/json", nil); err != nil {
			return err
		}
		fmt.Fprintf(env.Stdout, "%s: %s\n", node.Name, formatLabels(labels))
		return nil
	default:
		return errors.New(nodeUsage)
	}
}
