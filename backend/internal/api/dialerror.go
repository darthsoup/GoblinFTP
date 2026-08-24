package api

import (
	"errors"
	"log/slog"

	"github.com/labstack/echo/v4"

	gftperrors "github.com/darthsoup/goblinftp/internal/errors"
	"github.com/darthsoup/goblinftp/internal/transfer"
)

// dialFailure is the decoded form of a failed dial, shared by Connect and
// SSOConnect so the two switches cannot drift apart.
type dialFailure struct {
	// metricLabel is the ConnectAttempts outcome label.
	metricLabel string
	// credentialAttempt is true only for a genuine credential rejection. The
	// per-account lockout keys on it, so a certificate problem or a read-only
	// known_hosts can no longer throttle a user out of their own account.
	// The per-IP budget is charged for every failure regardless: this endpoint
	// dials arbitrary hosts, so it stays rate limited against abuse.
	credentialAttempt bool
	err               *gftperrors.GFTPError
}

// classifyDial maps a dial error onto its API failure.
func classifyDial(dialErr error) dialFailure {
	switch {
	case errors.Is(dialErr, transfer.ErrAuthFailed):
		return dialFailure{
			metricLabel:       "auth_failed",
			credentialAttempt: true,
			err: gftperrors.New(gftperrors.ErrAuthFailed,
				"the server rejected the username or password").WithCause(dialErr),
		}
	case errors.Is(dialErr, transfer.ErrHostKeyMismatch):
		return dialFailure{
			metricLabel: "host_key_mismatch",
			err: gftperrors.New(gftperrors.ErrHostKeyMismatch,
				"the server's host key does not match the trusted one (possible man-in-the-middle), so the connection was refused").WithCause(dialErr),
		}
	case errors.Is(dialErr, transfer.ErrHostKeyStoreUnavailable):
		return dialFailure{
			metricLabel: "host_key_store_unavailable",
			err: gftperrors.New(gftperrors.ErrStorageUnavailable,
				"the known_hosts store could not be read or written, check that GFTP_DATA_DIR is writable").WithCause(dialErr),
		}
	case errors.Is(dialErr, transfer.ErrTLSFailed):
		return dialFailure{
			metricLabel: "tls_failed",
			err: gftperrors.New(gftperrors.ErrTLSFailed,
				"the server's TLS certificate could not be verified. Install its CA, or set GFTP_CONNECTION_FTP_TLS_INSECURE_SKIP_VERIFY for an internal server").WithCause(dialErr),
		}
	case errors.Is(dialErr, transfer.ErrConnectionTimeout):
		return dialFailure{
			metricLabel: "timeout",
			err: gftperrors.New(gftperrors.ErrConnectionTimeout,
				"the server did not respond in time").WithCause(dialErr),
		}
	case errors.Is(dialErr, transfer.ErrSubsystemUnavailable):
		return dialFailure{
			metricLabel: "subsystem_unavailable",
			err: gftperrors.New(gftperrors.ErrConnectionFailed,
				"the server accepted the login but does not provide an SFTP subsystem").WithCause(dialErr),
		}
	case errors.Is(dialErr, transfer.ErrDataConnectionFailed):
		return dialFailure{
			metricLabel: "data_connection_failed",
			err: gftperrors.New(gftperrors.ErrDataConnectionFailed,
				messageFor(gftperrors.ErrDataConnectionFailed)).WithCause(dialErr),
		}
	default:
		return dialFailure{
			metricLabel: "failed",
			err: gftperrors.New(gftperrors.ErrConnectionFailed,
				"the server could not be reached").WithCause(dialErr),
		}
	}
}

// hostKeyLabel separates a first-use prompt from a key that actually changed, so
// the two are distinguishable in the metrics rather than sharing one label.
func hostKeyLabel(prompt *HostKeyPrompt) string {
	if prompt != nil && prompt.Changed {
		return "host_key_changed"
	}
	return "host_key_prompt"
}

// noteHostKeyChange records a changed host key as a security event. The response
// stays 200 so the user can still re-pin, but the access line and Sentry must
// see it: this is the one case that can mean a man-in-the-middle.
func noteHostKeyChange(c echo.Context, logger *slog.Logger, host string, prompt *HostKeyPrompt) {
	if prompt == nil || !prompt.Changed {
		return
	}
	if logger != nil {
		logger.LogAttrs(c.Request().Context(), slog.LevelWarn, "host key changed",
			slog.String("host", host),
			slog.String("old_fingerprint", prompt.OldFingerprint),
			slog.String("new_fingerprint", prompt.Fingerprint),
		)
	}
	c.Set(LoggedErrorKey, gftperrors.New(gftperrors.ErrHostKeyMismatch,
		"the server's host key changed since it was trusted"))
}
