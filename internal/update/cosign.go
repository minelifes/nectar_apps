package update

import (
	"context"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/sha256"
	"crypto/x509"
	_ "embed"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// Signature verification of update images (cosign, key based).
//
// The release workflow signs the image digest with a private key (cosign sign --key). cosign stores the
// signature in the same repository under the tag sha256-<digest>.sig: a manifest whose layers are small
// JSON payloads naming the signed digest, each with the signature in an annotation. Before an update is
// applied the panel fetches that manifest, checks that a payload names exactly the image digest of the
// release and that its signature was made with one of the trusted public keys. Releases are pinned by
// digest, so a verified signature covers exactly the bytes the cluster will pull. Only the standard
// library is used; the transparency log is not consulted (a key signature needs none).

// ReleaseKeys are the public keys official releases are signed with (PEM, cosign.pub), compiled in.
// NECTAR_UPDATE_PUBLIC_KEYS replaces them ("off" = don't verify).
//
//go:embed release_keys.pem
var ReleaseKeys string

// ErrSignature: the image has no signature made with a trusted key.
var ErrSignature = errors.New("signature check failed")

const (
	cosignSigAnnotation = "dev.cosignproject.cosign/signature"
	cosignPayloadType   = "cosign container image signature"
)

// Verifier checks cosign signatures of images. The zero value (no keys) verifies nothing.
type Verifier struct {
	Keys   []any        // *ecdsa.PublicKey or ed25519.PublicKey
	HTTP   *http.Client // default: 30 s timeout
	Scheme string       // default https (tests: http)
}

// Enabled reports whether there are keys to verify with.
func (v *Verifier) Enabled() bool { return v != nil && len(v.Keys) > 0 }

// ParsePublicKeys reads the PEM public keys in s (text around them is ignored).
func ParsePublicKeys(s string) ([]any, error) {
	var keys []any
	rest := []byte(s)
	for {
		var b *pem.Block
		if b, rest = pem.Decode(rest); b == nil {
			return keys, nil
		}
		if b.Type != "PUBLIC KEY" {
			return nil, fmt.Errorf("a %s block is not a public key (use cosign.pub)", b.Type)
		}
		k, err := x509.ParsePKIXPublicKey(b.Bytes)
		if err != nil {
			return nil, fmt.Errorf("public key: %w", err)
		}
		switch k.(type) {
		case *ecdsa.PublicKey, ed25519.PublicKey:
			keys = append(keys, k)
		default:
			return nil, fmt.Errorf("public key: %T is not supported (ECDSA or Ed25519)", k)
		}
	}
}

var pinnedRe = regexp.MustCompile(`^([a-zA-Z0-9.-]+(?::[0-9]+)?)/([a-z0-9._/-]+)@sha256:([a-f0-9]{64})$`)

func (v *Verifier) client() *http.Client {
	if v.HTTP != nil {
		return v.HTTP
	}
	return &http.Client{Timeout: 30 * time.Second}
}

// Verify returns nil when image (registry/repository@sha256:…) carries a cosign signature made with one
// of the keys. Errors wrap ErrSignature unless the registry could not be reached.
func (v *Verifier) Verify(ctx context.Context, image string) error {
	if !v.Enabled() {
		return nil
	}
	m := pinnedRe.FindStringSubmatch(image)
	if m == nil {
		return fmt.Errorf("%w: %s is not pinned by digest (registry/name@sha256:…)", ErrSignature, image)
	}
	host, repo, digest := m[1], m[2], m[3]
	if host == "docker.io" || host == "index.docker.io" {
		host = "registry-1.docker.io"
	}
	scheme := v.Scheme
	if scheme == "" {
		scheme = "https"
	}
	rc := &registryClient{http: v.client(), base: scheme + "://" + host, repo: repo}
	body, code, err := rc.get(ctx, "/v2/"+repo+"/manifests/sha256-"+digest+".sig", 1<<20)
	if err != nil {
		return fmt.Errorf("signature of %s: %w", image, err)
	}
	if code == http.StatusNotFound {
		return fmt.Errorf("%w: %s is not signed", ErrSignature, image)
	}
	if code != http.StatusOK {
		return fmt.Errorf("signature of %s: the registry answered HTTP %d", image, code)
	}
	var manifest struct {
		Layers []struct {
			Digest      string            `json:"digest"`
			Annotations map[string]string `json:"annotations"`
		} `json:"layers"`
	}
	if err := json.Unmarshal(body, &manifest); err != nil {
		return fmt.Errorf("%w: unreadable signature manifest of %s", ErrSignature, image)
	}
	why := "it has no signatures"
	for i, l := range manifest.Layers {
		if i >= 20 {
			break
		}
		sig, err := base64.StdEncoding.DecodeString(l.Annotations[cosignSigAnnotation])
		if err != nil || len(sig) == 0 || !strings.HasPrefix(l.Digest, "sha256:") {
			continue
		}
		payload, code, err := rc.get(ctx, "/v2/"+repo+"/blobs/"+l.Digest, 64<<10)
		if err != nil {
			return fmt.Errorf("signature of %s: %w", image, err)
		}
		sum := sha256.Sum256(payload)
		if code != http.StatusOK || "sha256:"+hex.EncodeToString(sum[:]) != l.Digest {
			why = "a signature payload does not match its digest"
			continue
		}
		var p struct {
			Critical struct {
				Image struct {
					Digest string `json:"docker-manifest-digest"`
				} `json:"image"`
				Type string `json:"type"`
			} `json:"critical"`
		}
		if json.Unmarshal(payload, &p) != nil || p.Critical.Type != cosignPayloadType || p.Critical.Image.Digest != "sha256:"+digest {
			why = "its signature is for another image"
			continue
		}
		for _, k := range v.Keys {
			switch key := k.(type) {
			case *ecdsa.PublicKey:
				if ecdsa.VerifyASN1(key, sum[:], sig) {
					return nil
				}
			case ed25519.PublicKey:
				if ed25519.Verify(key, payload, sig) {
					return nil
				}
			}
		}
		why = "it is signed, but not with a trusted key"
	}
	return fmt.Errorf("%w: %s: %s", ErrSignature, image, why)
}

// registryClient reads from an OCI registry, with the anonymous token flow (ghcr.io, Docker Hub).
type registryClient struct {
	http  *http.Client
	base  string
	repo  string
	token string
}

func (c *registryClient) get(ctx context.Context, path string, limit int64) ([]byte, int, error) {
	for attempt := 0; ; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+path, nil)
		if err != nil {
			return nil, 0, err
		}
		req.Header.Set("Accept", "application/vnd.oci.image.manifest.v1+json, application/vnd.docker.distribution.manifest.v2+json, */*")
		req.Header.Set("User-Agent", "nectar-apps-updater")
		if c.token != "" {
			req.Header.Set("Authorization", "Bearer "+c.token)
		}
		resp, err := c.http.Do(req)
		if err != nil {
			return nil, 0, err
		}
		body, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
		resp.Body.Close()
		if err != nil {
			return nil, 0, err
		}
		if resp.StatusCode == http.StatusUnauthorized && attempt == 0 {
			if err := c.login(ctx, resp.Header.Get("WWW-Authenticate")); err != nil {
				return nil, 0, err
			}
			continue
		}
		if int64(len(body)) > limit {
			return nil, 0, errors.New("the registry's answer is too large")
		}
		return body, resp.StatusCode, nil
	}
}

var challengeRe = regexp.MustCompile(`(realm|service|scope)="([^"]*)"`)

// login gets an anonymous pull token as the registry's challenge describes.
func (c *registryClient) login(ctx context.Context, challenge string) error {
	if !strings.HasPrefix(strings.ToLower(challenge), "bearer ") {
		return errors.New("the registry wants a login (only public images can be verified)")
	}
	p := map[string]string{}
	for _, m := range challengeRe.FindAllStringSubmatch(challenge, -1) {
		p[m[1]] = m[2]
	}
	realm, err := url.Parse(p["realm"])
	if err != nil || realm.Host == "" || (realm.Scheme != "https" && !strings.HasPrefix(c.base, "http://")) {
		return errors.New("the registry's token service is not usable")
	}
	q := realm.Query()
	if p["service"] != "" {
		q.Set("service", p["service"])
	}
	q.Set("scope", "repository:"+c.repo+":pull")
	realm.RawQuery = q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, realm.String(), nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	var out struct {
		Token       string `json:"token"`
		AccessToken string `json:"access_token"`
	}
	if resp.StatusCode != http.StatusOK || json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&out) != nil {
		return fmt.Errorf("the registry's token service answered HTTP %d", resp.StatusCode)
	}
	if c.token = out.Token; c.token == "" {
		c.token = out.AccessToken
	}
	if c.token == "" {
		return errors.New("the registry gave no token")
	}
	return nil
}
