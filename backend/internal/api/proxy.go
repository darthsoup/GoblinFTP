package api

import (
	"net"

	"github.com/labstack/echo/v4"

	"github.com/darthsoup/goblinftp/internal/config"
)

// IPExtractor matches the deployment's proxy config: with no trusted proxies the
// client-settable X-Forwarded-For is ignored, else the chain is walked right to left.
func IPExtractor(cfg *config.Config) echo.IPExtractor {
	if !cfg.TrustProxies() {
		return echo.ExtractIPDirect()
	}
	opts := trustOptions(cfg.Settings.Access.TrustedProxies)
	return echo.ExtractIPFromXFFHeader(opts...)
}

// trustOptions converts the configured ranges into echo trust options. Config
// validated every entry at startup; anything that still fails to parse is skipped.
func trustOptions(ranges []string) []echo.TrustOption {
	// echo's defaults trust every private range, too broad here: start from
	// "trust nothing" and add only what the operator listed.
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

// clientScheme reports the scheme the browser used: X-Forwarded-Proto is spoofable
// without a trusted proxy, but the only signal behind one (Caddy serves plain HTTP).
func clientScheme(c echo.Context, cfg *config.Config) string {
	if cfg.TrustProxies() {
		return c.Scheme()
	}
	if c.Request().TLS != nil {
		return "https"
	}
	return "http"
}
