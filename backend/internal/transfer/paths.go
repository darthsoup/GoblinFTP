package transfer

import (
	"fmt"
	"path"
	"strings"
	"unicode"
)

func ValidateRemotePath(p string, windows, allowRoot bool) error {
	if p == "" || strings.ContainsFunc(p, unicode.IsControl) || strings.HasPrefix(p, "//") || strings.HasPrefix(p, `\\`) {
		return fmt.Errorf("%w: invalid remote path", ErrInvalidType)
	}
	if windows && strings.Contains(p, `\`) {
		return fmt.Errorf("%w: Windows-style paths are not supported", ErrInvalidType)
	}
	if !path.IsAbs(p) {
		return fmt.Errorf("%w: remote path must be absolute", ErrInvalidType)
	}
	if len(strings.Split(p, "/")) > 66 {
		return ErrInvalidType
	}
	for _, seg := range strings.Split(p, "/") {
		if seg == ".." || seg == "." || (windows && HasDrivePrefix(seg)) || (windows && seg != "" && strings.TrimRight(seg, " .") != seg) {
			return fmt.Errorf("%w: ambiguous remote path segment", ErrInvalidType)
		}
	}
	if !allowRoot && path.Clean(p) == "/" {
		return fmt.Errorf("%w: refusing the remote root", ErrInvalidType)
	}
	return nil
}

func CleanDeletePath(p string) (string, error) {
	return p, ValidateRemotePath(p, true, false)
}

func ValidateChildName(name string, windows bool) error {
	separators := "/\x00\r\n"
	if windows {
		separators += `\`
	}
	if strings.ContainsFunc(name, unicode.IsControl) || name == "" || name == "." || name == ".." || strings.ContainsAny(name, separators) || (windows && (strings.TrimRight(name, " .") != name || strings.Contains(name, ":"))) {
		return fmt.Errorf("%w: invalid remote directory entry", ErrInvalidType)
	}
	return nil
}

func ComparisonPath(p string, windows bool) string {
	if windows {
		p = strings.ReplaceAll(p, `\`, "/")
		parts := strings.Split(p, "/")
		for i := range parts {
			parts[i] = strings.TrimRight(parts[i], " .")
		}
		p = strings.ToLower(strings.Join(parts, "/"))
	}
	return path.Clean(p)
}

func PathsOverlap(src, dst string, windows bool) bool {
	a, b := ComparisonPath(src, windows), ComparisonPath(dst, windows)
	return a == b || strings.HasPrefix(b, strings.TrimRight(a, "/")+"/")
}

func HasDrivePrefix(p string) bool {
	return len(p) >= 2 && p[1] == ':' && ((p[0] >= 'A' && p[0] <= 'Z') || (p[0] >= 'a' && p[0] <= 'z'))
}
