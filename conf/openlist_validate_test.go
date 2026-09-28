package conf

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateOpenListOptions(t *testing.T) {
	restore := SnapshotConfig()
	defer restore()

	t.Run("no gateways configured is fine", func(t *testing.T) {
		Server.OpenList = nil
		assert.NoError(t, validateOpenListOptions())
	})

	t.Run("complete gateway passes", func(t *testing.T) {
		Server.OpenList = map[string]OpenListOptions{
			"nas": {URL: "http://192.168.1.10:5244", Username: "admin", Password: "secret"},
		}
		assert.NoError(t, validateOpenListOptions())

		Server.OpenList = map[string]OpenListOptions{
			"nas": {URL: "https://openlist.example.com", Token: "abc"},
		}
		assert.NoError(t, validateOpenListOptions())
	})

	t.Run("missing URL fails with the field name", func(t *testing.T) {
		Server.OpenList = map[string]OpenListOptions{
			"nas": {Username: "admin", Password: "secret"},
		}
		err := validateOpenListOptions()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "OpenList.nas")
		assert.Contains(t, err.Error(), "URL")
	})

	t.Run("malformed URL fails", func(t *testing.T) {
		for _, raw := range []string{"192.168.1.10:5244", "ftp://host", "http://"} {
			Server.OpenList = map[string]OpenListOptions{
				"nas": {URL: raw, Token: "abc"},
			}
			assert.Error(t, validateOpenListOptions(), "URL %q must be rejected", raw)
		}
	})

	t.Run("missing credentials fails", func(t *testing.T) {
		Server.OpenList = map[string]OpenListOptions{
			"nas": {URL: "http://192.168.1.10:5244"},
		}
		err := validateOpenListOptions()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "credentials")

		Server.OpenList = map[string]OpenListOptions{
			"nas": {URL: "http://192.168.1.10:5244", Username: "admin"},
		}
		assert.Error(t, validateOpenListOptions())
	})

	t.Run("invalid TagMode fails", func(t *testing.T) {
		Server.OpenList = map[string]OpenListOptions{
			"nas": {URL: "http://192.168.1.10:5244", Token: "abc", TagMode: "quick"},
		}
		err := validateOpenListOptions()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "TagMode")
	})

	t.Run("out-of-range numeric options fail", func(t *testing.T) {
		Server.OpenList = map[string]OpenListOptions{
			"nas": {URL: "http://192.168.1.10:5244", Token: "abc", JitterFraction: 2},
		}
		assert.Error(t, validateOpenListOptions())

		Server.OpenList = map[string]OpenListOptions{
			"nas": {URL: "http://192.168.1.10:5244", Token: "abc", HeadBytes: -1},
		}
		assert.Error(t, validateOpenListOptions())

		Server.OpenList = map[string]OpenListOptions{
			"nas": {URL: "http://192.168.1.10:5244", Token: "abc", MaxRetries: -1},
		}
		assert.Error(t, validateOpenListOptions())
	})

	t.Run("reports every broken gateway at once", func(t *testing.T) {
		Server.OpenList = map[string]OpenListOptions{
			"a": {Username: "u"},                 // no URL, no password
			"b": {URL: "http://x", Token: "t"}, // OK
			"c": {URL: "nonsense"},               // bad URL, no credentials
		}
		err := validateOpenListOptions()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "OpenList.a")
		assert.Contains(t, err.Error(), "OpenList.c")
		assert.NotContains(t, err.Error(), "OpenList.b:")
	})
}
