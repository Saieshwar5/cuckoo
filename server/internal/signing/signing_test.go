package signing_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/Saieshwar5/cuckoo/server/internal/signing"
)

func key(t *testing.T, seed byte) *signing.Signer {
	t.Helper()
	s, err := signing.New(bytes.Repeat([]byte{seed}, signing.KeyLen))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return s
}

func TestSignAndVerify(t *testing.T) {
	s := key(t, 1)
	payload := []byte(`{"id":"msg_1","text":"hello"}`)
	sig := s.Sign(payload)
	if !s.Verify(payload, sig) {
		t.Fatal("a signature does not verify against its own payload")
	}
	if s.Verify([]byte(`{"id":"msg_1","text":"hallo"}`), sig) {
		t.Error("a changed payload verifies")
	}
	if key(t, 2).Verify(payload, sig) {
		t.Error("another key verifies this key's signature")
	}
	if s.Verify(payload, nil) || s.Verify(payload, []byte{}) {
		t.Error("an absent signature verifies")
	}
}

func TestSignIsDeterministic(t *testing.T) {
	s := key(t, 7)
	if !bytes.Equal(s.Sign([]byte("x")), s.Sign([]byte("x"))) {
		t.Error("signing the same bytes twice differs")
	}
}

func TestParseKey(t *testing.T) {
	good := strings.Repeat("ab", signing.KeyLen)
	k, err := signing.ParseKey(good)
	if err != nil || len(k) != signing.KeyLen {
		t.Fatalf("ParseKey(good) = %v, %v", k, err)
	}
	for _, bad := range []string{"", "abc", strings.Repeat("zz", signing.KeyLen), strings.Repeat("ab", signing.KeyLen-1)} {
		if _, err := signing.ParseKey(bad); err == nil {
			t.Errorf("ParseKey(%q) accepted", bad)
		}
	}
	if _, err := signing.New([]byte("short")); err == nil {
		t.Error("New accepted a short key")
	}
}
