package secretbox

import (
	"bytes"
	"testing"
)

func TestSealOpen(t *testing.T) {
	key := bytes.Repeat([]byte{7}, 32)
	b, err := New(key, "env")
	if err != nil {
		t.Fatal(err)
	}
	s1, _ := b.Seal("hunter2")
	s2, _ := b.Seal("hunter2")
	if s1 == s2 {
		t.Fatal("nonce reuse: two seals of the same value are identical")
	}
	got, err := b.Open(s1)
	if err != nil || got != "hunter2" {
		t.Fatalf("Open = %q, %v", got, err)
	}

	other, _ := New(key, "registry")
	if _, err := other.Open(s1); err != ErrDecrypt {
		t.Fatalf("different purpose must not decrypt, got %v", err)
	}
	wrong, _ := New(bytes.Repeat([]byte{8}, 32), "env")
	if _, err := wrong.Open(s1); err != ErrDecrypt {
		t.Fatalf("wrong key must not decrypt, got %v", err)
	}
	if _, err := b.Open("garbage"); err != ErrDecrypt {
		t.Fatalf("garbage: %v", err)
	}
	if _, err := New([]byte("short"), "env"); err == nil {
		t.Fatal("short key accepted")
	}
}
