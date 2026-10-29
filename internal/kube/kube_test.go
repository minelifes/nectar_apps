package kube

import (
	"context"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestParseQuantity(t *testing.T) {
	cases := map[string]float64{
		"4":          4,
		"3800m":      3.8,
		"250000000n": 0.25,
		"1500u":      0.0015,
		"1Ki":        1024,
		"16Gi":       16 * (1 << 30),
		"1.5Mi":      1.5 * (1 << 20),
		"128974848":  128974848,
		"1e3":        1000,
		"2k":         2000,
		"1G":         1e9,
	}
	for in, want := range cases {
		got, err := ParseQuantity(in)
		if err != nil || math.Abs(got-want) > 1e-9*math.Max(1, want) {
			t.Errorf("ParseQuantity(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "abc", "Gi", "1.2.3Mi"} {
		if _, err := ParseQuantity(bad); err == nil {
			t.Errorf("ParseQuantity(%q) accepted", bad)
		}
	}
}

const nodesJSON = `{"items":[{"metadata":{"name":"srv1","labels":{"node-role.kubernetes.io/control-plane":"true","node-role.kubernetes.io/master":"true"}},
 "status":{"capacity":{"cpu":"4","memory":"8148840Ki","pods":"110","ephemeral-storage":"81106868Ki"},
  "allocatable":{"cpu":"4","memory":"8148840Ki","pods":"110"},
  "conditions":[{"type":"MemoryPressure","status":"False"},{"type":"Ready","status":"True"}],
  "addresses":[{"type":"InternalIP","address":"10.0.0.5"},{"type":"Hostname","address":"srv1"}],
  "nodeInfo":{"osImage":"Ubuntu 24.04.1 LTS","kernelVersion":"6.8.0","architecture":"arm64","kubeletVersion":"v1.34.1+k3s1"}}}]}`

const metricsJSON = `{"items":[{"metadata":{"name":"srv1"},"usage":{"cpu":"512345678n","memory":"2097152Ki"}}]}`

func TestListNodes(t *testing.T) {
	withMetrics := true
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/nodes":
			w.Write([]byte(nodesJSON))
		case "/apis/metrics.k8s.io/v1beta1/nodes":
			if !withMetrics {
				http.Error(w, `{"message":"the server could not find the requested resource"}`, http.StatusNotFound)
				return
			}
			w.Write([]byte(metricsJSON))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	c, _ := New(srv.URL)

	nodes, err := c.ListNodes(context.Background())
	if err != nil || len(nodes) != 1 {
		t.Fatalf("ListNodes: %v %v", nodes, err)
	}
	n := nodes[0]
	if !n.Ready || n.InternalIP != "10.0.0.5" || n.Arch != "arm64" || len(n.Roles) != 2 || n.Roles[0] != "control-plane" {
		t.Errorf("unexpected node %+v", n)
	}
	if n.Capacity.CPUMillicores != 4000 || n.Capacity.MemoryBytes != 8148840*1024 || n.Capacity.Pods != 110 {
		t.Errorf("capacity %+v", n.Capacity)
	}
	if n.Usage == nil || n.Usage.CPUMillicores != 512 || n.Usage.MemoryBytes != 2<<30 {
		t.Errorf("usage %+v", n.Usage)
	}

	withMetrics = false
	nodes, err = c.ListNodes(context.Background())
	if err != nil || nodes[0].Usage != nil {
		t.Errorf("without metrics-server: %v %+v", err, nodes)
	}
}

func TestStatusError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"kind":"Status","message":"nodes is forbidden"}`))
	}))
	defer srv.Close()
	c, _ := New(srv.URL)
	_, err := c.ListNodes(context.Background())
	se, ok := err.(*StatusError)
	if !ok || se.Code != 403 || se.Message != "nodes is forbidden" {
		t.Fatalf("err = %#v", err)
	}
	if IsNotFound(err) {
		t.Fatal("403 reported as not found")
	}
}
