package gen

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/darthsoup/goblinftp/internal/config"
)

const repoRoot = "../../../.."

func TestEnvExampleShape(t *testing.T) {
	out := EnvExample()
	assert.Equal(t, out, EnvExample(), "rendering must be deterministic")
	assert.True(t, strings.HasSuffix(out, "\n"), "single trailing newline")
	assert.NotContains(t, out, "\r")

	for _, k := range config.Registry {
		assert.Contains(t, out, "#"+k.Env+"=", "every env key appears commented out")
	}
	for _, line := range strings.Split(out, "\n") {
		assert.False(t, strings.HasPrefix(line, "GFTP_"),
			"no live assignments in the example: %q", line)
	}
}

func TestInjectTablesErrors(t *testing.T) {
	_, err := InjectTables("<!-- confgen:begin env-table \"Server\" -->\nno end")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing end marker")

	_, err = InjectTables("<!-- confgen:begin env-table \"Nope\" -->\n<!-- confgen:end -->")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Nope")

	_, err = InjectTables("<!-- confgen:begin bogus -->\n<!-- confgen:end -->")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown confgen marker")
}

func TestInjectTablesReplacesBlockContent(t *testing.T) {
	doc := "before\n<!-- confgen:begin env-table \"Server\" -->\nstale row\n<!-- confgen:end -->\nafter"
	out, err := InjectTables(doc)
	require.NoError(t, err)
	assert.NotContains(t, out, "stale row")
	assert.Contains(t, out, "`GFTP_PORT`")
	assert.True(t, strings.HasPrefix(out, "before\n"))
	assert.True(t, strings.HasSuffix(out, "\nafter"))

	again, err := InjectTables(out)
	require.NoError(t, err)
	assert.Equal(t, out, again, "injection must be idempotent")
}

// TestCommittedArtifactsInSync is the local drift gate: committed artifacts
// must match what the registry renders; fix with `just confgen`.
func TestCommittedArtifactsInSync(t *testing.T) {
	committed, err := os.ReadFile(filepath.Join(repoRoot, ".env.example"))
	require.NoError(t, err)
	assert.Equal(t, EnvExample(), string(committed), ".env.example is stale — run `just confgen`")

	for _, page := range []string{
		"docs/configuration.md", "docs/logging.md", "docs/metrics.md",
		"docs/s3-staging.md", "docs/embedding.md", "docs/migration-0.24.md",
	} {
		raw, err := os.ReadFile(filepath.Join(repoRoot, page))
		require.NoError(t, err)
		injected, err := InjectTables(string(raw))
		require.NoError(t, err, page)
		assert.Equal(t, injected, string(raw), "%s tables are stale — run `just confgen`", page)
	}
}
