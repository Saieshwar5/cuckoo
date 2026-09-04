package domain

import (
	"crypto/rand"
	"crypto/sha256"
	"fmt"
)

// Secret prefixes. Like identifiers, a credential announces what it is, so a
// value found in a log or a paste is recognisable — and so a scanner can find
// it before it reaches a public repository.
const (
	PrefixBindingSecret = "bnd_sec"
)

// secretBytes is the entropy in every generated secret. 256 bits: not
// guessable at any hash cost, which is why a fast hash is sufficient.
const secretBytes = 32

// NewSecret generates a credential.
//
// It returns the plaintext, which is shown to the caller exactly once, and the
// hash, which is the only form ever stored. Nothing that has the hash can
// recover the plaintext; nothing that has the plaintext needs to store it.
func NewSecret(prefix string) (plaintext string, hash []byte) {
	raw := make([]byte, secretBytes)
	if _, err := rand.Read(raw); err != nil {
		// A failing system entropy source means nothing in the process can be
		// trusted, so there is no sensible way to continue.
		panic(fmt.Sprintf("domain: cannot generate secret: %v", err))
	}

	plaintext = prefix + "_" + idEncoding.EncodeToString(raw)
	return plaintext, HashSecret(plaintext)
}

// HashSecret returns the storable form of a secret.
//
// SHA-256 rather than a password hash such as argon2, on purpose: password
// hashes exist to make guessing expensive, and a 256-bit random secret cannot
// be guessed at any price. A slow hash here would only add tens of
// milliseconds to every request an agent backend makes.
func HashSecret(plaintext string) []byte {
	sum := sha256.Sum256([]byte(plaintext))
	return sum[:]
}
