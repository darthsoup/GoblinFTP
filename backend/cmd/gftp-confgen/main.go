// gftp-confgen regenerates .env.example and the marker-fenced doc tables from
// the config registry. Run via `just confgen`.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/darthsoup/goblinftp/internal/config/gen"
)

// docPages lists every file carrying confgen markers.
var docPages = []string{
	"docs/configuration.md",
	"docs/logging.md",
	"docs/metrics.md",
	"docs/s3-staging.md",
	"docs/embedding.md",
}

func run(root string) error {
	if err := os.WriteFile(filepath.Join(root, ".env.example"), []byte(gen.EnvExample()), 0o644); err != nil { //nolint:gosec // G306: a committed example file is meant to be world-readable
		return err
	}

	for _, page := range docPages {
		path := filepath.Join(root, page)
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		injected, err := gen.InjectTables(string(raw))
		if err != nil {
			return fmt.Errorf("%s: %w", page, err)
		}
		if err := os.WriteFile(path, []byte(injected), 0o644); err != nil { //nolint:gosec // G306: docs are world-readable
			return err
		}
	}
	return nil
}

func main() {
	root := flag.String("root", "..", "repo root (the directory holding .env.example and docs/)")
	flag.Parse()

	if err := run(*root); err != nil {
		fmt.Fprintln(os.Stderr, "confgen:", err)
		os.Exit(1)
	}
}
