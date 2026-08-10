package invitation

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

const (
	privateCapabilityPrefix = "oi1"
	openCapabilityPrefix    = "oo1"
	privateCapabilityDomain = "owl-invites/private-invitation/v1"
	openCapabilityDomain    = "owl-invites/open-enrollment/v1"
)

var ErrInvalidCapability = errors.New("invalid capability")

// CapabilitySigner reconstructs capability proofs from non-secret selectors.
// Raw capabilities are never persisted.
type CapabilitySigner struct {
	key []byte
}

func NewCapabilitySigner(secret string) (*CapabilitySigner, error) {
	if len(secret) < 32 {
		return nil, fmt.Errorf("OWL_INVITES_SECRET_KEY must contain at least 32 bytes")
	}
	return &CapabilitySigner{key: []byte(secret)}, nil
}

func (s *CapabilitySigner) Private(accessID string, tokenVersion int) string {
	return s.sign(privateCapabilityPrefix, privateCapabilityDomain, accessID, tokenVersion)
}

func (s *CapabilitySigner) Open(accessID string, tokenVersion int) string {
	return s.sign(openCapabilityPrefix, openCapabilityDomain, accessID, tokenVersion)
}

func (s *CapabilitySigner) ParsePrivate(raw string) (string, int, error) {
	return s.parse(raw, privateCapabilityPrefix, privateCapabilityDomain)
}

func (s *CapabilitySigner) ParseOpen(raw string) (string, int, error) {
	return s.parse(raw, openCapabilityPrefix, openCapabilityDomain)
}

func (s *CapabilitySigner) sign(prefix, domain, accessID string, tokenVersion int) string {
	version := strconv.Itoa(tokenVersion)
	mac := hmac.New(sha256.New, s.key)
	_, _ = mac.Write([]byte(domain))
	_, _ = mac.Write([]byte{0})
	_, _ = mac.Write([]byte(accessID))
	_, _ = mac.Write([]byte{0})
	_, _ = mac.Write([]byte(version))
	proof := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return strings.Join([]string{prefix, accessID, version, proof}, ".")
}

func (s *CapabilitySigner) parse(raw, prefix, domain string) (string, int, error) {
	parts := strings.Split(raw, ".")
	if len(parts) != 4 || parts[0] != prefix || parts[1] == "" {
		return "", 0, ErrInvalidCapability
	}
	version, err := strconv.Atoi(parts[2])
	if err != nil || version < 1 {
		return "", 0, ErrInvalidCapability
	}
	expected := s.sign(prefix, domain, parts[1], version)
	if subtle.ConstantTimeCompare([]byte(raw), []byte(expected)) != 1 {
		return "", 0, ErrInvalidCapability
	}
	return parts[1], version, nil
}

func randomToken(bytes int) (string, error) {
	b := make([]byte, bytes)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate random token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func hashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func (s *CapabilitySigner) fingerprint(parts ...string) string {
	mac := hmac.New(sha256.New, s.key)
	_, _ = mac.Write([]byte("owl-invites/recovery-rate-limit/v1"))
	for _, part := range parts {
		_, _ = mac.Write([]byte{0})
		_, _ = mac.Write([]byte(part))
	}
	return hex.EncodeToString(mac.Sum(nil))
}
