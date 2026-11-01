package api

import (
	"path"

	"github.com/labstack/echo/v4"

	"github.com/darthsoup/goblinftp/internal/auth"
	gftperrors "github.com/darthsoup/goblinftp/internal/errors"
	"github.com/darthsoup/goblinftp/internal/transfer"
)

func windowsPaths(c echo.Context) bool {
	sess, _ := c.Get("session").(*auth.Session)
	return sess == nil || sess.GetString("protocol") != "sftp"
}

func validPath(c echo.Context, p string, root bool) bool {
	return transfer.ValidateRemotePath(p, windowsPaths(c), root) == nil
}

func sameParent(c echo.Context, a, b string) bool {
	return path.Dir(transfer.ComparisonPath(a, windowsPaths(c))) == path.Dir(transfer.ComparisonPath(b, windowsPaths(c)))
}

func validateServerDirectory(directory, protocol string) *gftperrors.GFTPError {
	if err := transfer.ValidateRemotePath(directory, protocol != "sftp", true); err != nil {
		return gftperrors.New(gftperrors.ErrUnsupportedServer, "Windows-style server paths are not supported. Use a server with absolute POSIX paths.").WithCause(err)
	}
	return nil
}
