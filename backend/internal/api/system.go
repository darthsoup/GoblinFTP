// backend/internal/api/system.go
package api

import (
	"github.com/labstack/echo/v4"
)

type systemVarsData struct {
	Language          string             `json:"language"`
	UI                systemUIVars       `json:"ui"`
	Branding          systemBrandingVars `json:"branding"`
	Upload            systemUploadVars   `json:"upload"`
	Connection        systemConnVars     `json:"connection"`
	Editor            systemEditorVars   `json:"editor"`
	LoginFormDisabled bool               `json:"loginFormDisabled"`
	SSOEnabled        bool               `json:"ssoEnabled"`
	// FrontendLogEnabled tells the SPA whether to forward browser errors
	// to POST /api/log/frontend.
	FrontendLogEnabled bool `json:"frontendLogEnabled"`
	// Version is the build version ("dev" outside release builds).
	Version string `json:"version"`
}

type systemBrandingVars struct {
	AppName          string  `json:"appName"`
	LogoURL          *string `json:"logoUrl"`
	LogoDarkURL      *string `json:"logoDarkUrl"`
	FaviconURL       *string `json:"faviconUrl"`
	PrimaryColor     *string `json:"primaryColor"`
	PrimaryTextColor *string `json:"primaryTextColor"`
	HideAttribution  bool    `json:"hideAttribution"`
	// ThemeCssURL is the per-tenant stylesheet the SPA injects at runtime; nil
	// when no tenant theme applies.
	ThemeCssURL *string `json:"themeCssUrl"`
}

type systemUIVars struct {
	PageTitle             string `json:"pageTitle"`
	ShowDotFiles          bool   `json:"showDotFiles"`
	ShowNavigationHistory bool   `json:"showNavigationHistory"`
}

type systemUploadVars struct {
	ChunkSize            int64 `json:"chunkSize"`
	MaxConcurrentUploads int   `json:"maxConcurrentUploads"`
}

type systemConnVars struct {
	AllowedTypes []string `json:"allowedTypes"`
	DisableChmod bool     `json:"disableChmod"`
	PresetHost   *string  `json:"presetHost"`
	PresetPort   *int     `json:"presetPort"`
	LockHost     bool     `json:"lockHost"`
	PassiveMode  bool     `json:"passiveMode"`
}

type systemEditorVars struct {
	Disabled          bool     `json:"disabled"`
	ViewOnly          bool     `json:"viewOnly"`
	AllowedExtensions []string `json:"allowedExtensions"`
}

func (h *Handler) SystemVars(c echo.Context) error {
	branding := systemBrandingVars{
		AppName:          h.cfg.Settings.Branding.AppName,
		LogoURL:          h.cfg.Settings.Branding.LogoURL,
		LogoDarkURL:      h.cfg.Settings.Branding.LogoDarkURL,
		FaviconURL:       h.cfg.Settings.Branding.FaviconURL,
		PrimaryColor:     h.cfg.Settings.Branding.PrimaryColor,
		PrimaryTextColor: h.cfg.Settings.Branding.PrimaryTextColor,
		HideAttribution:  h.cfg.Settings.Branding.HideAttribution,
	}

	// Per-tenant white-label theme (resolved from the SSO session or the host).
	// The response now depends on the cookie/host, so it must not be shared-cached.
	if themeCSS, tenantLogo, tenantLogoDark, tenantFavicon := h.resolveTenantBranding(c); themeCSS != nil {
		branding.ThemeCssURL = themeCSS
		if tenantLogo != nil { // tenant assets win when a theme resolves
			branding.LogoURL = tenantLogo
		}
		if tenantLogoDark != nil {
			branding.LogoDarkURL = tenantLogoDark
		}
		if tenantFavicon != nil {
			branding.FaviconURL = tenantFavicon
		}
	}
	// A logo doubles as a favicon when none is set explicitly (SVG/PNG both work
	// as tab icons), so any brand that ships only a logo still gets a tab icon.
	if branding.FaviconURL == nil {
		branding.FaviconURL = branding.LogoURL
	}
	c.Response().Header().Set("Cache-Control", "private, no-store")
	c.Response().Header().Set("Vary", "Cookie")

	return OK(c, systemVarsData{
		Language: h.cfg.Settings.Language,
		UI: systemUIVars{
			PageTitle:             h.cfg.Settings.UI.PageTitle,
			ShowDotFiles:          h.cfg.Settings.UI.ShowDotFiles,
			ShowNavigationHistory: h.cfg.Settings.UI.ShowNavigationHistory,
		},
		Branding: branding,
		Upload: systemUploadVars{
			ChunkSize:            h.cfg.ChunkSize,
			MaxConcurrentUploads: h.cfg.MaxConcurrentUploads,
		},
		Connection: systemConnVars{
			AllowedTypes: h.cfg.Settings.Connection.AllowedTypes,
			DisableChmod: h.cfg.Settings.Connection.DisableChmod,
			PresetHost:   h.cfg.Settings.Connection.PresetHost,
			PresetPort:   h.cfg.Settings.Connection.PresetPort,
			LockHost:     h.cfg.Settings.Connection.LockHost,
			PassiveMode:  h.cfg.Settings.Connection.PassiveMode,
		},
		Editor: systemEditorVars{
			Disabled:          h.cfg.Settings.Editor.Disabled,
			ViewOnly:          h.cfg.Settings.Editor.ViewOnly,
			AllowedExtensions: h.cfg.Settings.Editor.AllowedExtensions,
		},
		LoginFormDisabled:  h.cfg.DisableLoginForm,
		SSOEnabled:         h.cfg.SSOEnabled,
		FrontendLogEnabled: h.cfg.FrontendLogEnabled,
		Version:            h.version,
	})
}
