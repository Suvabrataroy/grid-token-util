package security

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
)

// Sign computes an HMAC-SHA256 of payload using secret and returns the result
// as a lowercase hex string.
func Sign(secret string, payload []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
}

// Verify returns true if sig is a valid HMAC-SHA256 signature for payload
// under secret.  The comparison is performed in constant time to prevent
// timing side-channels.
func Verify(secret string, payload []byte, sig string) bool {
	expected := Sign(secret, payload)
	sigBytes, err := hex.DecodeString(sig)
	if err != nil {
		return false
	}
	expectedBytes, err := hex.DecodeString(expected)
	if err != nil {
		return false
	}
	return hmac.Equal(sigBytes, expectedBytes)
}
