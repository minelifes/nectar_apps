// Package webauthntest is a software passkey authenticator for tests: it answers creation and request
// options like a browser plus platform authenticator would (fmt "none", UV set).
package webauthntest

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"sort"

	"nectar_apps/internal/webauthn"
)

// Authenticator holds one passkey per relying party.
type Authenticator struct {
	Origin     string
	Key        *ecdsa.PrivateKey
	CredID     []byte
	UserHandle []byte
	Count      uint32
	NoUV       bool // pretend the user was not verified
}

// New creates an authenticator for an origin.
func New(origin string) *Authenticator {
	k, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	id := make([]byte, 16)
	_, _ = rand.Read(id)
	return &Authenticator{Origin: origin, Key: k, CredID: id, Count: 1}
}

// --- CBOR encoding (just enough) ---

func head(major byte, n uint64) []byte {
	switch {
	case n < 24:
		return []byte{major<<5 | byte(n)}
	case n < 256:
		return []byte{major<<5 | 24, byte(n)}
	case n < 65536:
		return []byte{major<<5 | 25, byte(n >> 8), byte(n)}
	}
	b := []byte{major<<5 | 26, 0, 0, 0, 0}
	binary.BigEndian.PutUint32(b[1:], uint32(n))
	return b
}

// Enc encodes int, int64, string, []byte and map[any]any (keys sorted for stable output).
func Enc(v any) []byte {
	switch x := v.(type) {
	case int:
		return Enc(int64(x))
	case int64:
		if x >= 0 {
			return head(0, uint64(x))
		}
		return head(1, uint64(-1-x))
	case string:
		return append(head(3, uint64(len(x))), x...)
	case []byte:
		return append(head(2, uint64(len(x))), x...)
	case map[any]any:
		keys := make([]any, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Slice(keys, func(i, j int) bool { return string(Enc(keys[i])) < string(Enc(keys[j])) })
		out := head(5, uint64(len(x)))
		for _, k := range keys {
			out = append(out, Enc(k)...)
			out = append(out, Enc(x[k])...)
		}
		return out
	}
	panic("webauthntest: cannot encode")
}

func (a *Authenticator) cose() []byte {
	pad := func(b []byte) []byte { return append(make([]byte, 32-len(b)), b...) }
	return Enc(map[any]any{int64(1): int64(2), int64(3): int64(-7), int64(-1): int64(1),
		int64(-2): pad(a.Key.X.Bytes()), int64(-3): pad(a.Key.Y.Bytes())})
}

func (a *Authenticator) authData(rpID string, attested bool) []byte {
	h := sha256.Sum256([]byte(rpID))
	flags := byte(0x01 | 0x04 | 0x08 | 0x10) // UP UV BE BS
	if a.NoUV {
		flags &^= 0x04
	}
	if attested {
		flags |= 0x40
	}
	out := append([]byte{}, h[:]...)
	out = append(out, flags, 0, 0, 0, 0)
	binary.BigEndian.PutUint32(out[33:], a.Count)
	if attested {
		out = append(out, make([]byte, 16)...) // AAGUID
		out = append(out, byte(len(a.CredID)>>8), byte(len(a.CredID)))
		out = append(out, a.CredID...)
		out = append(out, a.cose()...)
	}
	return out
}

func clientData(typ, challenge, origin string) []byte {
	b, _ := json.Marshal(map[string]any{"type": typ, "challenge": challenge, "origin": origin, "crossOrigin": false})
	return b
}

// Create answers creation options (as decoded JSON) with PublicKeyCredential.toJSON().
func (a *Authenticator) Create(opts map[string]any) map[string]any {
	rp := opts["rp"].(map[string]any)
	user := opts["user"].(map[string]any)
	a.UserHandle, _ = webauthn.Unb64(user["id"].(string))
	att := Enc(map[any]any{"fmt": "none", "attStmt": map[any]any{}, "authData": a.authData(rp["id"].(string), true)})
	return map[string]any{"id": webauthn.B64(a.CredID), "rawId": webauthn.B64(a.CredID), "type": "public-key",
		"response": map[string]any{"clientDataJSON": webauthn.B64(clientData("webauthn.create", opts["challenge"].(string), a.Origin)),
			"attestationObject": webauthn.B64(att), "transports": []string{"internal", "hybrid"}}}
}

// Get answers request options with PublicKeyCredential.toJSON().
func (a *Authenticator) Get(opts map[string]any) map[string]any {
	a.Count++
	ad := a.authData(opts["rpId"].(string), false)
	cd := clientData("webauthn.get", opts["challenge"].(string), a.Origin)
	h := sha256.Sum256(cd)
	digest := sha256.Sum256(append(append([]byte{}, ad...), h[:]...))
	sig, _ := ecdsa.SignASN1(rand.Reader, a.Key, digest[:])
	return map[string]any{"id": webauthn.B64(a.CredID), "rawId": webauthn.B64(a.CredID), "type": "public-key",
		"response": map[string]any{"clientDataJSON": webauthn.B64(cd), "authenticatorData": webauthn.B64(ad),
			"signature": webauthn.B64(sig), "userHandle": webauthn.B64(a.UserHandle)}}
}
