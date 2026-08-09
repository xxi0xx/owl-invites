package security

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func captureProxyRequest(t *testing.T, entries []string, remoteAddr string, headers map[string]string) *http.Request {
	t.Helper()
	var captured *http.Request
	h := TrustedProxyMiddleware(entries)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = r.Clone(r.Context())
		w.WriteHeader(http.StatusNoContent)
	}))
	req := httptest.NewRequest(http.MethodGet, "http://example.test/", nil)
	req.RemoteAddr = remoteAddr
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	h.ServeHTTP(httptest.NewRecorder(), req)
	require.NotNil(t, captured)
	return captured
}

func TestTrustedProxyMiddlewareRejectsSpoofedHeadersFromUntrustedPeer(t *testing.T) {
	r := captureProxyRequest(t, []string{"10.0.0.0/8"}, "203.0.113.8:1234", map[string]string{
		"X-Forwarded-For":   "198.51.100.20",
		"X-Forwarded-Proto": "https",
	})

	assert.Equal(t, "203.0.113.8:1234", r.RemoteAddr)
	assert.Empty(t, r.Header.Get("X-Forwarded-For"))
	assert.Empty(t, r.Header.Get("X-Forwarded-Proto"))
	assert.False(t, ForwardedHeadersTrusted(r.Context()))
}

func TestTrustedProxyMiddlewareUsesFirstUntrustedAddressFromRight(t *testing.T) {
	r := captureProxyRequest(t, []string{"10.0.0.0/8", "192.0.2.0/24"}, "10.1.2.3:4321", map[string]string{
		"X-Forwarded-For": "198.51.100.99, 192.0.2.44",
	})

	assert.Equal(t, "198.51.100.99:4321", r.RemoteAddr)
	assert.True(t, ForwardedHeadersTrusted(r.Context()))
}

func TestTrustedProxyMiddlewareAcceptsCloudflareClientHeaderOnlyFromTrustedPeer(t *testing.T) {
	r := captureProxyRequest(t, []string{"127.0.0.1"}, "127.0.0.1:8080", map[string]string{
		"CF-Connecting-IP": "203.0.113.77",
		"X-Forwarded-For":  "198.51.100.55",
	})

	assert.Equal(t, "203.0.113.77:8080", r.RemoteAddr)
}
