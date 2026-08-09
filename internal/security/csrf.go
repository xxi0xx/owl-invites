package security

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
)

// csrfHMACKey is a package-level key generated once at startup. It is used to
// bind CSRF tokens to session cookie values via HMAC-SHA256 so that a token
// issued for one session cannot be replayed against another.
var csrfHMACKey []byte

func init() {
	csrfHMACKey = make([]byte, 32)
	if _, err := rand.Read(csrfHMACKey); err != nil {
		panic("security: failed to generate CSRF HMAC key: " + err.Error())
	}
}

// generateCSRFNonce produces a cryptographically random 32-byte nonce
// encoded as a 64-character hex string.
func generateCSRFNonce() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// computeCSRFHMAC returns HMAC-SHA256(nonce, sessionValue) as a hex string.
func computeCSRFHMAC(nonce, sessionValue string) string {
	mac := hmac.New(sha256.New, csrfHMACKey)
	mac.Write([]byte(nonce))
	mac.Write([]byte(sessionValue))
	return hex.EncodeToString(mac.Sum(nil))
}

// buildCSRFToken creates a session-bound CSRF token: "nonce.hmac".
// If sessionValue is empty (unauthenticated), the token is just the nonce
// for backwards compatibility.
func buildCSRFToken(sessionValue string) (string, error) {
	nonce, err := generateCSRFNonce()
	if err != nil {
		return "", err
	}
	if sessionValue == "" {
		return nonce, nil
	}
	return nonce + "." + computeCSRFHMAC(nonce, sessionValue), nil
}

// isSessionBoundToken reports whether a CSRF token is in the session-bound
// "nonce.hmac" form (i.e. it contains a dot).
func isSessionBoundToken(token string) bool {
	return strings.IndexByte(token, '.') > 0
}

// verifyCSRFToken validates a CSRF token against the session value.
//
//   - The header and cookie must always match exactly (double-submit).
//   - When a session is present (authenticated request) the token MUST be
//     session-bound ("nonce.hmac") and the HMAC must validate against the
//     current session value.  Plain, dot-less tokens are rejected so that a
//     token minted before login cannot be replayed after authentication.
//   - When no session is present (pre-login flows) a plain double-submit
//     token is accepted.
func verifyCSRFToken(cookieToken, headerToken, sessionValue string) bool {
	// The header and cookie must match exactly first.
	if subtle.ConstantTimeCompare([]byte(cookieToken), []byte(headerToken)) != 1 {
		return false
	}

	// Authenticated requests require a session-bound token.
	if sessionValue != "" {
		idx := strings.IndexByte(cookieToken, '.')
		if idx <= 0 {
			// Plain (dot-less) token is not bound to this session — reject.
			return false
		}
		nonce := cookieToken[:idx]
		providedMAC := cookieToken[idx+1:]
		expectedMAC := computeCSRFHMAC(nonce, sessionValue)
		return subtle.ConstantTimeCompare([]byte(providedMAC), []byte(expectedMAC)) == 1
	}

	// Unauthenticated request.  A session-bound token cannot be validated
	// without a session, so reject it; a plain token is accepted as-is.
	if isSessionBoundToken(cookieToken) {
		return false
	}
	return true
}

// isBoundToSession reports whether a session-bound token's HMAC validates
// against the given session value.
func isBoundToSession(token, sessionValue string) bool {
	if sessionValue == "" {
		return false
	}
	idx := strings.IndexByte(token, '.')
	if idx <= 0 {
		return false
	}
	expectedMAC := computeCSRFHMAC(token[:idx], sessionValue)
	return subtle.ConstantTimeCompare([]byte(token[idx+1:]), []byte(expectedMAC)) == 1
}

// needsCSRFRebind reports whether a fresh csrf_token cookie should be issued on
// a safe request.  A cookie is (re)issued when it is absent, or when an
// authenticated request carries a token that is not bound to the current
// session (e.g. a pre-login plain token persisting after login).
func needsCSRFRebind(r *http.Request, sessionValue string) bool {
	cookie, err := r.Cookie("csrf_token")
	if err != nil {
		return true
	}
	if sessionValue != "" && !isBoundToSession(cookie.Value, sessionValue) {
		return true
	}
	return false
}

// setCSRFCookie writes the csrf_token cookie. It is readable by JavaScript so
// the SPA can echo it back in the X-CSRF-Token header.
func setCSRFCookie(w http.ResponseWriter, r *http.Request, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     "csrf_token",
		Value:    token,
		Path:     "/",
		HttpOnly: false, // JS needs to read the cookie
		Secure:   isSecureRequest(r),
		SameSite: http.SameSiteStrictMode,
	})
}

// CSRFMiddleware returns middleware that implements the Synchronizer Token
// Pattern for CSRF protection.
//
//   - On safe requests (GET, HEAD, OPTIONS): a csrf_token cookie is set if
//     one does not already exist.  When a session cookie is present, the
//     token is bound to it via HMAC-SHA256.
//   - On mutation requests (POST, PUT, PATCH, DELETE): the X-CSRF-Token
//     request header must match the csrf_token cookie value and (if the
//     user is authenticated) the HMAC must match the current session.
//
// Paths listed in excludePaths are exempt from CSRF validation (e.g. public
// RSVP endpoints that use honeypot protection instead).
func CSRFMiddleware(excludePaths []string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Check if the current path is excluded.
			for _, p := range excludePaths {
				if strings.HasPrefix(r.URL.Path, p) {
					next.ServeHTTP(w, r)
					return
				}
			}

			// Skip CSRF for requests using Bearer token authentication.
			// CSRF protects cookie-based auth; token-based auth is inherently
			// safe from CSRF because the attacker cannot inject the header.
			if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
				next.ServeHTTP(w, r)
				return
			}

			// Read the session cookie value (may be empty for unauthenticated users).
			sessionValue := ""
			if sc, err := r.Cookie("session"); err == nil {
				sessionValue = sc.Value
			} else if invitationSession, invitationErr := r.Cookie("owl_invitation_session"); invitationErr == nil {
				// Invitation sessions are an independent authorization boundary.
				// Bind CSRF tokens to them exactly as organizer sessions so a token
				// minted for one household cannot be replayed in another.
				sessionValue = invitationSession.Value
			}

			switch r.Method {
			case http.MethodGet, http.MethodHead, http.MethodOptions:
				// Set a CSRF cookie when one is absent, or re-mint it when the
				// existing cookie is not correctly bound to the current session.
				// The latter handles the post-login transition: a token minted
				// before authentication (plain, dot-less) must be replaced with
				// a session-bound token so subsequent mutations validate.
				if needsCSRFRebind(r, sessionValue) {
					token, genErr := buildCSRFToken(sessionValue)
					if genErr != nil {
						http.Error(w, "internal server error", http.StatusInternalServerError)
						return
					}
					setCSRFCookie(w, r, token)
				}

				next.ServeHTTP(w, r)

			default:
				// Mutation methods: validate the CSRF token.
				cookie, err := r.Cookie("csrf_token")
				if err != nil {
					csrfError(w)
					return
				}

				headerToken := r.Header.Get("X-CSRF-Token")
				if headerToken == "" {
					csrfError(w)
					return
				}

				if !verifyCSRFToken(cookie.Value, headerToken, sessionValue) {
					// If the request is authenticated but the cookie is not yet
					// session-bound, mint a fresh bound token so the client's
					// next request can succeed (the post-login transition).
					if sessionValue != "" && !isBoundToSession(cookie.Value, sessionValue) {
						if token, genErr := buildCSRFToken(sessionValue); genErr == nil {
							setCSRFCookie(w, r, token)
						}
					}
					csrfError(w)
					return
				}

				next.ServeHTTP(w, r)
			}
		})
	}
}

// csrfError writes a 403 Forbidden JSON response for CSRF validation failures.
func csrfError(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusForbidden)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"error":   "csrf_validation_failed",
		"message": "CSRF token missing or invalid.",
	})
}

// isSecureRequest returns true when the request was made over HTTPS,
// indicating that cookies should be marked Secure.
func isSecureRequest(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	// Respect the forwarded scheme only after TrustedProxyMiddleware verified
	// that the immediate peer is configured as trusted.
	if ForwardedHeadersTrusted(r.Context()) && strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
		return true
	}
	return false
}
