package config

import (
	"crypto/rand"
	"regexp"
)

// hexColorRe matches #RGB and #RRGGBB used by branding.primaryColor.
var hexColorRe = regexp.MustCompile(`^#([0-9a-fA-F]{3}|[0-9a-fA-F]{6})$`)

// UISettings holds the GFTP_UI_* values.
type UISettings struct {
	PageTitle             string
	ShowDotFiles          bool
	ShowNavigationHistory bool
}

// EditorSettings holds the GFTP_EDITOR_* values.
type EditorSettings struct {
	AllowedExtensions []string
	Disabled          bool
	ViewOnly          bool
}

// ConnectionSettings holds the GFTP_CONNECTION_* values.
type ConnectionSettings struct {
	AllowedTypes []string
	DisableChmod bool
	// PresetHost/PresetPort prefill the login form; LockHost makes the
	// host+port fields read-only. PassiveMode is the FTP passive default.
	PresetHost  *string
	PresetPort  *int
	LockHost    bool
	PassiveMode bool
	// FTPTLSInsecureSkipVerify disables FTPS (explicit TLS) certificate
	// verification. For self-signed or internal servers only, never end users.
	FTPTLSInsecureSkipVerify bool
}

// AccessSettings holds the GFTP_ACCESS_* values.
type AccessSettings struct {
	AllowedClientAddresses []string
	// TrustedProxies are CIDR ranges whose X-Forwarded-* headers are believed.
	// Empty means trust none, and the client address is the direct peer.
	TrustedProxies []string
}

// TrustProxies reports whether a proxy allowlist was configured.
func (c *Config) TrustProxies() bool { return len(c.Settings.Access.TrustedProxies) > 0 }

// BrandingSettings holds the GFTP_BRANDING_* white-labeling values, exposed to
// the SPA via /api/system/vars. Nil pointers mean "use the built-in default".
type BrandingSettings struct {
	AppName          string
	LogoURL          *string
	LogoDarkURL      *string // optional dark-mode logo (swapped client-side)
	FaviconURL       *string
	PrimaryColor     *string // hex, e.g. "#2563eb"
	PrimaryTextColor *string // hex; button/primary text, for a light accent
	HideAttribution  bool
}

// EmbedSettings holds the embed presentation knobs. The frame-ancestors
// allowlist lives on Config instead (GFTP_FRAME_ANCESTORS, also read by Caddy).
type EmbedSettings struct {
	// Chromeless: "auto" hides branding chrome only when the page is framed,
	// "on" always, "off" never.
	Chromeless string
}

// Settings groups the runtime-configurable UI/editor/connection/access values.
type Settings struct {
	Language   string
	UI         UISettings
	Editor     EditorSettings
	Connection ConnectionSettings
	Access     AccessSettings
	Branding   BrandingSettings
	Embed      EmbedSettings
}

// Config holds all runtime configuration for GoblinFTP.
type Config struct {
	Port                string
	LogLevel            string
	LogFormat           string
	LogFile             string
	LogFileMaxSizeMB    int
	LogFileMaxBackups   int
	LogFileMaxAgeDays   int
	FrontendLogEnabled  bool
	MetricsEnabled      bool
	MetricsPort         string
	SessionSecret       []byte
	DownloadTokenSecret []byte
	SSOEnabled          bool
	SSOSecret           []byte
	// FrameAncestors is the validated CSP frame-ancestors allowlist. Empty means
	// framing is denied. Env-only (GFTP_FRAME_ANCESTORS); see EmbedSettings.
	FrameAncestors       []string
	ChunkSize            int64
	MaxConcurrentUploads int
	DataDir              string
	LoginMaxAttempts     int
	LoginCooldownSeconds int
	SessionTTLSeconds    int
	SentryDSN            string
	SentryEnvironment    string
	SentryRelease        string
	SentrySampleRate     float64
	// SentryErrorSampleRate is the fraction of error events sent; 0 sends none.
	SentryErrorSampleRate     float64
	SentryCaptureRemoteErrors bool
	SentrySendSessionContext  bool
	LoginFormDisabled         bool
	S3Enabled                 bool
	S3Endpoint                string
	S3Bucket                  string
	S3Region                  string
	S3AccessKey               string
	S3SecretKey               string
	S3UsePathStyle            bool
	S3Prefix                  string
	S3TimeoutSeconds          int
	Settings                  Settings
}

// EmbeddingEnabled reports whether an iframe allowlist is configured. It is the
// single switch for the cross-site session-cookie policy.
func (c *Config) EmbeddingEnabled() bool { return len(c.FrameAncestors) > 0 }

func defaultSettings() Settings {
	return Settings{
		Language: "en",
		UI: UISettings{
			// Empty means "follow branding.appName"; the SPA falls back when
			// building the document title.
			PageTitle:             "",
			ShowDotFiles:          false,
			ShowNavigationHistory: true,
		},
		Editor: EditorSettings{
			// Text-editable defaults, aligned with the frontend editor's
			// syntax-highlighting support (CodeMirror language packages).
			AllowedExtensions: []string{
				"html", "htm", "xhtml", "css", "scss", "sass", "less",
				"js", "mjs", "cjs", "jsx", "ts", "tsx", "vue", "svelte",
				"php", "phtml", "py", "rb", "go", "rs", "java", "c", "h", "cpp", "hpp",
				"sh", "bash", "zsh", "pl", "lua",
				"json", "json5", "xml", "svg", "yaml", "yml", "toml", "ini", "conf",
				"cfg", "env", "properties", "sql", "csv", "tsv",
				"txt", "md", "markdown", "rst", "log",
				// dotfiles (the part after the dot)
				"htaccess", "htpasswd", "gitignore", "editorconfig",
				"twig", "ejs", "hbs", "mustache", "liquid", "erb", "j2",
			},
			Disabled: false,
			ViewOnly: false,
		},
		Connection: ConnectionSettings{
			AllowedTypes: []string{"ftp", "ftps", "sftp"},
			DisableChmod: false,
			PassiveMode:  true,
		},
		Access: AccessSettings{
			AllowedClientAddresses: []string{},
		},
		Branding: BrandingSettings{
			AppName: "GoblinFTP",
		},
		Embed: EmbedSettings{
			Chromeless: "auto",
		},
	}
}

func generateSecret(length int) ([]byte, error) {
	b := make([]byte, length)
	if _, err := rand.Read(b); err != nil {
		return nil, err
	}
	return b, nil
}
