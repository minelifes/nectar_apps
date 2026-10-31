package webauthn_test

import (
	"encoding/json"
	"errors"
	"testing"

	"nectar_apps/internal/webauthn"
	"nectar_apps/internal/webauthn/webauthntest"
)

func roundTrip(v any, out any) {
	b, _ := json.Marshal(v)
	_ = json.Unmarshal(b, out)
}

func TestRegisterAndLogin(t *testing.T) {
	rp := webauthn.RP{ID: "panel.example.com", Name: "Nectar", Origin: "https://panel.example.com"}
	auth := webauthntest.New(rp.Origin)
	ch := webauthn.NewChallenge()
	var opts map[string]any
	roundTrip(webauthn.CreationOptions(rp, webauthn.User{ID: []byte{0, 0, 0, 0, 0, 0, 0, 7}, Name: "ann@example.com", DisplayName: "Ann"}, ch, nil), &opts)
	var reg webauthn.RegistrationResponse
	roundTrip(auth.Create(opts), &reg)
	cred, err := webauthn.FinishRegistration(rp, ch, reg)
	if err != nil || !cred.BackupEligible || len(cred.ID) != 16 {
		t.Fatalf("register: %v %+v", err, cred)
	}
	if _, err := webauthn.FinishRegistration(rp, webauthn.NewChallenge(), reg); !errors.Is(err, webauthn.ErrVerify) {
		t.Fatalf("other challenge accepted: %v", err)
	}
	other := rp
	other.Origin = "https://evil.example"
	if _, err := webauthn.FinishRegistration(other, ch, reg); err == nil {
		t.Fatal("other origin accepted")
	}

	ch = webauthn.NewChallenge()
	roundTrip(webauthn.RequestOptions(rp, ch), &opts)
	var as webauthn.AssertionResponse
	roundTrip(auth.Get(opts), &as)
	id, handle, err := as.CredentialID()
	if err != nil || string(id) != string(cred.ID) || handle[7] != 7 {
		t.Fatalf("ids: %v", err)
	}
	n, err := webauthn.FinishLogin(rp, ch, as, cred)
	if err != nil || n != 2 {
		t.Fatalf("login: %v %d", err, n)
	}
	cred.SignCount = n
	if _, err := webauthn.FinishLogin(rp, ch, as, cred); !errors.Is(err, webauthn.ErrClone) {
		t.Fatalf("replayed counter: %v", err)
	}
	// a tampered signature and another site's passkey fail
	roundTrip(auth.Get(opts), &as)
	as.Response.Signature = webauthn.B64([]byte("nope"))
	if _, err := webauthn.FinishLogin(rp, ch, as, cred); !errors.Is(err, webauthn.ErrVerify) {
		t.Fatalf("bad signature: %v", err)
	}
	roundTrip(auth.Get(map[string]any{"rpId": "evil.example", "challenge": ch}), &as)
	if _, err := webauthn.FinishLogin(rp, ch, as, cred); !errors.Is(err, webauthn.ErrVerify) {
		t.Fatalf("other rp: %v", err)
	}
	auth.NoUV = true
	roundTrip(auth.Get(opts), &as)
	if _, err := webauthn.FinishLogin(rp, ch, as, cred); !errors.Is(err, webauthn.ErrVerify) {
		t.Fatalf("no user verification: %v", err)
	}
}
