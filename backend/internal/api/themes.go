package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/labstack/echo/v4"
)

// tenantSessionKey holds the (validated) white-label tenant on the session,
// carried from the SSO token so it survives the pending→connected transition.
const tenantSessionKey = "tenant"

// tenantRe bounds a tenant identifier to a filesystem- and URL-safe slug. It
// doubles as the first line of defense against path traversal (no '.', '/').
var tenantRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)

// themeFileRe allowlists the exact filenames a tenant theme directory may serve.
var themeFileRe = regexp.MustCompile(`^(config\.css|logo(-dark)?\.(png|svg|webp|jpg)|favicon\.(ico|png|svg))$`)

// sanitizeTenant returns the tenant if it matches tenantRe, else "".
func sanitizeTenant(s string) string {
	if tenantRe.MatchString(s) {
		return s
	}
	return ""
}

// normalizeHost lowercases a Host header value and strips any :port.
func normalizeHost(host string) string {
	if i := strings.IndexByte(host, ':'); i >= 0 {
		host = host[:i]
	}
	return strings.ToLower(strings.TrimSpace(host))
}

// themeManifest is the optional <dataDir>/themes/<tenant>/theme.json. Only the
// custom-domain mapping is read today; unknown JSON fields are ignored.
type themeManifest struct {
	// Hosts are the exact FQDNs (case-insensitive) that select this tenant,
	// matched by string only and never used in a filesystem path.
	Hosts []string `json:"hosts"`
}

// themeManifestHosts reads and normalizes the "hosts" list from a theme dir's
// theme.json. Missing file or malformed JSON → no hosts (fail-soft).
func themeManifestHosts(dir string) []string {
	b, err := os.ReadFile(filepath.Join(dir, "theme.json"))
	if err != nil {
		return nil
	}
	var m themeManifest
	if json.Unmarshal(b, &m) != nil {
		return nil
	}
	out := make([]string, 0, len(m.Hosts))
	for _, h := range m.Hosts {
		if n := normalizeHost(h); n != "" {
			out = append(out, n)
		}
	}
	return out
}

// buildThemeHostMap scans <dataDir>/themes/*/theme.json into a host→tenant map.
// On a duplicate host the alphabetically first tenant wins (os.ReadDir is sorted).
func buildThemeHostMap(dataDir string) map[string]string {
	out := map[string]string{}
	if dataDir == "" {
		return out
	}
	root := filepath.Join(dataDir, "themes")
	entries, err := os.ReadDir(root)
	if err != nil {
		return out
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		tenant := sanitizeTenant(e.Name())
		if tenant == "" {
			continue
		}
		for _, hh := range themeManifestHosts(filepath.Join(root, tenant)) {
			if _, exists := out[hh]; !exists {
				out[hh] = tenant
			}
		}
	}
	return out
}

// themeHostIndex caches the host→tenant map, rebuilt at most once per ttl so the
// unauthenticated SystemVars endpoint never scans directories per request.
type themeHostIndex struct {
	dataDir string
	ttl     time.Duration
	mu      sync.Mutex
	hosts   map[string]string
	builtAt time.Time
}

func newThemeHostIndex(dataDir string) *themeHostIndex {
	return &themeHostIndex{dataDir: dataDir, ttl: 15 * time.Second}
}

// tenantFor returns the tenant whose theme.json lists host, or "".
func (idx *themeHostIndex) tenantFor(host string) string {
	host = normalizeHost(host)
	if host == "" {
		return ""
	}
	idx.mu.Lock()
	defer idx.mu.Unlock()
	if idx.hosts == nil || time.Since(idx.builtAt) > idx.ttl {
		idx.hosts = buildThemeHostMap(idx.dataDir)
		idx.builtAt = time.Now()
	}
	return idx.hosts[host]
}

// resolveTenantTheme returns cache-busted asset URLs for <dataDir>/themes/<tenant>/.
// A theme requires config.css; fail-soft, ok=false leaves the caller's branding.
func resolveTenantTheme(dataDir, tenant string) (cssURL string, logoURL, logoDarkURL, faviconURL *string, ok bool) {
	tenant = sanitizeTenant(tenant)
	if tenant == "" || dataDir == "" {
		return "", nil, nil, nil, false
	}
	dir := filepath.Join(dataDir, "themes", tenant)
	info, err := os.Stat(filepath.Join(dir, "config.css"))
	if err != nil || info.IsDir() {
		return "", nil, nil, nil, false
	}
	cssURL = fmt.Sprintf("/themes/%s/config.css?v=%d", tenant, info.ModTime().Unix())
	if u := firstThemeAsset(dir, tenant, "logo", "png", "svg", "webp", "jpg"); u != "" {
		logoURL = &u
	}
	// Optional dark-mode logo, swapped client-side by color mode: a dark-ink
	// wordmark would otherwise vanish on the dark canvas.
	if u := firstThemeAsset(dir, tenant, "logo-dark", "png", "svg", "webp", "jpg"); u != "" {
		logoDarkURL = &u
	}
	if u := firstThemeAsset(dir, tenant, "favicon", "ico", "png", "svg"); u != "" {
		faviconURL = &u
	}
	return cssURL, logoURL, logoDarkURL, faviconURL, true
}

// firstThemeAsset returns a cache-busted URL for the first existing
// <base>.<ext> in dir, or "" if none exists.
func firstThemeAsset(dir, tenant, base string, exts ...string) string {
	for _, ext := range exts {
		if info, err := os.Stat(filepath.Join(dir, base+"."+ext)); err == nil && !info.IsDir() {
			return fmt.Sprintf("/themes/%s/%s.%s?v=%d", tenant, base, ext, info.ModTime().Unix())
		}
	}
	return ""
}

// themeContentType maps an allowlisted extension to an explicit Content-Type so
// config.css applies as a stylesheet whatever the host OS mime registry says.
var themeContentType = map[string]string{
	".css":  "text/css; charset=utf-8",
	".png":  "image/png",
	".svg":  "image/svg+xml",
	".webp": "image/webp",
	".jpg":  "image/jpeg",
	".ico":  "image/x-icon",
}

// ServeTheme handles GET /themes/:tenant/:file, the per-tenant static assets.
// Public (branding is not secret); params allowlisted, path confined to the root.
func (h *Handler) ServeTheme(c echo.Context) error {
	tenant := sanitizeTenant(c.Param("tenant"))
	file := c.Param("file")
	if tenant == "" || !themeFileRe.MatchString(file) {
		return c.NoContent(http.StatusNotFound)
	}
	themesRoot := filepath.Join(h.cfg.DataDir, "themes")
	p := filepath.Clean(filepath.Join(themesRoot, tenant, file))
	if !strings.HasPrefix(p, themesRoot+string(os.PathSeparator)) {
		return c.NoContent(http.StatusNotFound)
	}
	if info, err := os.Stat(p); err != nil || info.IsDir() {
		return c.NoContent(http.StatusNotFound)
	}
	if ct := themeContentType[strings.ToLower(filepath.Ext(p))]; ct != "" {
		c.Response().Header().Set("Content-Type", ct)
	}
	// Assets are cache-busted via ?v=<mtime>, so a short public cache is safe.
	c.Response().Header().Set("Cache-Control", "public, max-age=300")
	// An SVG navigated to directly is a document and can execute script on this
	// origin. Overrides the app-wide CSP for theme assets only.
	c.Response().Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; sandbox")
	return c.File(p)
}

// resolveTenantBranding resolves the request's tenant (SSO session first, then
// host) and returns its theme asset URLs, or all-nil when no theme applies.
func (h *Handler) resolveTenantBranding(c echo.Context) (themeCSS, logo, logoDark, favicon *string) {
	tenant := ""
	if sess, ok := lookupSession(c, h.store); ok {
		tenant = sess.GetString(tenantSessionKey)
	}
	if tenant == "" {
		// Custom domain → tenant, from each theme's theme.json "hosts" list.
		tenant = h.themeHosts.tenantFor(c.Request().Host)
	}
	css, logoURL, logoDarkURL, faviconURL, ok := resolveTenantTheme(h.cfg.DataDir, tenant)
	if !ok {
		return nil, nil, nil, nil
	}
	return &css, logoURL, logoDarkURL, faviconURL
}
