package config

import (
	"fmt"
	"net/url"
	"strings"
)

// parseFrameAncestors validates GFTP_FRAME_ANCESTORS into a CSP source list;
// empty denies framing. Not a regex: single-label hosts (compose, k8s) must pass.
func parseFrameAncestors(raw string) ([]string, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	// Caddy interpolates this verbatim into a space-separated directive, so a
	// comma would silently produce an invalid policy.
	if strings.Contains(raw, ",") {
		return nil, fmt.Errorf("invalid GFTP_FRAME_ANCESTORS: separate origins with spaces, not commas (got %q)", raw)
	}

	seen := map[string]bool{}
	out := []string{}
	for _, token := range strings.Fields(raw) {
		origin := strings.ToLower(token)
		if origin == "*" || origin == "https://*" || origin == "http://*" {
			return nil, fmt.Errorf("invalid GFTP_FRAME_ANCESTORS: %q - allowing every origin defeats the allowlist; list them explicitly", token)
		}
		if !strings.Contains(origin, "://") {
			return nil, fmt.Errorf("invalid GFTP_FRAME_ANCESTORS: %q - origins need a scheme, e.g. https://panel.example.com", token)
		}
		// A leftmost-label wildcard is legal CSP but not a legal URL host, so
		// swap in a placeholder for parsing and restore it afterwards.
		parsed := origin
		wildcard := false
		if idx := strings.Index(parsed, "://*."); idx != -1 {
			wildcard = true
			// Drop the "*." so the remainder is a parseable URL; len("://") is 3
			// and len("://*.") is 5.
			parsed = parsed[:idx+3] + parsed[idx+5:]
		}
		u, err := url.Parse(parsed)
		if err != nil {
			return nil, fmt.Errorf("invalid GFTP_FRAME_ANCESTORS: %q - %w", token, err)
		}
		switch {
		case u.Scheme != "http" && u.Scheme != "https":
			return nil, fmt.Errorf("invalid GFTP_FRAME_ANCESTORS: %q - scheme must be http or https", token)
		case u.Host == "":
			return nil, fmt.Errorf("invalid GFTP_FRAME_ANCESTORS: %q - missing host", token)
		case u.User != nil:
			return nil, fmt.Errorf("invalid GFTP_FRAME_ANCESTORS: %q - must not carry credentials", token)
		case u.Path != "" && u.Path != "/", u.RawQuery != "", u.Fragment != "":
			return nil, fmt.Errorf("invalid GFTP_FRAME_ANCESTORS: %q - expected scheme://host[:port] with no path, query or fragment", token)
		case u.Path == "/":
			return nil, fmt.Errorf("invalid GFTP_FRAME_ANCESTORS: %q - drop the trailing slash", token)
		}
		// A wildcard needs a registrable domain under it, or https://*.com would
		// hand framing rights to an entire TLD.
		if wildcard && strings.Count(u.Hostname(), ".") < 1 {
			return nil, fmt.Errorf("invalid GFTP_FRAME_ANCESTORS: %q - a wildcard needs at least two labels beneath it, e.g. https://*.example.com", token)
		}
		// Plain http cannot receive the SameSite=None; Secure session cookie the
		// embed policy sets, so it would frame successfully and never log in.
		if u.Scheme == "http" && !isLoopbackHost(u.Hostname()) {
			return nil, fmt.Errorf("invalid GFTP_FRAME_ANCESTORS: %q - an http:// embedder cannot receive the Secure session cookie; use https:// (http://localhost is allowed for development)", token)
		}
		if !seen[origin] {
			seen[origin] = true
			out = append(out, origin)
		}
	}
	return out, nil
}

func isLoopbackHost(host string) bool {
	return host == "localhost" || host == "127.0.0.1" || host == "::1" || host == "[::1]"
}
