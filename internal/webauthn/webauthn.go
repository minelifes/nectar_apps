// Package webauthn implements the parts of WebAuthn Level 2 the panel needs for passkeys: discoverable
// credentials, "none" attestation (the authenticator's make is not checked), user verification required,
// and the ES256, RS256, PS256 and EdDSA signature algorithms. Options and responses use the JSON forms of
// PublicKeyCredential.parseCreationOptionsFromJSON / toJSON (base64url without padding).
package webauthn

import (
	"bytes"
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"strings"
)

// RP is the relying party: the panel at one host.
type RP struct {
	ID     string // the host name, e.g. panel.apps.example.com
	Name   string // shown by the authenticator
	Origin string // https://panel.apps.example.com (http://localhost:8080 in development)
}

// User is the account a passkey is created for.
type User struct {
	ID          []byte // user handle (opaque, ≤ 64 bytes)
	Name        string // e.g. the email
	DisplayName string
}

// Credential is a registered passkey.
type Credential struct {
	ID             []byte
	PublicKey      []byte // COSE_Key
	SignCount      uint32
	AAGUID         []byte
	BackupEligible bool // a synced passkey (iCloud Keychain, Google Password Manager, 1Password …)
	BackedUp       bool
	Transports     []string
}

// B64 encodes base64url without padding.
func B64(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

// Unb64 decodes base64url (with or without padding).
func Unb64(s string) ([]byte, error) {
	return base64.RawURLEncoding.DecodeString(strings.TrimRight(s, "="))
}

// NewChallenge returns 32 random bytes, base64url.
func NewChallenge() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return B64(b)
}

var algorithms = []any{
	map[string]any{"type": "public-key", "alg": -7},   // ES256
	map[string]any{"type": "public-key", "alg": -8},   // EdDSA
	map[string]any{"type": "public-key", "alg": -257}, // RS256
	map[string]any{"type": "public-key", "alg": -37},  // PS256
}

// CreationOptions are the options for navigator.credentials.create (JSON form). exclude lists the user's
// existing credentials, so an authenticator is not registered twice.
func CreationOptions(rp RP, u User, challenge string, exclude [][]byte) map[string]any {
	ex := []any{}
	for _, id := range exclude {
		ex = append(ex, map[string]any{"type": "public-key", "id": B64(id)})
	}
	return map[string]any{
		"rp":                 map[string]any{"id": rp.ID, "name": rp.Name},
		"user":               map[string]any{"id": B64(u.ID), "name": u.Name, "displayName": u.DisplayName},
		"challenge":          challenge,
		"pubKeyCredParams":   algorithms,
		"timeout":            300000,
		"excludeCredentials": ex,
		"authenticatorSelection": map[string]any{
			"residentKey": "required", "requireResidentKey": true, "userVerification": "required",
		},
		"attestation": "none",
	}
}

// RequestOptions are the options for navigator.credentials.get: any passkey of this host (the user picks).
func RequestOptions(rp RP, challenge string) map[string]any {
	return map[string]any{"challenge": challenge, "rpId": rp.ID, "timeout": 300000, "userVerification": "required", "allowCredentials": []any{}}
}

// RegistrationResponse is PublicKeyCredential.toJSON() after create().
type RegistrationResponse struct {
	ID       string `json:"id"`
	RawID    string `json:"rawId"`
	Type     string `json:"type"`
	Response struct {
		ClientDataJSON    string   `json:"clientDataJSON"`
		AttestationObject string   `json:"attestationObject"`
		Transports        []string `json:"transports"`
	} `json:"response"`
}

// AssertionResponse is PublicKeyCredential.toJSON() after get().
type AssertionResponse struct {
	ID       string `json:"id"`
	RawID    string `json:"rawId"`
	Type     string `json:"type"`
	Response struct {
		ClientDataJSON    string `json:"clientDataJSON"`
		AuthenticatorData string `json:"authenticatorData"`
		Signature         string `json:"signature"`
		UserHandle        string `json:"userHandle"`
	} `json:"response"`
}

// Errors.
var (
	ErrVerify = errors.New("the passkey could not be verified")
	ErrClone  = errors.New("the passkey's signature counter went backwards (a cloned authenticator?)")
)

func verifyClientData(raw []byte, typ, challenge string, rp RP) error {
	var cd struct {
		Type        string `json:"type"`
		Challenge   string `json:"challenge"`
		Origin      string `json:"origin"`
		CrossOrigin bool   `json:"crossOrigin"`
	}
	if err := json.Unmarshal(raw, &cd); err != nil {
		return fmt.Errorf("%w: client data", ErrVerify)
	}
	want, _ := Unb64(challenge)
	got, err := Unb64(cd.Challenge)
	switch {
	case cd.Type != typ:
		return fmt.Errorf("%w: wrong ceremony %q", ErrVerify, cd.Type)
	case err != nil || len(want) == 0 || !bytes.Equal(want, got):
		return fmt.Errorf("%w: the challenge does not match (expired? try again)", ErrVerify)
	case cd.Origin != rp.Origin:
		return fmt.Errorf("%w: origin %s is not %s", ErrVerify, cd.Origin, rp.Origin)
	case cd.CrossOrigin:
		return fmt.Errorf("%w: cross-origin request", ErrVerify)
	}
	return nil
}

type authData struct {
	rpIDHash  []byte
	flags     byte
	signCount uint32
	aaguid    []byte
	credID    []byte
	pubKey    []byte
}

const (
	flagUP = 0x01
	flagUV = 0x04
	flagBE = 0x08
	flagBS = 0x10
	flagAT = 0x40
)

func parseAuthData(b []byte) (authData, error) {
	var a authData
	if len(b) < 37 {
		return a, fmt.Errorf("%w: authenticator data too short", ErrVerify)
	}
	a.rpIDHash, a.flags, a.signCount = b[:32], b[32], binary.BigEndian.Uint32(b[33:37])
	if a.flags&flagAT != 0 {
		rest := b[37:]
		if len(rest) < 18 {
			return a, fmt.Errorf("%w: attested credential data", ErrVerify)
		}
		a.aaguid = rest[:16]
		n := int(binary.BigEndian.Uint16(rest[16:18]))
		if n == 0 || n > 1023 || len(rest) < 18+n {
			return a, fmt.Errorf("%w: credential id", ErrVerify)
		}
		a.credID = rest[18 : 18+n]
		_, used, err := decodeCBOR(rest[18+n:])
		if err != nil {
			return a, fmt.Errorf("%w: credential public key", ErrVerify)
		}
		a.pubKey = rest[18+n : 18+n+used]
	}
	return a, nil
}

func (a authData) check(rp RP) error {
	h := sha256.Sum256([]byte(rp.ID))
	switch {
	case !bytes.Equal(a.rpIDHash, h[:]):
		return fmt.Errorf("%w: the passkey belongs to another site", ErrVerify)
	case a.flags&flagUP == 0:
		return fmt.Errorf("%w: user presence missing", ErrVerify)
	case a.flags&flagUV == 0:
		return fmt.Errorf("%w: user verification missing (PIN, fingerprint or face)", ErrVerify)
	}
	return nil
}

// FinishRegistration verifies the response to CreationOptions and returns the new credential.
func FinishRegistration(rp RP, challenge string, resp RegistrationResponse) (Credential, error) {
	var c Credential
	if resp.Type != "public-key" {
		return c, fmt.Errorf("%w: not a public key credential", ErrVerify)
	}
	cdj, err1 := Unb64(resp.Response.ClientDataJSON)
	att, err2 := Unb64(resp.Response.AttestationObject)
	if err1 != nil || err2 != nil {
		return c, fmt.Errorf("%w: encoding", ErrVerify)
	}
	if err := verifyClientData(cdj, "webauthn.create", challenge, rp); err != nil {
		return c, err
	}
	obj, _, err := decodeCBOR(att)
	m, ok := obj.(map[any]any)
	if err != nil || !ok {
		return c, fmt.Errorf("%w: attestation object", ErrVerify)
	}
	raw, ok := m["authData"].([]byte)
	if !ok {
		return c, fmt.Errorf("%w: attestation object without authData", ErrVerify)
	}
	ad, err := parseAuthData(raw)
	if err != nil {
		return c, err
	}
	if err := ad.check(rp); err != nil {
		return c, err
	}
	if ad.credID == nil {
		return c, fmt.Errorf("%w: no credential in the response", ErrVerify)
	}
	if rawID, err := Unb64(resp.RawID); err != nil || !bytes.Equal(rawID, ad.credID) {
		return c, fmt.Errorf("%w: credential id mismatch", ErrVerify)
	}
	if _, _, err := parsePublicKey(ad.pubKey); err != nil {
		return c, err
	}
	return Credential{ID: ad.credID, PublicKey: ad.pubKey, SignCount: ad.signCount, AAGUID: ad.aaguid,
		BackupEligible: ad.flags&flagBE != 0, BackedUp: ad.flags&flagBS != 0, Transports: resp.Response.Transports}, nil
}

// FinishLogin verifies the response to RequestOptions against the stored credential (looked up by the
// caller from CredentialID and UserHandle) and returns the new signature counter.
func FinishLogin(rp RP, challenge string, resp AssertionResponse, cred Credential) (uint32, error) {
	if resp.Type != "public-key" {
		return 0, fmt.Errorf("%w: not a public key credential", ErrVerify)
	}
	cdj, err1 := Unb64(resp.Response.ClientDataJSON)
	adRaw, err2 := Unb64(resp.Response.AuthenticatorData)
	sig, err3 := Unb64(resp.Response.Signature)
	if err1 != nil || err2 != nil || err3 != nil {
		return 0, fmt.Errorf("%w: encoding", ErrVerify)
	}
	if err := verifyClientData(cdj, "webauthn.get", challenge, rp); err != nil {
		return 0, err
	}
	ad, err := parseAuthData(adRaw)
	if err != nil {
		return 0, err
	}
	if err := ad.check(rp); err != nil {
		return 0, err
	}
	pub, alg, err := parsePublicKey(cred.PublicKey)
	if err != nil {
		return 0, err
	}
	h := sha256.Sum256(cdj)
	if !verifySignature(pub, alg, append(append([]byte{}, adRaw...), h[:]...), sig) {
		return 0, fmt.Errorf("%w: bad signature", ErrVerify)
	}
	if (ad.signCount != 0 || cred.SignCount != 0) && ad.signCount <= cred.SignCount {
		return 0, ErrClone
	}
	return ad.signCount, nil
}

// CredentialID decodes the credential id and user handle of an assertion (for the lookup).
func (a AssertionResponse) CredentialID() (id, userHandle []byte, err error) {
	if id, err = Unb64(a.RawID); err != nil || len(id) == 0 {
		return nil, nil, fmt.Errorf("%w: credential id", ErrVerify)
	}
	if userHandle, err = Unb64(a.Response.UserHandle); err != nil {
		return nil, nil, fmt.Errorf("%w: user handle", ErrVerify)
	}
	return id, userHandle, nil
}

func intKey(m map[any]any, k int64) (int64, bool) {
	v, ok := m[k].(int64)
	return v, ok
}

func bytesKey(m map[any]any, k int64) []byte {
	v, _ := m[k].([]byte)
	return v
}

// parsePublicKey reads a COSE_Key: EC2 P-256 (ES256), OKP Ed25519 (EdDSA), RSA (RS256, PS256).
func parsePublicKey(cose []byte) (crypto.PublicKey, int64, error) {
	v, _, err := decodeCBOR(cose)
	m, ok := v.(map[any]any)
	if err != nil || !ok {
		return nil, 0, fmt.Errorf("%w: public key", ErrVerify)
	}
	kty, _ := intKey(m, 1)
	alg, _ := intKey(m, 3)
	switch {
	case kty == 2 && alg == -7:
		crv, _ := intKey(m, -1)
		x, y := bytesKey(m, -2), bytesKey(m, -3)
		if crv != 1 || len(x) != 32 || len(y) != 32 {
			return nil, 0, fmt.Errorf("%w: unsupported EC key", ErrVerify)
		}
		pub := &ecdsa.PublicKey{Curve: elliptic.P256(), X: new(big.Int).SetBytes(x), Y: new(big.Int).SetBytes(y)}
		if !pub.Curve.IsOnCurve(pub.X, pub.Y) { //nolint:staticcheck // validating untrusted input
			return nil, 0, fmt.Errorf("%w: invalid EC point", ErrVerify)
		}
		return pub, alg, nil
	case kty == 1 && alg == -8:
		crv, _ := intKey(m, -1)
		x := bytesKey(m, -2)
		if crv != 6 || len(x) != ed25519.PublicKeySize {
			return nil, 0, fmt.Errorf("%w: unsupported OKP key", ErrVerify)
		}
		return ed25519.PublicKey(x), alg, nil
	case kty == 3 && (alg == -257 || alg == -37):
		n, e := bytesKey(m, -1), bytesKey(m, -2)
		if len(n) < 256 || len(e) == 0 || len(e) > 4 {
			return nil, 0, fmt.Errorf("%w: unsupported RSA key", ErrVerify)
		}
		ev := 0
		for _, b := range e {
			ev = ev<<8 | int(b)
		}
		return &rsa.PublicKey{N: new(big.Int).SetBytes(n), E: ev}, alg, nil
	}
	return nil, 0, fmt.Errorf("%w: unsupported key type %d / algorithm %d", ErrVerify, kty, alg)
}

func verifySignature(pub crypto.PublicKey, alg int64, data, sig []byte) bool {
	switch k := pub.(type) {
	case *ecdsa.PublicKey:
		h := sha256.Sum256(data)
		return ecdsa.VerifyASN1(k, h[:], sig)
	case ed25519.PublicKey:
		return ed25519.Verify(k, data, sig)
	case *rsa.PublicKey:
		h := sha256.Sum256(data)
		if alg == -37 {
			return rsa.VerifyPSS(k, crypto.SHA256, h[:], sig, nil) == nil
		}
		return rsa.VerifyPKCS1v15(k, crypto.SHA256, h[:], sig) == nil
	}
	return false
}
