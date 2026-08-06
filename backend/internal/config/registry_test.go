package config

import (
	"math"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// clearGftpEnv unsets every GFTP_* variable via the documented "empty means
// unset" semantics; unlike a hand-maintained list, it can never go stale.
func clearGftpEnv(t *testing.T) {
	t.Helper()
	for _, kv := range os.Environ() {
		if name, _, ok := strings.Cut(kv, "="); ok && strings.HasPrefix(name, "GFTP_") {
			t.Setenv(name, "")
		}
	}
	for _, k := range Registry {
		t.Setenv(k.Env, "")
	}
}

func TestRegistryShape(t *testing.T) {
	envSeen := map[string]bool{}
	spaSeen := map[string]bool{}
	for _, k := range Registry {
		require.NotEmpty(t, k.Env, "key without an env name")
		assert.False(t, envSeen[k.Env], "duplicate env name %s", k.Env)
		envSeen[k.Env] = true
		assert.True(t, strings.HasPrefix(k.Env, "GFTP_"), "%s must carry the GFTP_ prefix", k.Env)

		if k.SPA {
			assert.NotEmpty(t, k.SPAPath, "%s: SPA keys need an explicit JSON path", k.Env)
			assert.False(t, spaSeen[k.SPAPath], "duplicate SPA path %s", k.SPAPath)
			spaSeen[k.SPAPath] = true
			assert.False(t, k.Secret, "%s: secrets must not be exposed to the SPA", k.Env)
		}
		assert.NotEmpty(t, k.Doc, "%s: doc line required (generated artifacts depend on it)", k.Env)
		assert.NotEmpty(t, k.Section, "%s: section required", k.Env)
		assert.NotEmpty(t, k.DocPage, "%s: doc page required", k.Env)
		assert.NotNil(t, k.render, "%s: render required", k.Env)
	}
}

// matrixCase carries the per-kind sample values for the generic loader matrix.
type matrixCase struct {
	valid   string // env encoding of a non-default value
	want    string // expected rendered form after applying valid
	invalid []string
}

func matrixFor(t *testing.T, k *Key, def *Config) (matrixCase, bool) {
	t.Helper()
	var m matrixCase
	switch k.kind {
	case "secret", "custom":
		return m, false
	case "bool":
		neg := strconv.FormatBool(k.render(def) != "true")
		m = matrixCase{valid: neg, want: neg, invalid: []string{"banana"}}
	case "string", "optString":
		v := "from-env"
		if k.matchRe != nil {
			v = k.sampleValid
			m.invalid = []string{k.sampleInvalid}
		}
		m.valid, m.want = v, v
	case "enum":
		require.GreaterOrEqual(t, len(k.enumAllowed), 2, "%s: enum needs two values for the matrix", k.Env)
		defVal := k.render(def)
		for _, v := range k.enumAllowed {
			if v != defVal {
				m.valid, m.want = v, v
				break
			}
		}
		require.NotEmpty(t, m.valid, "%s: enum needs a non-default value", k.Env)
		m.invalid = []string{"zzz-not-a-value"}
	case "int", "int64", "optInt":
		lo := k.intMin
		if lo == math.MinInt64 {
			lo = 0
		}
		m.valid = strconv.FormatInt(lo+3, 10)
		m.want = m.valid
		m.invalid = []string{"abc"}
		if k.intMin > math.MinInt64 {
			m.invalid = append(m.invalid, strconv.FormatInt(k.intMin-1, 10))
		}
	case "float":
		lo, hi := k.floatMin, k.floatMax
		require.False(t, math.IsInf(lo, -1) || math.IsInf(hi, 1), "%s: float matrix assumes a bounded range", k.Env)
		m.valid = strconv.FormatFloat(lo+(hi-lo)/4, 'g', -1, 64)
		m.want = m.valid
		m.invalid = []string{"abc", strconv.FormatFloat(hi+1, 'g', -1, 64)}
	case "port":
		m = matrixCase{valid: "9301", want: "9301", invalid: []string{"abc", "0", "70000"}}
	case "list":
		vals := []string{"alpha"}
		if k.listAllowed != nil {
			vals = []string{k.listAllowed[0]}
			m.invalid = []string{"banana"}
		}
		m.valid = strings.Join(vals, ",")
		m.want = m.valid
	default:
		t.Fatalf("unknown kind %q", k.kind)
	}
	return m, true
}

// companionEnv satisfies cross-key validation while the matrix exercises a
// single key (an enabled toggle needs its required counterparts).
var companionEnv = map[string]map[string]string{
	"GFTP_SSO_ENABLED": {"GFTP_SSO_SECRET": "matrix-secret"},
	"GFTP_S3_ENABLED": {
		"GFTP_S3_ENDPOINT":   "http://localhost:9000",
		"GFTP_S3_BUCKET":     "matrix-bucket",
		"GFTP_S3_ACCESS_KEY": "matrix-access",
		"GFTP_S3_SECRET_KEY": "matrix-secret",
	},
	"GFTP_CONNECTION_LOCK_HOST": {"GFTP_CONNECTION_PRESET_HOST": "ftp.example.com"},
}

// TestRegistryMatrix drives every non-secret key through the loader: default,
// env value, and invalid env values naming the offending key.
func TestRegistryMatrix(t *testing.T) {
	defaults := defaultConfig()
	for i := range Registry {
		k := &Registry[i]
		m, ok := matrixFor(t, k, defaults)
		if !ok {
			continue
		}

		t.Run(k.Env, func(t *testing.T) {
			t.Run("default", func(t *testing.T) {
				clearGftpEnv(t)
				cfg, err := Load(nil)
				require.NoError(t, err)
				assert.Equal(t, k.render(defaults), k.render(cfg))
			})

			t.Run("env", func(t *testing.T) {
				clearGftpEnv(t)
				t.Setenv(k.Env, m.valid)
				for env, val := range companionEnv[k.Env] {
					t.Setenv(env, val)
				}
				cfg, err := Load(nil)
				require.NoError(t, err)
				assert.Equal(t, m.want, k.render(cfg))
			})

			for _, invalid := range m.invalid {
				t.Run("invalid "+invalid, func(t *testing.T) {
					clearGftpEnv(t)
					t.Setenv(k.Env, invalid)
					_, err := Load(nil)
					require.Error(t, err)
					assert.Contains(t, err.Error(), k.Env)
				})
			}
		})
	}
}
