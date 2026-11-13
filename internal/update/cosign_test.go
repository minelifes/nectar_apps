package update

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fakeRegistry serves cosign signatures the way ghcr.io does: anonymous pulls need a token from /token.
type fakeRegistry struct {
	srv       *httptest.Server
	host      string
	manifests map[string][]byte // "repo:tag"
	blobs     map[string][]byte // digest
	tokens    int
}

func newFakeRegistry(t *testing.T) *fakeRegistry {
	r := &fakeRegistry{manifests: map[string][]byte{}, blobs: map[string][]byte{}}
	r.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, q *http.Request) {
		if q.URL.Path == "/token" {
			r.tokens++
			if q.URL.Query().Get("scope") != "repository:acme/panel:pull" || q.URL.Query().Get("service") != "fake" {
				http.Error(w, "bad scope", 400)
				return
			}
			fmt.Fprint(w, `{"token":"t0k"}`)
			return
		}
		if q.Header.Get("Authorization") != "Bearer t0k" {
			w.Header().Set("WWW-Authenticate", `Bearer realm="`+r.srv.URL+`/token",service="fake",scope="repository:acme/panel:pull"`)
			w.WriteHeader(401)
			return
		}
		p := strings.TrimPrefix(q.URL.Path, "/v2/acme/panel/")
		if tag, ok := strings.CutPrefix(p, "manifests/"); ok {
			if m, ok := r.manifests[tag]; ok {
				w.Write(m)
				return
			}
		}
		if d, ok := strings.CutPrefix(p, "blobs/"); ok {
			if b, ok := r.blobs[d]; ok {
				w.Write(b)
				return
			}
		}
		http.NotFound(w, q)
	}))
	t.Cleanup(r.srv.Close)
	r.host = strings.TrimPrefix(r.srv.URL, "http://")
	return r
}

// sign stores a cosign signature of image digest made with key (payloadDigest is what the payload names).
func (r *fakeRegistry) sign(t *testing.T, key *ecdsa.PrivateKey, digest, payloadDigest string) {
	payload := []byte(`{"critical":{"identity":{"docker-reference":"x/acme/panel"},"image":{"docker-manifest-digest":"sha256:` +
		payloadDigest + `"},"type":"cosign container image signature"},"optional":null}`)
	sum := sha256.Sum256(payload)
	sig, err := ecdsa.SignASN1(rand.Reader, key, sum[:])
	if err != nil {
		t.Fatal(err)
	}
	bd := "sha256:" + hex.EncodeToString(sum[:])
	r.blobs[bd] = payload
	tag := "sha256-" + digest + ".sig"
	var m struct {
		Layers []map[string]any `json:"layers"`
	}
	if old, ok := r.manifests[tag]; ok {
		json.Unmarshal(old, &m)
	}
	m.Layers = append(m.Layers, map[string]any{"mediaType": "application/vnd.dev.cosign.simplesigning.v1+json", "digest": bd,
		"size": len(payload), "annotations": map[string]string{cosignSigAnnotation: base64.StdEncoding.EncodeToString(sig)}})
	r.manifests[tag], _ = json.Marshal(m)
}

func testKey(t *testing.T) (*ecdsa.PrivateKey, string) {
	k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, _ := x509.MarshalPKIXPublicKey(&k.PublicKey)
	return k, string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}))
}

func TestCosignVerify(t *testing.T) {
	reg := newFakeRegistry(t)
	ours, oursPEM := testKey(t)
	other, otherPEM := testKey(t)
	keys, err := ParsePublicKeys("# release keys\n" + oursPEM + "\n")
	if err != nil || len(keys) != 1 {
		t.Fatalf("keys: %v %v", keys, err)
	}
	v := &Verifier{Keys: keys, Scheme: "http"}
	ctx := context.Background()
	d := func(c string) string { return strings.Repeat(c, 64) }
	img := func(c string) string { return reg.host + "/acme/panel@sha256:" + d(c) }

	reg.sign(t, ours, d("a"), d("a"))
	if err := v.Verify(ctx, img("a")); err != nil {
		t.Fatalf("signed image: %v", err)
	}
	if reg.tokens != 1 {
		t.Fatalf("token requests: %d", reg.tokens)
	}

	for name, c := range map[string]struct{ image, want string }{
		"unsigned":        {img("b"), "is not signed"},
		"other key":       {img("c"), "not with a trusted key"},
		"other image":     {img("d"), "for another image"},
		"tag, not digest": {reg.host + "/acme/panel:1.2.3", "not pinned by digest"},
	} {
		reg.sign(t, other, d("c"), d("c"))
		reg.sign(t, ours, d("d"), d("a")) // a valid signature of image a, stored for image d
		err := v.Verify(ctx, c.image)
		if !errors.Is(err, ErrSignature) || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: %v", name, err)
		}
	}

	// several signatures: one by a trusted key is enough; a rotated key list accepts both
	reg.sign(t, ours, d("c"), d("c"))
	if err := v.Verify(ctx, img("c")); err != nil {
		t.Fatalf("two signatures: %v", err)
	}
	both, _ := ParsePublicKeys(otherPEM + oursPEM)
	if err := (&Verifier{Keys: both, Scheme: "http"}).Verify(ctx, img("a")); err != nil || len(both) != 2 {
		t.Fatalf("two keys: %v", err)
	}

	// a payload changed after signing
	for dg := range reg.blobs {
		reg.blobs[dg] = append(reg.blobs[dg], ' ')
	}
	if err := v.Verify(ctx, img("a")); !errors.Is(err, ErrSignature) {
		t.Fatalf("tampered payload: %v", err)
	}

	// an unreachable registry is an error, but not a verdict about the signature
	if err := v.Verify(ctx, "127.0.0.1:1/acme/panel@sha256:"+d("a")); err == nil || errors.Is(err, ErrSignature) {
		t.Fatalf("unreachable: %v", err)
	}
	// no keys: nothing is verified
	if err := (&Verifier{}).Verify(ctx, "anything"); err != nil || (*Verifier)(nil).Enabled() {
		t.Fatal("verifier without keys")
	}
	if _, err := ParsePublicKeys("-----BEGIN ENCRYPTED SIGSTORE PRIVATE KEY-----\nAAAA\n-----END ENCRYPTED SIGSTORE PRIVATE KEY-----\n"); err == nil {
		t.Fatal("a private key accepted as a public key")
	}
	if keys, err := ParsePublicKeys(ReleaseKeys); err != nil {
		t.Fatalf("compiled-in keys: %v (%d)", err, len(keys))
	}
}

// An update whose image is not signed with a trusted key is not started.
func TestStartRefusesUnsignedUpdate(t *testing.T) {
	reg := newFakeRegistry(t)
	ours, oursPEM := testKey(t)
	good := reg.host + "/acme/panel@sha256:" + strings.Repeat("a", 64)
	bad := reg.host + "/acme/panel@sha256:" + strings.Repeat("b", 64)
	reg.sign(t, ours, strings.Repeat("a", 64), strings.Repeat("a", 64))
	w := newWorld(t)
	feed := feedServer(t,
		[]Release{{Version: "0.6.2", Image: bad}, {Version: "0.6.1", Image: good}, {Version: "0.6.0", Image: "ghcr/p:0.6.0"}},
		[]Release{{Version: "1.2.1", Image: "ghcr/g:1.2.1", APIVersion: intp(1)}})
	m, _ := newManager(t, w, feed)
	keys, _ := ParsePublicKeys(oursPEM)
	m.Verifier = &Verifier{Keys: keys, Scheme: "http"}
	ctx := context.Background()
	if _, err := m.Check(ctx); err != nil {
		t.Fatal(err)
	}
	if !m.State(ctx).Signatures {
		t.Fatal("state does not say signatures are verified")
	}
	plan, err := m.Plan(ctx, PlanRequest{Panel: "0.6.2"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.Start(ctx, plan, "manual", nil, nil); !errors.Is(err, ErrSignature) || !strings.Contains(err.Error(), "panel 0.6.2") {
		t.Fatalf("unsigned update started: %v", err)
	}
	if runs, _ := m.Store.ActiveUpdateRuns(ctx); len(runs) != 0 {
		t.Fatalf("a run was created: %v", runs)
	}
	plan, _ = m.Plan(ctx, PlanRequest{Panel: "0.6.1"}, false)
	run, err := m.Start(ctx, plan, "manual", nil, nil)
	if err != nil {
		t.Fatalf("signed update: %v", err)
	}
	// let the run finish (in-process) so the manager stops tracking it
	r := w.runner(plan.Steps...)
	r.Spec.ID = run.ID
	r.Run(ctx)
	m.Wait()
}
