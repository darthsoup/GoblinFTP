package transfer

import (
	"fmt"
	"path"
	"strings"
)

func CleanDeletePath(p string) (string, error) {
	if !path.IsAbs(p) || strings.ContainsAny(p, "\\\x00\r\n") {
		return "", fmt.Errorf("%w: deletion requires an absolute remote path", ErrInvalidType)
	}
	cleaned := path.Clean(p)
	if cleaned == "/" {
		return "", fmt.Errorf("%w: refusing to delete the remote root", ErrInvalidType)
	}
	return cleaned, nil
}
