// Package secretkey contains non-authenticating operator metadata for the
// critical Owl Invites capability key.
package secretkey

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
)

const fingerprintDomain = "owl-invites-secret-fingerprint:v1"

// Fingerprint returns a domain-separated, non-secret restore-material marker.
// It is deliberately not accepted for authentication or capability signing.
func Fingerprint(secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(fingerprintDomain))
	return "oi-secret-v1:" + hex.EncodeToString(mac.Sum(nil)[:16])
}
