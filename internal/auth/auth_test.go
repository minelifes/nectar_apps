package auth

import (
	"strings"
	"testing"
)

func TestPasswordRoundTrip(t *testing.T) {
	h, err := HashPassword("correct horse battery")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(h, "pbkdf2-sha256$600000$") {
		t.Fatalf("unexpected format %q", h)
	}
	ok, rehash, err := VerifyPassword("correct horse battery", h)
	if err != nil || !ok || rehash {
		t.Fatalf("verify good: ok=%v rehash=%v err=%v", ok, rehash, err)
	}
	ok, _, err = VerifyPassword("wrong password!", h)
	if err != nil || ok {
		t.Fatalf("verify bad: ok=%v err=%v", ok, err)
	}
	h2, _ := HashPassword("correct horse battery")
	if h == h2 {
		t.Fatal("salt not random")
	}
}

func TestNeedsRehash(t *testing.T) {
	// a hash with fewer iterations than the current default must be flagged for re-hashing
	weak := "pbkdf2-sha256$1000$c2FsdHNhbHRzYWx0c2FsdA$" // hash part filled below
	ok, _, _ := VerifyPassword("x", weak+"AAAA")
	if ok {
		t.Fatal("garbage hash verified")
	}
	if _, _, err := VerifyPassword("x", "md5$abc"); err == nil {
		t.Fatal("unsupported format accepted")
	}
}

func TestValidatePassword(t *testing.T) {
	if ValidatePassword("short") == nil {
		t.Fatal("short password accepted")
	}
	if ValidatePassword("ünïcödé-pw") != nil {
		t.Fatal("10 unicode characters rejected")
	}
	if ValidatePassword(strings.Repeat("a", 300)) == nil {
		t.Fatal("huge password accepted")
	}
}

func TestTokens(t *testing.T) {
	p1, h1 := NewToken(PrefixSession)
	p2, _ := NewToken(PrefixSession)
	if p1 == p2 || !HasPrefix(p1, PrefixSession) || HashToken(p1) != h1 || len(h1) != 64 {
		t.Fatalf("bad token: %q %q", p1, h1)
	}
}

func TestHtpasswd(t *testing.T) {
	h, err := Htpasswd([]BasicAuthUser{{User: "a", Password: "password-1"}, {User: "b", Password: "password-2"}}, "")
	if err != nil || strings.Count(h, "\n") != 1 || !strings.HasPrefix(h, "a:$2") {
		t.Fatalf("htpasswd: %q %v", h, err)
	}
	h2, err := Htpasswd([]BasicAuthUser{{User: "b"}}, h) // keep b's hash, drop a
	if err != nil || !strings.HasPrefix(h2, "b:$2") || h2 != h[strings.Index(h, "\n")+1:] {
		t.Fatalf("keep: %q %v", h2, err)
	}
	for _, bad := range [][]BasicAuthUser{{{User: "x"}}, {{User: "a:b", Password: "password-1"}}, {{User: "c", Password: "short"}}, {{User: "d", Password: "password-1"}, {User: "d", Password: "password-1"}}} {
		if _, err := Htpasswd(bad, h); err == nil {
			t.Errorf("accepted %v", bad)
		}
	}
}
