package api

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/labstack/echo/v4"

	gftperrors "github.com/darthsoup/goblinftp/internal/errors"
)

// APIError is a single error entry in a failure response.
type APIError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Response is the standard API envelope for all endpoints.
type Response struct {
	Success bool       `json:"success"`
	Data    any        `json:"data,omitempty"`
	Errors  []APIError `json:"errors,omitempty"`
}

// OK writes a 200 success response with the given data payload.
func OK(c echo.Context, data any) error {
	return c.JSON(http.StatusOK, Response{Success: true, Data: data})
}

// LoggedErrorKey is where Fail stashes the first GFTPError for the request
// logger. Handlers return nil after Fail, so it cannot ride the return value.
const LoggedErrorKey = "gftp_logged_error"

// Fail writes an error response. The HTTP status code comes from the first error's HTTPStatus().
func Fail(c echo.Context, errs ...*gftperrors.GFTPError) error {
	if len(errs) > 0 {
		c.Set(LoggedErrorKey, errs[0])
	}
	apiErrors := make([]APIError, len(errs))
	status := http.StatusInternalServerError
	if len(errs) > 0 {
		status = errs[0].HTTPStatus()
	}
	for i, e := range errs {
		apiErrors[i] = APIError{Code: string(e.Code()), Message: e.Error()}
	}
	return c.JSON(status, Response{Success: false, Errors: apiErrors})
}

// NotImplemented returns a 501 stub response for routes planned in future phases.
func NotImplemented(c echo.Context) error {
	return Fail(c, gftperrors.New(gftperrors.ErrNotImplemented, "not implemented in this phase"))
}

// bindJSON separates a malformed or oversized body from a valid body that fails
// field validation. Folding both into "path is required" told the caller to fix
// a field when the real problem was the payload.
func bindJSON(c echo.Context, req any) *gftperrors.GFTPError {
	if err := c.Bind(req); err != nil {
		var httpErr *echo.HTTPError
		if errors.As(err, &httpErr) && httpErr.Code == http.StatusRequestEntityTooLarge {
			return gftperrors.New(gftperrors.ErrFileTooLarge, "the request body is too large").WithCause(err)
		}
		return gftperrors.New(gftperrors.ErrBadRequest, "the request body could not be read").WithCause(err)
	}
	return nil
}

// httpErrorHandler envelopes the failures echo raises before or outside a
// handler (no route, oversized body, unreadable JSON) so the SPA never has to
// parse echo's own {"message":...} shape.
func httpErrorHandler(logger *slog.Logger) echo.HTTPErrorHandler {
	return func(err error, c echo.Context) {
		if c.Response().Committed {
			return
		}

		code := gftperrors.ErrInternal
		message := "an unexpected error occurred"

		var httpErr *echo.HTTPError
		if errors.As(err, &httpErr) {
			switch httpErr.Code {
			case http.StatusNotFound:
				code, message = gftperrors.ErrFileNotFound, "the requested endpoint does not exist"
			case http.StatusMethodNotAllowed:
				code, message = gftperrors.ErrBadRequest, "that method is not allowed on this endpoint"
			case http.StatusRequestEntityTooLarge:
				code, message = gftperrors.ErrFileTooLarge, "the request body is too large"
			case http.StatusBadRequest:
				code, message = gftperrors.ErrBadRequest, "the request could not be read"
			case http.StatusUnsupportedMediaType:
				code, message = gftperrors.ErrBadRequest, "unsupported content type"
			}
		}

		if failErr := Fail(c, gftperrors.New(code, message).WithCause(err)); failErr != nil && logger != nil {
			logger.LogAttrs(c.Request().Context(), slog.LevelError, "error response failed",
				slog.String("error", failErr.Error()))
		}
	}
}
