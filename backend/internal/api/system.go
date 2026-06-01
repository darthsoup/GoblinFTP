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
	LoginFormDisabled bool             `json:"loginFormDisabled"`
	SSOEnabled        bool             `json:"ssoEnabled"`
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
		},
		LoginFormDisabled: h.cfg.DisableLoginForm,
		SSOEnabled:        h.cfg.SSOEnabled,
	})
}
