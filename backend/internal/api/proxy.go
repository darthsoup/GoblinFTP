// backend/internal/api/proxy.go
package api

import (
	"net"

	"github.com/labstack/echo/v4"

	"github.com/darthsoup/goblinftp/internal/config"
)

// IPExtractor returns the echo.IPExtractor matching the deployment's proxy
// configuration.
//
// With no GFTP_ACCESS_TRUSTED_PROXIES set, forwarded headers are ignored
// entirely and the direct peer is the client: echo's default would otherwise
// take the leftmost X-Forwarded-For value, which any client can set.
//
// With trusted ranges configured, the chain is walked right to left and trusted
// hops are skipped, yielding the first address the deployment did not vouch
// for. This is the case that matters behind an external TLS terminator, where
// otherwise every client collapses to the terminator's address and the client
// allowlist and per-IP throttles stop distinguishing anyone.
func IPExtractor(cfg *config.Config) echo.IPExtractor {
	if !cfg.TrustProxies() {
		return echo.ExtractIPDirect()
	}
	opts := trustOptions(cfg.Settings.Access.TrustedProxies)
	return echo.ExtractIPFromXFFHeader(opts...)
}

// trustOptions converts the configured ranges into echo trust options. Config
// validates each entry as an IP or CIDR at startup, so a parse failure here is
// not reachable; entries that somehow fail are skipped rather than trusted.
func trustOptions(ranges []string) []echo.TrustOption {
	// The defaults trust all private ranges. That is too broad for a decision
	// this security-sensitive, so start from "trust nothing" and add only what
	// the operator listed.
	opts := []echo.TrustOption{
		echo.TrustLoopback(false),
		echo.TrustLinkLocal(false),
		echo.TrustPrivateNet(false),
	}
	for _, entry := range ranges {
		if _, ipnet, err := net.ParseCIDR(entry); err == nil {
			opts = append(opts, echo.TrustIPRange(ipnet))
			continue
		}
		if ip := net.ParseIP(entry); ip != nil {
			opts = append(opts, echo.TrustIPRange(singleHostNet(ip)))
		}
	}
	return opts
}

// singleHostNet turns a bare address into a /32 or /128 range.
func singleHostNet(ip net.IP) *net.IPNet {
	if v4 := ip.To4(); v4 != nil {
		return &net.IPNet{IP: v4, Mask: net.CIDRMask(32, 32)}
	}
	return &net.IPNet{IP: ip, Mask: net.CIDRMask(128, 128)}
}

// clientScheme reports the scheme the browser used. c.Scheme() reads
// X-Forwarded-Proto unconditionally, which is spoofable when no proxy is
// trusted; behind a trusted proxy it is the only source of truth, because Caddy
// listens on plain HTTP inside the container.
func clientScheme(c echo.Context, cfg *config.Config) string {
	if cfg.TrustProxies() {
		return c.Scheme()
	}
	if c.Request().TLS != nil {
		return "https"
	}
	return "http"
}
