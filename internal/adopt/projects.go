package adopt

import (
	"context"
	"sort"
	"strings"

	"nectar_apps/internal/kube"
)

// ProjectRequestsConfigMap lists (data "namespaces", space separated) the namespaces the installer asked
// the panel to adopt as projects. The installer has no panel login, so the panel picks the list up itself
// and deletes the ConfigMap once every request is handled.
const ProjectRequestsConfigMap = "nectar-adopt-projects"

// AdoptableNamespaces returns the namespaces that can become Nectar projects: not k3s's own, not Nectar's
// system namespace, not already a project. "default" is included only when something of the user runs in it.
func AdoptableNamespaces(ctx context.Context, k Kube, systemNamespace string) ([]string, error) {
	nss, err := k.List(ctx, "Namespace", "", "")
	if err != nil {
		return nil, err
	}
	deps, _ := list(ctx, k, "Deployment")
	sts, _ := list(ctx, k, "StatefulSet")
	used := map[string]bool{}
	for _, o := range append(deps, sts...) {
		used[kube.Str(o, "metadata", "namespace")] = true
	}
	var out []string
	for _, ns := range nss {
		name := kube.Str(ns, "metadata", "name")
		if strings.HasPrefix(name, "kube-") || name == systemNamespace ||
			kube.Str(ns, "metadata", "labels", "app.kubernetes.io/managed-by") == "nectar-apps" ||
			(name == "default" && !used[name]) {
			continue
		}
		out = append(out, name)
	}
	sort.Strings(out)
	return out, nil
}

// RequestProjects adds namespaces to the panel's adoption requests.
func RequestProjects(ctx context.Context, k Kube, systemNamespace string, names []string) error {
	seen := map[string]bool{}
	var all []string
	var cm kube.Object
	if err := k.GetObject(ctx, "ConfigMap", systemNamespace, ProjectRequestsConfigMap, &cm); err == nil {
		names = append(strings.Fields(kube.Str(cm, "data", "namespaces")), names...)
	}
	for _, n := range names {
		if n != "" && !seen[n] {
			seen[n] = true
			all = append(all, n)
		}
	}
	_, err := k.Apply(ctx, kube.Object{
		"apiVersion": "v1", "kind": "ConfigMap",
		"metadata": map[string]any{"name": ProjectRequestsConfigMap, "namespace": systemNamespace,
			"labels": map[string]any{"app.kubernetes.io/part-of": "nectar"}},
		"data": map[string]any{"namespaces": strings.Join(all, " ")},
	})
	return err
}
