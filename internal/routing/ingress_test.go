package routing

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"io"
	"log/slog"
	"math/big"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"nectar_apps/internal/kube"
	"nectar_apps/internal/kubefake"
	"nectar_apps/internal/store"
)

func TestIngressProvider(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, "sqlite", store.DSN(filepath.Join(t.TempDir(), "t.db")))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	k := kubefake.New()
	prov := &IngressProvider{Kube: k, ClassName: "traefik", ClusterIssuer: "letsencrypt"}
	s := New(st, prov, slog.New(slog.NewTextHandler(io.Discard, nil)))

	p, _ := st.CreateProject(ctx, "shop", "Shop", nil)
	app, _ := st.CreateApp(ctx, store.App{ProjectID: p.ID, Slug: "api", Name: "API", Kind: "web", RuntimeID: "java",
		Config: store.AppConfig{Port: 8080, Replicas: 1}})
	rel, _ := st.CreateRelease(ctx, app.ID, nil, "img", app.Config, nil)
	st.SetReleaseStatus(ctx, rel.ID, store.ReleaseLive, "")
	d, _ := st.CreateDomain(ctx, store.Domain{AppID: app.ID, Host: "shop.example.com", PathPrefix: "/api", TLS: true, ForceHTTPS: true})
	redir, _ := st.CreateDomain(ctx, store.Domain{AppID: app.ID, Host: "www.shop.example.com", TLS: true,
		DomainOptions: store.DomainOptions{Kind: store.DomainRedirect, RedirectTo: "https://shop.example.com"}})

	if err := s.Sync(ctx); err == nil {
		t.Fatal("expected an error for the redirect domain")
	}
	ing := k.Get("Ingress", "prj-shop", ingressName(d.ID))
	if ing == nil {
		t.Fatal("ingress missing")
	}
	rule := kube.Slice(ing, "spec", "rules")[0]
	path := kube.Slice(rule, "http", "paths")[0]
	if kube.Str(rule, "host") != "shop.example.com" || kube.Str(path, "path") != "/api" ||
		kube.Str(path, "backend", "service", "name") != "api" || kube.Int(path, "backend", "service", "port", "number") != 8080 ||
		kube.Str(ing, "spec", "ingressClassName") != "traefik" || kube.Str(ing, "metadata", "annotations", annotationIssuer) != "letsencrypt" {
		t.Fatalf("ingress: %v", ing)
	}
	if rd, _ := st.GetDomain(ctx, redir.ID); rd.Status != StatusError || !strings.Contains(rd.StatusMessage, "nectar-gateway") {
		t.Fatalf("redirect domain status: %s %q", rd.Status, rd.StatusMessage)
	}
	if got, _ := st.GetDomain(ctx, d.ID); got.Status != StatusActive || !strings.Contains(got.StatusMessage, "certificate") {
		t.Fatalf("domain: %s %q", got.Status, got.StatusMessage)
	}

	// cert-manager issued the certificate → expiry shows up; nothing is re-applied
	st.DeleteDomain(ctx, redir.ID)
	k.Apply(ctx, kube.Object{"apiVersion": "v1", "kind": "Secret", "metadata": map[string]any{"name": ingressName(d.ID) + "-tls", "namespace": "prj-shop"},
		"data": map[string]any{"tls.crt": base64.StdEncoding.EncodeToString(testCert(t))}})
	n := len(k.Applied)
	if err := s.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	if len(k.Applied) != n {
		t.Fatalf("unchanged ingress re-applied: %v", k.Applied[n:])
	}
	if got, _ := st.GetDomain(ctx, d.ID); got.CertNotAfter == nil || got.StatusMessage != "" {
		t.Fatalf("cert expiry: %+v", got)
	}

	// port change → updated; domain deleted → ingress removed
	st.UpdateDomainSettings(ctx, d.ID, 9090, true, true)
	s.Sync(ctx)
	if port := kube.Int(kube.Slice(kube.Slice(k.Get("Ingress", "prj-shop", ingressName(d.ID)), "spec", "rules")[0], "http", "paths")[0], "backend", "service", "port", "number"); port != 9090 {
		t.Fatalf("port not updated: %d", port)
	}
	st.DeleteDomain(ctx, d.ID)
	s.Sync(ctx)
	if k.Has("Ingress", "prj-shop", ingressName(d.ID)) {
		t.Fatal("ingress not deleted")
	}
	if _, err := prov.IssueCert(ctx, 1); err == nil {
		t.Fatal("IssueCert should explain cert-manager")
	}
}

func testCert(t *testing.T) []byte {
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	tmpl := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "shop.example.com"},
		NotBefore: time.Now(), NotAfter: time.Now().Add(90 * 24 * time.Hour)}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
}
