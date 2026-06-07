// backend/internal/api/system.go
package api

import (
	"github.com/labstack/echo/v4"
)

type systemVarsData struct {
	Language          string           `json:"language"`
	UI                systemUIVars     `json:"ui"`
	Upload            systemUploadVars `json:"upload"`
	Connection        systemConnVars   `json:"connection"`
	Editor            systemEditorVars `json:"editor"`
	LoginFormDisabled bool             `json:"loginFormDisabled"`
	SSOEnabled        bool             `json:"ssoEnabled"`
	// FrontendLogEnabled tells the SPA whether to forward browser errors
	// to POST /api/log/frontend.
	FrontendLogEnabled bool `json:"frontendLogEnabled"`
	// Version is the build version ("dev" outside release builds).
	Version string `json:"version"`
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
	return OK(c, systemVarsData{
		Language: h.cfg.Settings.Language,
		UI: systemUIVars{
			PageTitle:             h.cfg.Settings.UI.PageTitle,
			ShowDotFiles:          h.cfg.Settings.UI.ShowDotFiles,
			ShowNavigationHistory: h.cfg.Settings.UI.ShowNavigationHistory,
		},
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
