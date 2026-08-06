package gen

import (
	"os"
	"path/filepath"
	"regexp"
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

// TestCommittedEnvExampleInSync guards the fully generated artifact: a stale
// .env.example advertises defaults the loader no longer uses.
func TestCommittedEnvExampleInSync(t *testing.T) {
	committed, err := os.ReadFile(filepath.Join(repoRoot, ".env.example"))
	require.NoError(t, err)
	assert.Equal(t, EnvExample(), string(committed), ".env.example is stale, run `just confgen`")
}

// TestCommittedDocTablesInSync checks that the doc tables list the registry's
// variables with their current defaults. The description column is deliberately
// not compared: wording in the docs is not a test failure.
func TestCommittedDocTablesInSync(t *testing.T) {
	for _, page := range DocPages {
		t.Run(page, func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join(repoRoot, page))
			require.NoError(t, err)
			injected, err := InjectTables(string(raw))
			require.NoError(t, err)
			assert.Equal(t, tableRows(injected), tableRows(string(raw)),
				"documented variables or defaults are stale, run `just confgen`")
		})
	}
}

// TestDocPagesCoverEverySection catches a dropped table: without its marker a
// whole section is simply never injected, which the row comparison cannot see.
func TestDocPagesCoverEverySection(t *testing.T) {
	documented := map[string]bool{}
	for _, page := range DocPages {
		raw, err := os.ReadFile(filepath.Join(repoRoot, page))
		require.NoError(t, err)
		for _, m := range sectionMarkerRe.FindAllStringSubmatch(string(raw), -1) {
			documented[m[1]] = true
		}
	}

	for i := range config.Registry {
		k := &config.Registry[i]
		if k.Env == "" {
			continue
		}
		assert.True(t, documented[k.Section], "section %q is undocumented (%s)", k.Section, k.Env)
	}
}

var sectionMarkerRe = regexp.MustCompile(`<!-- confgen:begin env-table "([^"]+)" -->`)

// tableRows reduces markdown table rows to variable and default. Cutting at the
// third pipe keeps pipes inside a description from shifting the columns.
func tableRows(doc string) []string {
	var rows []string
	for _, line := range strings.Split(doc, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "|") {
			continue
		}
		if cut := nthIndex(line, '|', 3); cut >= 0 {
			line = line[:cut+1]
		}
		rows = append(rows, line)
	}
	return rows
}

func nthIndex(s string, c byte, n int) int {
	for i := range len(s) {
		if s[i] == c {
			if n--; n == 0 {
				return i
			}
		}
	}
	return -1
}
