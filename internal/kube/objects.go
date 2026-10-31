package kube

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// Object is a Kubernetes object as generic JSON.
type Object = map[string]any

// FieldManager is the server-side-apply field manager of the panel.
const FieldManager = "nectar-apps"

type resource struct {
	prefix     string // /api/v1 or /apis/<group>/<version>
	plural     string
	namespaced bool
}

// resources lists the kinds the panel manages.
var resources = map[string]resource{
	"Namespace":             {"/api/v1", "namespaces", false},
	"ServiceAccount":        {"/api/v1", "serviceaccounts", true},
	"Secret":                {"/api/v1", "secrets", true},
	"ConfigMap":             {"/api/v1", "configmaps", true},
	"Service":               {"/api/v1", "services", true},
	"PersistentVolumeClaim": {"/api/v1", "persistentvolumeclaims", true},
	"PersistentVolume":      {"/api/v1", "persistentvolumes", false},
	"ResourceQuota":         {"/api/v1", "resourcequotas", true},
	"LimitRange":            {"/api/v1", "limitranges", true},
	"Pod":                   {"/api/v1", "pods", true},
	"Deployment":            {"/apis/apps/v1", "deployments", true},
	"Job":                   {"/apis/batch/v1", "jobs", true},
	"CronJob":               {"/apis/batch/v1", "cronjobs", true},
	"NetworkPolicy":         {"/apis/networking.k8s.io/v1", "networkpolicies", true},
	"Ingress":               {"/apis/networking.k8s.io/v1", "ingresses", true},
	"IngressClass":          {"/apis/networking.k8s.io/v1", "ingressclasses", false},
	"DaemonSet":             {"/apis/apps/v1", "daemonsets", true},
	"StatefulSet":           {"/apis/apps/v1", "statefulsets", true},
	"Endpoints":             {"/api/v1", "endpoints", true},
	"Event":                 {"/api/v1", "events", true},
	// discovery and migration of existing clusters (adopt mode); missing CRDs answer 404
	"CustomResourceDefinition": {"/apis/apiextensions.k8s.io/v1", "customresourcedefinitions", false},
	"HelmChart":                {"/apis/helm.cattle.io/v1", "helmcharts", true},
	"HelmChartConfig":          {"/apis/helm.cattle.io/v1", "helmchartconfigs", true},
	"IngressRoute":             {"/apis/traefik.io/v1alpha1", "ingressroutes", true},
	"Middleware":               {"/apis/traefik.io/v1alpha1", "middlewares", true},
	"IngressRoute.containo.us": {"/apis/traefik.containo.us/v1alpha1", "ingressroutes", true},
	"Middleware.containo.us":   {"/apis/traefik.containo.us/v1alpha1", "middlewares", true},
	"HTTPRoute":                {"/apis/gateway.networking.k8s.io/v1", "httproutes", true},
	"Gateway":                  {"/apis/gateway.networking.k8s.io/v1", "gateways", true},
	"GatewayClass":             {"/apis/gateway.networking.k8s.io/v1", "gatewayclasses", false},
	"IngressRouteTCP":          {"/apis/traefik.io/v1alpha1", "ingressroutetcps", true},
	"IngressRouteUDP":          {"/apis/traefik.io/v1alpha1", "ingressrouteudps", true},
	"Node":                     {"/api/v1", "nodes", false},
	"StorageClass":             {"/apis/storage.k8s.io/v1", "storageclasses", false},
	"Certificate":              {"/apis/cert-manager.io/v1", "certificates", true},
	"ClusterIssuer":            {"/apis/cert-manager.io/v1", "clusterissuers", false},
	"VirtualService":           {"/apis/networking.istio.io/v1beta1", "virtualservices", true},
	// k3s upgrades (system-upgrade-controller)
	"Plan": {"/apis/upgrade.cattle.io/v1", "plans", true},
}

// HasKind reports whether the client knows a kind.
func HasKind(kind string) bool { _, ok := resources[kind]; return ok }

// Path returns the API path of a named object (name may be "" for the collection).
func Path(kind, namespace, name string) (string, error) {
	r, ok := resources[kind]
	if !ok {
		return "", fmt.Errorf("kube: unsupported kind %q", kind)
	}
	p := r.prefix
	if r.namespaced {
		if namespace == "" {
			return "", fmt.Errorf("kube: %s needs a namespace", kind)
		}
		p += "/namespaces/" + url.PathEscape(namespace)
	}
	p += "/" + r.plural
	if name != "" {
		p += "/" + url.PathEscape(name)
	}
	return p, nil
}

// Meta returns kind, namespace and name of an object.
func Meta(o Object) (kind, namespace, name string) {
	kind, _ = o["kind"].(string)
	if md, ok := o["metadata"].(map[string]any); ok {
		namespace, _ = md["namespace"].(string)
		name, _ = md["name"].(string)
	}
	return
}

// Apply creates or updates an object with server-side apply (the panel owns the fields it sets).
func (c *Client) Apply(ctx context.Context, o Object) (Object, error) {
	kind, ns, name := Meta(o)
	p, err := Path(kind, ns, name)
	if err != nil {
		return nil, err
	}
	body, err := json.Marshal(o)
	if err != nil {
		return nil, err
	}
	var out Object
	err = c.Do(ctx, http.MethodPatch, p+"?fieldManager="+FieldManager+"&force=true", "application/apply-patch+yaml", bytes.NewReader(body), &out)
	if err != nil {
		return nil, fmt.Errorf("apply %s %s/%s: %w", kind, ns, name, err)
	}
	return out, nil
}

// Patch applies a JSON merge patch (RFC 7386) to an object the panel does not own (e.g. the Service of
// an existing ingress controller during a migration). Fields set to nil are removed.
func (c *Client) Patch(ctx context.Context, kind, namespace, name string, patch any) (Object, error) {
	p, err := Path(kind, namespace, name)
	if err != nil {
		return nil, err
	}
	body, err := json.Marshal(patch)
	if err != nil {
		return nil, err
	}
	var out Object
	if err := c.Do(ctx, http.MethodPatch, p, "application/merge-patch+json", bytes.NewReader(body), &out); err != nil {
		return nil, fmt.Errorf("patch %s %s/%s: %w", kind, namespace, name, err)
	}
	return out, nil
}

// GetObject fetches an object into out. Returns a StatusError (404 → IsNotFound) if missing.
func (c *Client) GetObject(ctx context.Context, kind, namespace, name string, out any) error {
	p, err := Path(kind, namespace, name)
	if err != nil {
		return err
	}
	return c.Get(ctx, p, out)
}

// Delete deletes an object (background propagation). Missing objects are not an error.
func (c *Client) Delete(ctx context.Context, kind, namespace, name string) error {
	p, err := Path(kind, namespace, name)
	if err != nil {
		return err
	}
	body := strings.NewReader(`{"kind":"DeleteOptions","apiVersion":"v1","propagationPolicy":"Background"}`)
	err = c.Do(ctx, http.MethodDelete, p, "application/json", body, nil)
	if IsNotFound(err) {
		return nil
	}
	return err
}

// List returns the items of a kind in a namespace (all namespaces when namespace is "" for
// namespaced kinds), filtered by a label selector (may be empty).
func (c *Client) List(ctx context.Context, kind, namespace, labelSelector string) ([]Object, error) {
	r, ok := resources[kind]
	if !ok {
		return nil, fmt.Errorf("kube: unsupported kind %q", kind)
	}
	p := r.prefix
	if r.namespaced && namespace != "" {
		p += "/namespaces/" + url.PathEscape(namespace)
	}
	p += "/" + r.plural
	if labelSelector != "" {
		p += "?labelSelector=" + url.QueryEscape(labelSelector)
	}
	var list struct {
		Items []Object `json:"items"`
	}
	if err := c.Get(ctx, p, &list); err != nil {
		return nil, err
	}
	for _, it := range list.Items { // list items omit kind; fill it in for callers
		it["kind"] = strings.TrimSuffix(kind, ".containo.us")
	}
	return list.Items, nil
}

// PodLogs returns the last tailLines lines of a container's log.
func (c *Client) PodLogs(ctx context.Context, namespace, pod, container string, tailLines int, previous bool) (string, error) {
	p, _ := Path("Pod", namespace, pod)
	q := url.Values{"tailLines": {fmt.Sprint(tailLines)}}
	if container != "" {
		q.Set("container", container)
	}
	if previous {
		q.Set("previous", "true")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+p+"/log?"+q.Encode(), nil)
	if err != nil {
		return "", err
	}
	if tok, err := c.bearer(); err != nil {
		return "", err
	} else if tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 256<<10))
	if resp.StatusCode != http.StatusOK {
		return "", &StatusError{Code: resp.StatusCode, Message: strings.TrimSpace(string(b))}
	}
	return string(b), nil
}

// ---------------------------------------------------------- JSON helpers --

// Str reads a nested string field, e.g. Str(o, "status", "phase").
func Str(o Object, path ...string) string {
	v, _ := Field(o, path...).(string)
	return v
}

// Int reads a nested number field.
func Int(o Object, path ...string) int64 {
	switch v := Field(o, path...).(type) {
	case float64:
		return int64(v)
	case int64:
		return v
	case int:
		return int64(v)
	}
	return 0
}

// Field reads a nested field (nil if missing).
func Field(o Object, path ...string) any {
	var cur any = o
	for _, p := range path {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil
		}
		cur = m[p]
	}
	return cur
}

// Slice reads a nested array of objects.
func Slice(o Object, path ...string) []Object {
	arr, _ := Field(o, path...).([]any)
	out := make([]Object, 0, len(arr))
	for _, it := range arr {
		if m, ok := it.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}
