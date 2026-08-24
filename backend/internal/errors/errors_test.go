package errors_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"

	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	gftperrors "github.com/darthsoup/goblinftp/internal/errors"
)

func TestNewError(t *testing.T) {
	err := gftperrors.New(gftperrors.ErrAuthFailed, "authentication failed")
	assert.Equal(t, gftperrors.ErrAuthFailed, err.Code())
	assert.Equal(t, "authentication failed", err.Error())
}

func TestWrapError(t *testing.T) {
	underlying := fmt.Errorf("connection refused")
	err := gftperrors.Wrap(gftperrors.ErrConnectionFailed, underlying)
	assert.Equal(t, gftperrors.ErrConnectionFailed, err.Code())
	assert.Equal(t, "connection refused", err.Error())
}

func TestWrapNilError(t *testing.T) {
	err := gftperrors.Wrap(gftperrors.ErrInternal, nil)
	assert.Equal(t, gftperrors.ErrInternal, err.Code())
	assert.Equal(t, "", err.Error())
}

func TestGFTPErrorImplementsErrorInterface(t *testing.T) {
	var err error = gftperrors.New(gftperrors.ErrInternal, "oops")
	assert.Equal(t, "oops", err.Error())
}

func TestNilReceiver(t *testing.T) {
	var err *gftperrors.GFTPError
	assert.Equal(t, gftperrors.Code(""), err.Code())
	assert.Equal(t, "", err.Error())
	assert.Equal(t, http.StatusInternalServerError, err.HTTPStatus())
}

func TestHTTPStatus(t *testing.T) {
	tests := []struct {
		code     gftperrors.Code
		expected int
	}{
		{gftperrors.ErrBadRequest, http.StatusBadRequest},
		{gftperrors.ErrInvalidType, http.StatusBadRequest},
		{gftperrors.ErrUnauthorized, http.StatusUnauthorized},
		{gftperrors.ErrAuthFailed, http.StatusUnauthorized},
		{gftperrors.ErrSessionNotFound, http.StatusUnauthorized},
		{gftperrors.ErrCSRFInvalid, http.StatusUnauthorized},
		{gftperrors.ErrForbidden, http.StatusForbidden},
		{gftperrors.ErrLoginThrottled, http.StatusForbidden},
		{gftperrors.ErrFilePermission, http.StatusForbidden},
		{gftperrors.ErrLoginDisabled, http.StatusForbidden},
		{gftperrors.ErrFileNotFound, http.StatusNotFound},
		{gftperrors.ErrFileExists, http.StatusConflict},
		{gftperrors.ErrDirNotEmpty, http.StatusConflict},
		{gftperrors.ErrNotImplemented, http.StatusNotImplemented},
		{gftperrors.ErrInternal, http.StatusInternalServerError},
		{gftperrors.ErrConnectionFailed, http.StatusBadGateway},
		{gftperrors.ErrOperationFailed, http.StatusBadGateway},
		{gftperrors.ErrListFailed, http.StatusBadGateway},
		{gftperrors.ErrQuotaExceeded, http.StatusInsufficientStorage},
		{gftperrors.ErrFileTooLarge, http.StatusRequestEntityTooLarge},
		{gftperrors.ErrDataConnectionFailed, http.StatusBadGateway},
		{gftperrors.ErrTransferIncomplete, http.StatusBadGateway},
	}
	for _, tt := range tests {
		err := gftperrors.New(tt.code, "msg")
		assert.Equal(t, tt.expected, err.HTTPStatus(), "code=%s", tt.code)
	}
}

// Guards against a new Code silently defaulting to 500: every constant must have
// an explicit HTTPStatus case. Parses the source so it cannot drift.
func TestEveryCodeHasExplicitStatus(t *testing.T) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "errors.go", nil, 0)
	require.NoError(t, err)

	var body string
	{
		src, readErr := os.ReadFile("errors.go")
		require.NoError(t, readErr)
		_, after, found := strings.Cut(string(src), "func (e *GFTPError) HTTPStatus() int {")
		require.True(t, found)
		body = after
	}

	var names []string
	for _, decl := range f.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.CONST {
			continue
		}
		for _, spec := range gen.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for _, name := range vs.Names {
				if strings.HasPrefix(name.Name, "Err") {
					names = append(names, name.Name)
				}
			}
		}
	}
	require.NotEmpty(t, names)

	for _, name := range names {
		assert.True(t, strings.Contains(body, name),
			"%s has no explicit HTTPStatus case, so it silently falls through to 500", name)
	}
}
