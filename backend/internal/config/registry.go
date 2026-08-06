package config

import (
	"fmt"
	"log/slog"
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// Key is one configuration knob; loader, generated artifacts, and drift tests
// all iterate Registry, so a knob is defined here and nowhere else.
type Key struct {
	// Env is the GFTP_* variable name.
	Env string
	// Secret marks values that must never appear in logs or rendered examples.
	Secret bool
	// SPA keys are surfaced via /api/system/vars at the SPAPath JSON path.
	SPA     bool
	SPAPath string
	// Section groups keys in generated artifacts; DocPage names the docs file
	// owning the key's reference table row.
	Section string
	DocPage string
	// Doc is the one-line description for generated doc tables.
	Doc string

	fromEnv func(c *Config, raw string) error
	render  func(c *Config) string

	applyAutogen func(c *Config, logger *slog.Logger) error

	kind          string
	enumAllowed   []string
	listAllowed   []string
	intMin        int64
	intMax        int64
	floatMin      float64
	floatMax      float64
	matchRe       *regexp.Regexp
	matchHint     string
	autogenLen    int
	autogenWarn   string
	sampleValid   string
	sampleInvalid string
}

type keyOpt func(*Key)

func spaPath(p string) keyOpt    { return func(k *Key) { k.SPA = true; k.SPAPath = p } }
func doc(s string) keyOpt        { return func(k *Key) { k.Doc = s } }
func secret() keyOpt             { return func(k *Key) { k.Secret = true } }
func minMax(lo, hi int64) keyOpt { return func(k *Key) { k.intMin = lo; k.intMax = hi } }
func positive() keyOpt           { return minMax(1, math.MaxInt64) }
func nonNegative() keyOpt        { return minMax(0, math.MaxInt64) }

func floatRange(lo, hi float64) keyOpt {
	return func(k *Key) { k.floatMin = lo; k.floatMax = hi }
}

func matches(re *regexp.Regexp, hint string) keyOpt {
	return func(k *Key) { k.matchRe = re; k.matchHint = hint }
}

func subsetOf(values ...string) keyOpt {
	return func(k *Key) { k.listAllowed = values }
}

func autogen(n int, warn string) keyOpt {
	return func(k *Key) { k.autogenLen = n; k.autogenWarn = warn }
}

func samples(valid, invalid string) keyOpt {
	return func(k *Key) { k.sampleValid = valid; k.sampleInvalid = invalid }
}

func newKey(env, kind string, opts []keyOpt) Key {
	k := Key{
		Env: env, kind: kind,
		intMin: math.MinInt64, intMax: math.MaxInt64,
		floatMin: math.Inf(-1), floatMax: math.Inf(1),
	}
	for _, o := range opts {
		o(&k)
	}
	return k
}

func rangeErr(name string, n, lo, hi int64) error {
	switch {
	case lo == 1 && hi == math.MaxInt64:
		return fmt.Errorf("invalid %s: must be positive, got %d", name, n)
	case lo == 0 && hi == math.MaxInt64:
		return fmt.Errorf("invalid %s: must not be negative, got %d", name, n)
	default:
		return fmt.Errorf("invalid %s: must be between %d and %d, got %d", name, lo, hi, n)
	}
}

func str(env string, target func(*Config) *string, opts ...keyOpt) Key {
	k := newKey(env, "string", opts)
	k.fromEnv = func(c *Config, raw string) error { *target(c) = raw; return nil }
	k.render = func(c *Config) string { return *target(c) }
	return k
}

func optStr(env string, target func(*Config) **string, opts ...keyOpt) Key {
	k := newKey(env, "optString", opts)
	re, hint := k.matchRe, k.matchHint
	k.fromEnv = func(c *Config, raw string) error {
		if re != nil && !re.MatchString(raw) {
			return fmt.Errorf("invalid %s: %s, got %q", env, hint, raw)
		}
		*target(c) = &raw
		return nil
	}
	k.render = func(c *Config) string {
		if p := *target(c); p != nil {
			return *p
		}
		return ""
	}
	return k
}

func boolean(env string, target func(*Config) *bool, opts ...keyOpt) Key {
	k := newKey(env, "bool", opts)
	k.fromEnv = func(c *Config, raw string) error {
		v, err := strconv.ParseBool(raw)
		if err != nil {
			return fmt.Errorf("invalid %s: must be true or false, got %q", env, raw)
		}
		*target(c) = v
		return nil
	}
	k.render = func(c *Config) string { return strconv.FormatBool(*target(c)) }
	return k
}

func integer(env string, target func(*Config) *int, opts ...keyOpt) Key {
	k := newKey(env, "int", opts)
	lo, hi := k.intMin, k.intMax
	k.fromEnv = func(c *Config, raw string) error {
		n, err := strconv.Atoi(raw)
		if err != nil {
			return fmt.Errorf("invalid %s: %w", env, err)
		}
		if int64(n) < lo || int64(n) > hi {
			return rangeErr(env, int64(n), lo, hi)
		}
		*target(c) = n
		return nil
	}
	k.render = func(c *Config) string { return strconv.Itoa(*target(c)) }
	return k
}

func optInt(env string, target func(*Config) **int, opts ...keyOpt) Key {
	k := newKey(env, "optInt", opts)
	lo, hi := k.intMin, k.intMax
	k.fromEnv = func(c *Config, raw string) error {
		n, err := strconv.Atoi(raw)
		if err != nil {
			return fmt.Errorf("invalid %s: %w", env, err)
		}
		if int64(n) < lo || int64(n) > hi {
			return rangeErr(env, int64(n), lo, hi)
		}
		*target(c) = &n
		return nil
	}
	k.render = func(c *Config) string {
		if p := *target(c); p != nil {
			return strconv.Itoa(*p)
		}
		return ""
	}
	return k
}

func int64Key(env string, target func(*Config) *int64, opts ...keyOpt) Key {
	k := newKey(env, "int64", opts)
	lo, hi := k.intMin, k.intMax
	k.fromEnv = func(c *Config, raw string) error {
		n, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return fmt.Errorf("invalid %s: %w", env, err)
		}
		if n < lo || n > hi {
			return rangeErr(env, n, lo, hi)
		}
		*target(c) = n
		return nil
	}
	k.render = func(c *Config) string { return strconv.FormatInt(*target(c), 10) }
	return k
}

func floatKey(env string, target func(*Config) *float64, opts ...keyOpt) Key {
	k := newKey(env, "float", opts)
	lo, hi := k.floatMin, k.floatMax
	k.fromEnv = func(c *Config, raw string) error {
		v, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			return fmt.Errorf("invalid %s: %w", env, err)
		}
		if v < lo || v > hi {
			return fmt.Errorf("invalid %s: must be between %v and %v, got %v", env, lo, hi, v)
		}
		*target(c) = v
		return nil
	}
	k.render = func(c *Config) string { return strconv.FormatFloat(*target(c), 'g', -1, 64) }
	return k
}

func enum(env string, target func(*Config) *string, allowed []string, opts ...keyOpt) Key {
	k := newKey(env, "enum", opts)
	k.enumAllowed = allowed
	k.fromEnv = func(c *Config, raw string) error {
		v := strings.ToLower(raw)
		if !slices.Contains(allowed, v) {
			return fmt.Errorf("invalid %s: must be one of %s, got %q", env, strings.Join(allowed, ", "), raw)
		}
		*target(c) = v
		return nil
	}
	k.render = func(c *Config) string { return *target(c) }
	return k
}

func list(env string, target func(*Config) *[]string, opts ...keyOpt) Key {
	k := newKey(env, "list", opts)
	allowed := k.listAllowed
	k.fromEnv = func(c *Config, raw string) error {
		vs := []string{}
		for _, part := range strings.Split(raw, ",") {
			if v := strings.TrimSpace(part); v != "" {
				vs = append(vs, v)
			}
		}
		if allowed != nil {
			for _, v := range vs {
				if !slices.Contains(allowed, v) {
					return fmt.Errorf("invalid %s: %q is not one of %s", env, v, strings.Join(allowed, ", "))
				}
			}
		}
		*target(c) = vs
		return nil
	}
	k.render = func(c *Config) string { return strings.Join(*target(c), ",") }
	return k
}

// portStr validates a numeric port but stores it as a string, keeping the
// ":"+cfg.Port call sites unchanged.
func portStr(env string, target func(*Config) *string, opts ...keyOpt) Key {
	k := newKey(env, "port", opts)
	k.fromEnv = func(c *Config, raw string) error {
		n, err := strconv.Atoi(raw)
		if err != nil {
			return fmt.Errorf("invalid %s: %w", env, err)
		}
		if n < 1 || n > 65535 {
			return fmt.Errorf("invalid %s: must be between 1 and 65535, got %d", env, n)
		}
		*target(c) = raw
		return nil
	}
	k.render = func(c *Config) string { return *target(c) }
	return k
}

func bytesSecret(env string, target func(*Config) *[]byte, opts ...keyOpt) Key {
	k := newKey(env, "secret", opts)
	k.Secret = true
	k.fromEnv = func(c *Config, raw string) error { *target(c) = []byte(raw); return nil }
	k.render = func(*Config) string { return "" }
	if k.autogenLen > 0 {
		n, warn := k.autogenLen, k.autogenWarn
		k.applyAutogen = func(c *Config, logger *slog.Logger) error {
			if len(*target(c)) > 0 {
				return nil
			}
			generated, err := generateSecret(n)
			if err != nil {
				return fmt.Errorf("failed to generate %s: %w", env, err)
			}
			*target(c) = generated
			if logger != nil {
				logger.Warn(warn)
			}
			return nil
		}
	}
	return k
}

// custom wires a key with bespoke parsing (e.g. frame-ancestors).
func custom(env string, fromEnv func(c *Config, raw string) error, opts ...keyOpt) Key {
	k := newKey(env, "custom", opts)
	k.fromEnv = fromEnv
	k.render = func(*Config) string { return "" }
	return k
}

// group stamps a section title and doc page on a block of keys.
func group(section, docPage string, keys ...Key) []Key {
	for i := range keys {
		keys[i].Section = section
		keys[i].DocPage = docPage
	}
	return keys
}

// Registry is the single definition of every configuration knob, in the order
// generated artifacts present them.
var Registry = slices.Concat(
	group("Server", "configuration",
		portStr("GFTP_PORT", func(c *Config) *string { return &c.Port },
			doc("Listen port of the Go backend, for from-source installs. Fixed at 8080 inside the container.")),
		str("GFTP_DATA_DIR", func(c *Config) *string { return &c.DataDir },
			doc("Writable data directory: SFTP known_hosts, local chunk staging, themes. For local dev use a relative path like data (just dev-be resolves it from the repo root).")),
	),
	group("UI", "configuration",
		str("GFTP_LANGUAGE", func(c *Config) *string { return &c.Settings.Language },
			spaPath("language"),
			doc("Default UI language. Users can override it locally; the SPA gates the actual set.")),
		str("GFTP_UI_PAGE_TITLE", func(c *Config) *string { return &c.Settings.UI.PageTitle },
			spaPath("ui.pageTitle"),
			doc("Browser tab title. Empty follows the branding app name.")),
		boolean("GFTP_UI_SHOW_DOT_FILES", func(c *Config) *bool { return &c.Settings.UI.ShowDotFiles },
			spaPath("ui.showDotFiles"),
			doc("Show dotfiles by default (users can override).")),
		boolean("GFTP_UI_SHOW_NAVIGATION_HISTORY", func(c *Config) *bool { return &c.Settings.UI.ShowNavigationHistory },
			spaPath("ui.showNavigationHistory"),
			doc("Show the recent-paths navigation history.")),
	),
	group("Editor", "configuration",
		list("GFTP_EDITOR_ALLOWED_EXTENSIONS", func(c *Config) *[]string { return &c.Settings.Editor.AllowedExtensions },
			spaPath("editor.allowedExtensions"),
			doc("Editable file extensions, without the dot.")),
		boolean("GFTP_EDITOR_DISABLED", func(c *Config) *bool { return &c.Settings.Editor.Disabled },
			spaPath("editor.disabled"),
			doc("Disable the file editor entirely.")),
		boolean("GFTP_EDITOR_VIEW_ONLY", func(c *Config) *bool { return &c.Settings.Editor.ViewOnly },
			spaPath("editor.viewOnly"),
			doc("Open files read-only.")),
	),
	group("Connections and TLS", "configuration",
		list("GFTP_CONNECTION_ALLOWED_TYPES", func(c *Config) *[]string { return &c.Settings.Connection.AllowedTypes },
			subsetOf("ftp", "ftps", "sftp"), spaPath("connection.allowedTypes"),
			doc("Protocols offered on the login form; any subset of ftp, ftps, sftp.")),
		boolean("GFTP_CONNECTION_DISABLE_CHMOD", func(c *Config) *bool { return &c.Settings.Connection.DisableChmod },
			spaPath("connection.disableChmod"),
			doc("Hide the chmod UI.")),
		optStr("GFTP_CONNECTION_PRESET_HOST", func(c *Config) **string { return &c.Settings.Connection.PresetHost },
			spaPath("connection.presetHost"),
			doc("Prefill the login form host.")),
		optInt("GFTP_CONNECTION_PRESET_PORT", func(c *Config) **int { return &c.Settings.Connection.PresetPort },
			minMax(1, 65535), spaPath("connection.presetPort"),
			doc("Prefill the login form port.")),
		boolean("GFTP_CONNECTION_LOCK_HOST", func(c *Config) *bool { return &c.Settings.Connection.LockHost },
			spaPath("connection.lockHost"),
			doc("Make host and port read-only on the login form; requires a preset host.")),
		boolean("GFTP_CONNECTION_PASSIVE_MODE", func(c *Config) *bool { return &c.Settings.Connection.PassiveMode },
			spaPath("connection.passiveMode"),
			doc("Default for the FTP passive-mode toggle.")),
		boolean("GFTP_CONNECTION_FTP_TLS_INSECURE_SKIP_VERIFY",
			func(c *Config) *bool { return &c.Settings.Connection.FTPTLSInsecureSkipVerify },
			doc("Skip FTPS (explicit TLS) certificate verification, for self-signed or internal servers only. Does not affect SFTP.")),
	),
	group("Access", "configuration",
		list("GFTP_ACCESS_ALLOWED_CLIENT_ADDRESSES", func(c *Config) *[]string { return &c.Settings.Access.AllowedClientAddresses },
			doc("Client IP allowlist; empty allows all.")),
	),
	group("White-label branding", "configuration",
		str("GFTP_BRANDING_APP_NAME", func(c *Config) *string { return &c.Settings.Branding.AppName },
			spaPath("branding.appName"),
			doc("App name in header, login card, title, and footer. Falls back to GoblinFTP when empty.")),
		optStr("GFTP_BRANDING_LOGO_URL", func(c *Config) **string { return &c.Settings.Branding.LogoURL },
			spaPath("branding.logoUrl"),
			doc("Logo image URL. A set logo hides the app-name text.")),
		optStr("GFTP_BRANDING_LOGO_DARK_URL", func(c *Config) **string { return &c.Settings.Branding.LogoDarkURL },
			spaPath("branding.logoDarkUrl"),
			doc("Dark-mode logo (a light wordmark), swapped in client-side under dark mode.")),
		optStr("GFTP_BRANDING_FAVICON_URL", func(c *Config) **string { return &c.Settings.Branding.FaviconURL },
			spaPath("branding.faviconUrl"),
			doc("Favicon image URL; falls back to the logo.")),
		optStr("GFTP_BRANDING_PRIMARY_COLOR", func(c *Config) **string { return &c.Settings.Branding.PrimaryColor },
			matches(hexColorRe, "must be a hex color like #2563eb"), samples("#2563eb", "blue"), spaPath("branding.primaryColor"),
			doc("Accent color as #RGB or #RRGGBB; recolors the theme at runtime.")),
		optStr("GFTP_BRANDING_PRIMARY_TEXT_COLOR", func(c *Config) **string { return &c.Settings.Branding.PrimaryTextColor },
			matches(hexColorRe, "must be a hex color like #0b1220"), samples("#0b1220", "blue"), spaPath("branding.primaryTextColor"),
			doc("Button/primary text color as hex; pair a light accent with dark text.")),
		boolean("GFTP_BRANDING_HIDE_ATTRIBUTION", func(c *Config) *bool { return &c.Settings.Branding.HideAttribution },
			spaPath("branding.hideAttribution"),
			doc("Hide the app-name/version footer attribution.")),
	),
	group("Iframe embedding", "embedding",
		custom("GFTP_FRAME_ANCESTORS", func(c *Config, raw string) error {
			fa, err := parseFrameAncestors(raw)
			if err != nil {
				return err
			}
			c.FrameAncestors = fa
			return nil
		},
			doc("Space-separated origins allowed to embed GoblinFTP in an iframe. Unset denies framing. Also read by Caddy.")),
		enum("GFTP_EMBED_CHROMELESS", func(c *Config) *string { return &c.Settings.Embed.Chromeless },
			[]string{"auto", "on", "off"}, spaPath("embed.chromeless"),
			doc("auto hides branding chrome only when framed, on always, off never.")),
	),
	group("Secrets", "configuration",
		bytesSecret("GFTP_SESSION_SECRET", func(c *Config) *[]byte { return &c.SessionSecret },
			autogen(32, "GFTP_SESSION_SECRET not set, using ephemeral random secret — sessions will be invalidated on restart"),
			doc("Session cookie signing key, used verbatim as bytes.")),
		bytesSecret("GFTP_DOWNLOAD_TOKEN_SECRET", func(c *Config) *[]byte { return &c.DownloadTokenSecret },
			autogen(32, "GFTP_DOWNLOAD_TOKEN_SECRET not set, using ephemeral random secret — download links will be invalidated on restart"),
			doc("HMAC key for signed download tokens.")),
	),
	group("Login and sessions", "configuration",
		integer("GFTP_SESSION_TTL_SECONDS", func(c *Config) *int { return &c.SessionTTLSeconds },
			positive(),
			doc("Session lifetime in seconds.")),
		integer("GFTP_LOGIN_MAX_ATTEMPTS", func(c *Config) *int { return &c.LoginMaxAttempts },
			positive(),
			doc("Failed connect attempts tolerated per host:username before the cooldown rejects further attempts.")),
		integer("GFTP_LOGIN_COOLDOWN_SECONDS", func(c *Config) *int { return &c.LoginCooldownSeconds },
			positive(),
			doc("Sliding login cooldown in seconds, keyed on host:username.")),
		boolean("GFTP_LOGIN_FORM_DISABLED", func(c *Config) *bool { return &c.LoginFormDisabled },
			spaPath("loginFormDisabled"),
			doc("Hide the manual login form (SSO-only deployments). A signed-out visitor gets a 404 with no preference controls.")),
	),
	group("SSO login links", "configuration",
		boolean("GFTP_SSO_ENABLED", func(c *Config) *bool { return &c.SSOEnabled },
			spaPath("ssoEnabled"),
			doc("Enable one-time SSO login links.")),
		bytesSecret("GFTP_SSO_SECRET", func(c *Config) *[]byte { return &c.SSOSecret },
			doc("Shared secret for SSO token validation. Required when SSO is enabled.")),
	),
	group("Uploads and chunking", "configuration",
		int64Key("GFTP_UPLOAD_CHUNK_SIZE", func(c *Config) *int64 { return &c.ChunkSize },
			positive(), spaPath("upload.chunkSize"),
			doc("Upload chunk size in bytes.")),
		integer("GFTP_UPLOAD_MAX_CONCURRENT", func(c *Config) *int { return &c.MaxConcurrentUploads },
			positive(), spaPath("upload.maxConcurrentUploads"),
			doc("Maximum parallel chunk uploads per session. A single control connection serializes transfers, so values above 1 mostly queue.")),
	),
	group("Logging", "logging",
		enum("GFTP_LOG_LEVEL", func(c *Config) *string { return &c.LogLevel },
			[]string{"debug", "info", "warn", "warning", "error"},
			doc("Log level.")),
		enum("GFTP_LOG_FORMAT", func(c *Config) *string { return &c.LogFormat },
			[]string{"json", "text"},
			doc("Log output format.")),
		str("GFTP_LOG_FILE", func(c *Config) *string { return &c.LogFile },
			doc("Optional rotating file sink in addition to stdout; empty disables it.")),
		integer("GFTP_LOG_FILE_MAX_SIZE_MB", func(c *Config) *int { return &c.LogFileMaxSizeMB },
			positive(),
			doc("Rotate the log file when it exceeds this size.")),
		integer("GFTP_LOG_FILE_MAX_BACKUPS", func(c *Config) *int { return &c.LogFileMaxBackups },
			nonNegative(),
			doc("Rotated files to keep; 0 keeps all.")),
		integer("GFTP_LOG_FILE_MAX_AGE_DAYS", func(c *Config) *int { return &c.LogFileMaxAgeDays },
			nonNegative(),
			doc("Days to keep rotated files; 0 keeps them indefinitely.")),
		boolean("GFTP_LOG_FRONTEND", func(c *Config) *bool { return &c.FrontendLogEnabled },
			spaPath("frontendLogEnabled"),
			doc("Forward browser errors to POST /api/log/frontend.")),
	),
	group("Metrics", "metrics",
		boolean("GFTP_METRICS_ENABLED", func(c *Config) *bool { return &c.MetricsEnabled },
			doc("Enable the Prometheus /metrics listener.")),
		portStr("GFTP_METRICS_PORT", func(c *Config) *string { return &c.MetricsPort },
			doc("Port for the metrics-only listener.")),
	),
	group("Error tracking (Sentry)", "configuration",
		str("GFTP_SENTRY_DSN", func(c *Config) *string { return &c.SentryDSN },
			secret(),
			doc("Backend DSN. Empty disables backend reporting.")),
		str("GFTP_SENTRY_ENVIRONMENT", func(c *Config) *string { return &c.SentryEnvironment },
			doc("Environment tag passed through verbatim.")),
		str("GFTP_SENTRY_RELEASE", func(c *Config) *string { return &c.SentryRelease },
			doc("Release tag; defaults to the build version.")),
		floatKey("GFTP_SENTRY_SAMPLE_RATE", func(c *Config) *float64 { return &c.SentrySampleRate },
			floatRange(0, 1), samples("0.25", "1.5"),
			doc("Traces sample rate between 0 and 1.")),
	),
	group("S3 chunk staging", "s3-staging",
		boolean("GFTP_S3_ENABLED", func(c *Config) *bool { return &c.S3Enabled },
			doc("Stage upload chunks in S3-compatible storage instead of local disk.")),
		str("GFTP_S3_ENDPOINT", func(c *Config) *string { return &c.S3Endpoint },
			doc("S3 endpoint URL including http:// or https://.")),
		str("GFTP_S3_BUCKET", func(c *Config) *string { return &c.S3Bucket },
			doc("Bucket for staged chunks.")),
		str("GFTP_S3_REGION", func(c *Config) *string { return &c.S3Region },
			doc("S3 region.")),
		str("GFTP_S3_ACCESS_KEY", func(c *Config) *string { return &c.S3AccessKey },
			secret(),
			doc("S3 access key.")),
		str("GFTP_S3_SECRET_KEY", func(c *Config) *string { return &c.S3SecretKey },
			secret(),
			doc("S3 secret key.")),
		boolean("GFTP_S3_USE_PATH_STYLE", func(c *Config) *bool { return &c.S3UsePathStyle },
			doc("Path-style addressing; true for MinIO, false for AWS.")),
		str("GFTP_S3_PREFIX", func(c *Config) *string { return &c.S3Prefix },
			doc("Key prefix for staged chunks.")),
		integer("GFTP_S3_TIMEOUT_SECONDS", func(c *Config) *int { return &c.S3TimeoutSeconds },
			positive(),
			doc("Per-operation S3 timeout in seconds.")),
	),
)

// Defaults returns a Config populated with every built-in default, for
// rendering generated artifacts.
func Defaults() *Config { return defaultConfig() }

// RenderDefault formats the key's value from cfg for generated artifacts;
// empty for secrets and keys without a rendered form.
func (k *Key) RenderDefault(cfg *Config) string {
	if k.render == nil {
		return ""
	}
	return k.render(cfg)
}

// Kind reports the key's value kind (string, bool, int, list, enum, ...).
func (k *Key) Kind() string { return k.kind }

// EnumValues lists the allowed values of an enum key, nil otherwise.
func (k *Key) EnumValues() []string { return k.enumAllowed }

// AutoGenerated reports whether an unset secret is generated at startup.
func (k *Key) AutoGenerated() bool { return k.autogenLen > 0 }

// SPAKey names one /api/system/vars field that is backed by a config key.
type SPAKey struct {
	Env      string
	JSONPath string
}

// SPAKeys lists every registry key flagged for the SPA, with the JSON path it
// must occupy in the /api/system/vars response.
func SPAKeys() []SPAKey {
	out := []SPAKey{}
	for _, k := range Registry {
		if k.SPA {
			out = append(out, SPAKey{Env: k.Env, JSONPath: k.SPAPath})
		}
	}
	return out
}
