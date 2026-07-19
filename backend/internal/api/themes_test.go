package api

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSanitizeTenant(t *testing.T) {
	valid := []string{"acme", "a", "acme-corp", "tenant_1", "a0", strings.Repeat("x", 64)}
	for _, s := range valid {
		if sanitizeTenant(s) != s {
			t.Errorf("sanitizeTenant(%q) = %q, want %q", s, sanitizeTenant(s), s)
		}
	}
	invalid := []string{"", "Acme", "-acme", "a.b", "a/b", "..", "../etc", "a b", strings.Repeat("x", 65), "café"}
	for _, s := range invalid {
		if got := sanitizeTenant(s); got != "" {
			t.Errorf("sanitizeTenant(%q) = %q, want \"\"", s, got)
		}
	}
}

func TestThemeHostMapping(t *testing.T) {
	dataDir := t.TempDir()
	write := func(rel, body string) {
		p := filepath.Join(dataDir, "themes", rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("acme/config.css", ":root{}")
	write("acme/theme.json", `{"hosts":["ftp.acme.com","Portal.ACME.io"]}`)
	write("beta/theme.json", `{"hosts":["files.beta.io"]}`) // no config.css — still indexed
	write("Upper/theme.json", `{"hosts":["skip.me"]}`)      // invalid slug dir → skipped
	write("broke/theme.json", `{bad json`)                  // malformed → no hosts

	idx := newThemeHostIndex(dataDir)
	cases := map[string]string{
		"ftp.acme.com":        "acme",
		"FTP.ACME.COM:8443":   "acme", // normalized (case + port)
		"portal.acme.io":      "acme", // manifest host was mixed-case
		"files.beta.io":       "beta",
		"skip.me":             "", // owning dir had an invalid slug name
		"unknown.example.com": "",
		"":                    "",
	}
	for host, want := range cases {
		if got := idx.tenantFor(host); got != want {
			t.Errorf("tenantFor(%q) = %q, want %q", host, got, want)
		}
	}

	// Malformed manifest yields no hosts rather than an error.
	if hs := themeManifestHosts(filepath.Join(dataDir, "themes", "broke")); hs != nil {
		t.Errorf("malformed theme.json hosts = %v, want nil", hs)
	}
}

func TestResolveTenantTheme(t *testing.T) {
	dataDir := t.TempDir()
	dir := filepath.Join(dataDir, "themes", "acme")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.css"), []byte(":root{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "logo.svg"), []byte("<svg/>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "logo-dark.svg"), []byte("<svg/>"), 0o644); err != nil {
		t.Fatal(err)
	}

	css, logo, logoDark, favicon, ok := resolveTenantTheme(dataDir, "acme")
	if !ok {
		t.Fatal("expected ok for existing theme")
	}
	if !strings.HasPrefix(css, "/themes/acme/config.css?v=") {
		t.Errorf("css URL = %q", css)
	}
	if logo == nil || !strings.HasPrefix(*logo, "/themes/acme/logo.svg?v=") {
		t.Errorf("logo URL = %v", logo)
	}
	if logoDark == nil || !strings.HasPrefix(*logoDark, "/themes/acme/logo-dark.svg?v=") {
		t.Errorf("logo-dark URL = %v", logoDark)
	}
	if favicon != nil {
		t.Errorf("favicon should be nil, got %v", favicon)
	}

	// Missing tenant, invalid slug, and empty dataDir all fail soft.
	for _, tenant := range []string{"ghost", "../etc", "Acme", ""} {
		if _, _, _, _, ok := resolveTenantTheme(dataDir, tenant); ok {
			t.Errorf("resolveTenantTheme(%q) unexpectedly ok", tenant)
		}
	}
	if _, _, _, _, ok := resolveTenantTheme("", "acme"); ok {
		t.Error("empty dataDir should not resolve")
	}
}
