package security

import (
	"context"
	"net"
	"net/http"
	"strings"
)

type forwardedTrustContextKey struct{}

// ForwardedHeadersTrusted reports whether the request arrived directly from a
// configured trusted proxy. Callers must not act on forwarded scheme or client
// identity headers unless this returns true.
func ForwardedHeadersTrusted(ctx context.Context) bool {
	trusted, _ := ctx.Value(forwardedTrustContextKey{}).(bool)
	return trusted
}

// TrustedProxyMiddleware accepts forwarded client identity only when the
// immediate network peer is inside TRUSTED_PROXIES. For X-Forwarded-For chains,
// it walks from the closest proxy toward the client and selects the first
// untrusted address, preventing a caller from prepending a spoofed address.
//
// Invalid proxy entries are ignored fail-closed. Config.Load rejects them for
// normal application startup; ignoring them here also keeps directly-created
// test configurations safe.
func TrustedProxyMiddleware(entries []string) func(http.Handler) http.Handler {
	networks := trustedNetworks(entries)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			peerIP, peerPort := remoteIPAndPort(r.RemoteAddr)
			if peerIP == nil || !ipInNetworks(peerIP, networks) {
				stripForwardedHeaders(r.Header)
				next.ServeHTTP(w, r)
				return
			}

			ctx := context.WithValue(r.Context(), forwardedTrustContextKey{}, true)
			r = r.WithContext(ctx)
			if clientIP := forwardedClientIP(r.Header, networks); clientIP != nil {
				if peerPort == "" {
					peerPort = "0"
				}
				r.RemoteAddr = net.JoinHostPort(clientIP.String(), peerPort)
			}
			next.ServeHTTP(w, r)
		})
	}
}

func trustedNetworks(entries []string) []*net.IPNet {
	networks := make([]*net.IPNet, 0, len(entries))
	for _, entry := range entries {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		if ip := net.ParseIP(entry); ip != nil {
			bits := 128
			if ip.To4() != nil {
				bits = 32
			}
			networks = append(networks, &net.IPNet{IP: ip, Mask: net.CIDRMask(bits, bits)})
			continue
		}
		if _, network, err := net.ParseCIDR(entry); err == nil {
			networks = append(networks, network)
		}
	}
	return networks
}

func remoteIPAndPort(remoteAddr string) (net.IP, string) {
	host, port, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		return net.ParseIP(strings.Trim(remoteAddr, "[]")), ""
	}
	return net.ParseIP(strings.Trim(host, "[]")), port
}

func ipInNetworks(ip net.IP, networks []*net.IPNet) bool {
	for _, network := range networks {
		if network.Contains(ip) {
			return true
		}
	}
	return false
}

func forwardedClientIP(header http.Header, trusted []*net.IPNet) net.IP {
	// Cloudflare overwrites CF-Connecting-IP before forwarding to a configured
	// origin. It is accepted only because the immediate peer was trusted above.
	if ip := net.ParseIP(strings.TrimSpace(header.Get("CF-Connecting-IP"))); ip != nil {
		return ip
	}

	if raw := header.Get("X-Forwarded-For"); raw != "" {
		parts := strings.Split(raw, ",")
		parsed := make([]net.IP, 0, len(parts))
		for _, part := range parts {
			ip := net.ParseIP(strings.TrimSpace(part))
			if ip == nil {
				return nil
			}
			parsed = append(parsed, ip)
		}
		for i := len(parsed) - 1; i >= 0; i-- {
			if !ipInNetworks(parsed[i], trusted) {
				return parsed[i]
			}
		}
		if len(parsed) > 0 {
			return parsed[0]
		}
	}

	return net.ParseIP(strings.TrimSpace(header.Get("X-Real-IP")))
}

func stripForwardedHeaders(header http.Header) {
	for _, name := range []string{
		"CF-Connecting-IP",
		"Forwarded",
		"X-Forwarded-For",
		"X-Forwarded-Host",
		"X-Forwarded-Proto",
		"X-Real-IP",
	} {
		header.Del(name)
	}
}
