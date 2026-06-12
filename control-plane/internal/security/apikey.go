// Package security provides cryptographic helpers for API key management,
// HMAC signing, and secret scanning.
package security

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"

	"golang.org/x/crypto/argon2"
)

const (
	// KeyPrefix is prepended to every generated API key for easy identification.
	KeyPrefix = "gw_"

	// KeyPrefixLen is the number of base64 characters used to form the public
	// prefix stored in the database for prefix-based lookup.
	KeyPrefixLen = 8

	// keyRandomBytes is the number of cryptographically random bytes in the key.
	keyRandomBytes = 32
)

// Argon2id parameters — these are intentionally tunable via SecurityConfig;
// the constants below are the defaults used when calling GenerateAPIKey
// directly without external configuration.
const (
	defaultArgonMemory      uint32 = 65536 // 64 MiB
	defaultArgonIterations  uint32 = 3
	defaultArgonParallelism uint8  = 2
	defaultArgonSaltLen     int    = 16
	defaultArgonKeyLen      uint32 = 32
)

// ArgonParams bundles the Argon2id tuning parameters.
type ArgonParams struct {
	Memory      uint32
	Iterations  uint32
	Parallelism uint8
	SaltLen     int
	KeyLen      uint32
}

// DefaultArgonParams returns the default Argon2id parameters.
func DefaultArgonParams() ArgonParams {
	return ArgonParams{
		Memory:      defaultArgonMemory,
		Iterations:  defaultArgonIterations,
		Parallelism: defaultArgonParallelism,
		SaltLen:     defaultArgonSaltLen,
		KeyLen:      defaultArgonKeyLen,
	}
}

// GenerateAPIKey creates a cryptographically random API key.
// It returns:
//   - plaintext: the full secret to be returned to the caller once
//   - prefix:    the public prefix stored in the DB for prefix-based lookup
//   - hash:      the Argon2id hash stored in the DB for verification
func GenerateAPIKey() (plaintext, prefix, hash string, err error) {
	return GenerateAPIKeyWithParams(DefaultArgonParams())
}

// GenerateAPIKeyWithParams is like GenerateAPIKey but uses caller-supplied
// Argon2id parameters.
func GenerateAPIKeyWithParams(p ArgonParams) (plaintext, prefix, hash string, err error) {
	raw := make([]byte, keyRandomBytes)
	if _, err = rand.Read(raw); err != nil {
		return "", "", "", fmt.Errorf("apikey: read random: %w", err)
	}

	b64 := base64.RawURLEncoding.EncodeToString(raw)
	plaintext = KeyPrefix + b64
	prefix = KeyPrefix + b64[:KeyPrefixLen]

	hash, err = hashArgon2id(plaintext, p)
	if err != nil {
		return "", "", "", fmt.Errorf("apikey: hash: %w", err)
	}
	return plaintext, prefix, hash, nil
}

// VerifyAPIKey checks whether the supplied plaintext matches the stored
// Argon2id hash.  It extracts the embedded salt from the hash string.
func VerifyAPIKey(plaintext, storedHash string) bool {
	// The stored hash format: hex(salt) + "$" + hex(derived_key)
	salt, dk, err := parseArgon2idHash(storedHash)
	if err != nil {
		return false
	}

	p := DefaultArgonParams()
	candidate := argon2.IDKey([]byte(plaintext), salt, p.Iterations, p.Memory, p.Parallelism, p.KeyLen)
	return hex.EncodeToString(candidate) == hex.EncodeToString(dk)
}

// hashArgon2id produces a portable hash string embedding the random salt.
func hashArgon2id(plaintext string, p ArgonParams) (string, error) {
	salt := make([]byte, p.SaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("argon2id: read salt: %w", err)
	}

	dk := argon2.IDKey([]byte(plaintext), salt, p.Iterations, p.Memory, p.Parallelism, p.KeyLen)
	return hex.EncodeToString(salt) + "$" + hex.EncodeToString(dk), nil
}

// parseArgon2idHash splits a hash string produced by hashArgon2id back into
// salt and derived-key byte slices.
func parseArgon2idHash(h string) (salt, dk []byte, err error) {
	// Find the separator between salt and derived key.
	for i := 0; i < len(h); i++ {
		if h[i] == '$' {
			salt, err = hex.DecodeString(h[:i])
			if err != nil {
				return nil, nil, fmt.Errorf("decode salt: %w", err)
			}
			dk, err = hex.DecodeString(h[i+1:])
			if err != nil {
				return nil, nil, fmt.Errorf("decode dk: %w", err)
			}
			return salt, dk, nil
		}
	}
	return nil, nil, fmt.Errorf("invalid hash format: missing separator")
}
