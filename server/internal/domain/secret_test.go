package domain

import (
	"strings"
	"testing"
)

func TestNewSecretShape(t *testing.T) {
	plaintext, hash := NewSecret(PrefixBindingSecret)

	if !strings.HasPrefix(plaintext, PrefixBindingSecret+"_") {
		t.Errorf("secret %q does not carry its prefix", plaintext)
	}
	// 32 bytes in base32 with no padding is 52 characters.
	if body := strings.TrimPrefix(plaintext, PrefixBindingSecret+"_"); len(body) != 52 {
		t.Errorf("secret body is %d characters, want 52", len(body))
	}
	if len(hash) != 32 {
		t.Errorf("hash is %d bytes, want 32 (SHA-256)", len(hash))
	}
}

func TestNewSecretIsUnique(t *testing.T) {
	seen := make(map[string]struct{}, 1000)
	for range 1000 {
		s, _ := NewSecret(PrefixBindingSecret)
		if _, dup := seen[s]; dup {
			t.Fatalf("NewSecret produced a duplicate: %s", s)
		}
		seen[s] = struct{}{}
	}
}

// The hash must be a pure function of the plaintext, or a stored hash could
// never be matched against a later presentation.
func TestHashSecretIsDeterministic(t *testing.T) {
	plaintext, hash := NewSecret(PrefixBindingSecret)

	if string(HashSecret(plaintext)) != string(hash) {
		t.Error("HashSecret(plaintext) differs from the hash NewSecret returned")
	}
}

// The plaintext must not be derivable from the hash. This cannot be proven by
// a test, but a hash that happens to contain the plaintext would be caught.
func TestHashDoesNotContainPlaintext(t *testing.T) {
	plaintext, hash := NewSecret(PrefixBindingSecret)
	if strings.Contains(string(hash), plaintext[len(PrefixBindingSecret)+1:][:16]) {
		t.Error("hash contains part of the plaintext")
	}
}
