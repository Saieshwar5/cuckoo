// Package signing puts the hub's mark on a message.
//
// A message the hub has finished with is signed over its content with a key
// only the hub holds. The signature travels with the message to whoever
// keeps a copy — an agent's backend, one day a person's own export — and
// comes back with it. The hub can then tell that a copy is exactly what was
// said, in the order it was said, without trusting whoever kept it. A keeper
// can leave a message out; they cannot change one.
//
// Only the hub verifies, so a keyed hash is enough: there is no second party
// who needs to check without the key.
package signing

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
)

// KeyLen is how many bytes a key is. Shorter is weaker; longer buys nothing.
const KeyLen = 32

// Signer signs and verifies with one key.
type Signer struct{ key []byte }

// New builds a signer. The key must be KeyLen bytes.
func New(key []byte) (*Signer, error) {
	if len(key) != KeyLen {
		return nil, fmt.Errorf("signing key must be %d bytes, got %d", KeyLen, len(key))
	}
	k := make([]byte, KeyLen)
	copy(k, key)
	return &Signer{key: k}, nil
}

// ParseKey reads a key written as hex, which is how it lives in the
// environment: `openssl rand -hex 32` produces one.
func ParseKey(text string) ([]byte, error) {
	if text == "" {
		return nil, errors.New("signing key is empty")
	}
	key, err := hex.DecodeString(text)
	if err != nil {
		return nil, fmt.Errorf("signing key is not hex: %w", err)
	}
	if len(key) != KeyLen {
		return nil, fmt.Errorf("signing key must be %d hex characters, got %d", KeyLen*2, len(text))
	}
	return key, nil
}

// Sign returns the signature over payload.
func (s *Signer) Sign(payload []byte) []byte {
	mac := hmac.New(sha256.New, s.key)
	mac.Write(payload)
	return mac.Sum(nil)
}

// Verify reports whether sig is this signer's signature over payload. A
// missing signature is simply not a signature.
func (s *Signer) Verify(payload, sig []byte) bool {
	if len(sig) == 0 {
		return false
	}
	return hmac.Equal(sig, s.Sign(payload))
}
