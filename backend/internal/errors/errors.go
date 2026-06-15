package errors

import "net/http"

// Code is a machine-readable error identifier sent to clients.
type Code string

const (
	ErrConnectionFailed        Code = "ERR_CONNECTION_FAILED"
	ErrAuthFailed              Code = "ERR_AUTH_FAILED"
	ErrLoginThrottled          Code = "ERR_LOGIN_THROTTLED"
	ErrFileNotFound            Code = "ERR_FILE_NOT_FOUND"
	ErrFileExists              Code = "ERR_FILE_EXISTS"
	ErrDirNotEmpty             Code = "ERR_DIR_NOT_EMPTY"
	ErrFilePermission          Code = "ERR_FILE_PERMISSION"
	ErrFileNotWritable         Code = "ERR_FILE_NOT_WRITABLE"
	ErrListFailed              Code = "ERR_LIST_FAILED"
	ErrOperationFailed         Code = "ERR_OPERATION_FAILED"
	ErrBadRequest              Code = "ERR_BAD_REQUEST"
	ErrUnauthorized            Code = "ERR_UNAUTHORIZED"
	ErrForbidden               Code = "ERR_FORBIDDEN"
	ErrInternal                Code = "ERR_INTERNAL"
	ErrNotImplemented          Code = "ERR_NOT_IMPLEMENTED"
	ErrInvalidType             Code = "ERR_INVALID_TYPE"
	ErrLoginDisabled           Code = "ERR_LOGIN_DISABLED"
	ErrCSRFInvalid             Code = "ERR_CSRF_INVALID"
	ErrSessionNotFound         Code = "ERR_SESSION_NOT_FOUND"
	ErrQuotaExceeded           Code = "ERR_QUOTA_EXCEEDED"
	ErrConnectionTimeout       Code = "ERR_CONNECTION_TIMEOUT"
	ErrPermissionsNotSupported Code = "ERR_PERMISSIONS_NOT_SUPPORTED"
	ErrUploadNotFound          Code = "ERR_UPLOAD_NOT_FOUND"
	ErrInvalidToken            Code = "ERR_INVALID_TOKEN" //nolint:gosec // G101: error code constant, not a credential
	ErrArchiveFormat           Code = "ERR_ARCHIVE_FORMAT"
	ErrFileTooLarge            Code = "ERR_FILE_TOO_LARGE"
	ErrEditorDisabled          Code = "ERR_EDITOR_DISABLED"
	ErrStorageUnavailable      Code = "ERR_STORAGE_UNAVAILABLE"
	ErrConnectionLost          Code = "ERR_CONNECTION_LOST"
	ErrHostKeyMismatch         Code = "ERR_HOST_KEY_MISMATCH"
	ErrTLSFailed               Code = "ERR_TLS_FAILED"
)

// GFTPError is a typed error with a machine-readable code and human-readable message.
// An optional cause carries the underlying error for server-side logging only —
// it is never serialized into the API response envelope.
type GFTPError struct {
	code    Code
	message string
	cause   error
}

// New creates a new GFTPError.
func New(code Code, message string) *GFTPError {
	return &GFTPError{code: code, message: message}
}

// WithCause attaches the underlying error for log enrichment and returns e (chainable).
func (e *GFTPError) WithCause(err error) *GFTPError {
	if e != nil {
		e.cause = err
	}
	return e
}

// Unwrap returns the underlying cause, if any.
func (e *GFTPError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.cause
}

// Wrap creates a GFTPError from an existing error, using its Error() string as the message.
func Wrap(code Code, err error) *GFTPError {
	if err == nil {
		return &GFTPError{code: code}
	}
	return &GFTPError{code: code, message: err.Error()}
}

// Code returns the error code.
func (e *GFTPError) Code() Code {
	if e == nil {
		return ""
	}
	return e.code
}

// Error implements the error interface.
func (e *GFTPError) Error() string {
	if e == nil {
		return ""
	}
	return e.message
}

// HTTPStatus maps the error code to an appropriate HTTP status code.
func (e *GFTPError) HTTPStatus() int {
	if e == nil {
		return http.StatusInternalServerError
	}

	switch e.code {
	case ErrBadRequest, ErrInvalidType:
		return http.StatusBadRequest
	case ErrUnauthorized, ErrAuthFailed, ErrSessionNotFound, ErrCSRFInvalid:
		return http.StatusUnauthorized
	case ErrForbidden, ErrLoginThrottled, ErrFilePermission, ErrLoginDisabled:
		return http.StatusForbidden
	case ErrFileNotFound:
		return http.StatusNotFound
	case ErrFileExists, ErrDirNotEmpty:
		return http.StatusConflict
	case ErrQuotaExceeded:
		return http.StatusInsufficientStorage
	case ErrNotImplemented:
		return http.StatusNotImplemented
	case ErrConnectionTimeout:
		return http.StatusGatewayTimeout
	case ErrPermissionsNotSupported:
		return http.StatusUnprocessableEntity
	case ErrUploadNotFound:
		return http.StatusNotFound
	case ErrInvalidToken:
		return http.StatusUnauthorized
	case ErrArchiveFormat:
		return http.StatusUnprocessableEntity
	case ErrFileTooLarge, ErrEditorDisabled:
		return http.StatusForbidden
	case ErrStorageUnavailable:
		return http.StatusServiceUnavailable
	case ErrConnectionLost, ErrHostKeyMismatch, ErrTLSFailed:
		return http.StatusBadGateway
	default:
		return http.StatusInternalServerError
	}
}
