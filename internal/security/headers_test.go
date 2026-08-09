package security

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSecurityHeadersMiddleware_BaselineSet(t *testing.T) {
	h := SecurityHeadersMiddleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "http://example.test/", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	assert.Equal(t, "nosniff", rec.Header().Get("X-Content-Type-Options"))
	assert.Equal(t, "DENY", rec.Header().Get("X-Frame-Options"))
	assert.Equal(t, "no-referrer", rec.Header().Get("Referrer-Policy"))
	assert.Equal(t, "same-origin", rec.Header().Get("Cross-Origin-Opener-Policy"))
}

func TestSecurityHeadersMiddleware_HSTSOnlyOverHTTPS(t *testing.T) {
	h := SecurityHeadersMiddleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	plainReq := httptest.NewRequest(http.MethodGet, "http://example.test/", nil)
	plainRec := httptest.NewRecorder()
	h.ServeHTTP(plainRec, plainReq)
	assert.Empty(t, plainRec.Header().Get("Strict-Transport-Security"), "HSTS must not be set on plain HTTP")

	tlsReq := httptest.NewRequest(http.MethodGet, "http://example.test/", nil)
	tlsReq.RemoteAddr = "127.0.0.1:1234"
	tlsReq.Header.Set("X-Forwarded-Proto", "https")
	tlsRec := httptest.NewRecorder()
	TrustedProxyMiddleware([]string{"127.0.0.1"})(h).ServeHTTP(tlsRec, tlsReq)
	assert.Contains(t, tlsRec.Header().Get("Strict-Transport-Security"), "max-age=", "HSTS must be set behind HTTPS proxy")
}

func TestSecurityHeadersMiddleware_DoesNotTrustSpoofedForwardedProto(t *testing.T) {
	h := TrustedProxyMiddleware(nil)(SecurityHeadersMiddleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})))
	req := httptest.NewRequest(http.MethodGet, "http://example.test/", nil)
	req.RemoteAddr = "203.0.113.10:1234"
	req.Header.Set("X-Forwarded-Proto", "https")
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	assert.Empty(t, rec.Header().Get("Strict-Transport-Security"))
}
