package conf

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestApplyOpenListEnv(t *testing.T) {
	restore := SnapshotConfig()
	defer restore()
	Server.OpenList = nil

	t.Run("creates a gateway from env vars", func(t *testing.T) {
		Server.OpenList = nil
		applyOpenListEnv([]string{
			"ND_OPENLIST_NAS_URL=http://192.168.31.190:5244",
			"ND_OPENLIST_NAS_USERNAME=admin",
			"ND_OPENLIST_NAS_PASSWORD=secret",
		})
		assert.Equal(t, map[string]OpenListOptions{
			"NAS": {URL: "http://192.168.31.190:5244", Username: "admin", Password: "secret"},
		}, Server.OpenList)
	})

	t.Run("gateway names may contain underscores (field is the last segment)", func(t *testing.T) {
		Server.OpenList = nil
		applyOpenListEnv([]string{"ND_OPENLIST_MY_NAS_TOKEN=abc"})
		assert.Equal(t, map[string]OpenListOptions{
			"MY_NAS": {Token: "abc"},
		}, Server.OpenList)
	})

	t.Run("env overrides the config file field-by-field", func(t *testing.T) {
		Server.OpenList = map[string]OpenListOptions{
			"NAS": {URL: "http://old:1", Username: "old-user", Password: "old-pass"},
		}
		applyOpenListEnv([]string{
			"ND_OPENLIST_NAS_URL=http://new:2",
			"ND_OPENLIST_NAS_PASSWORD=new-pass",
		})
		assert.Equal(t, "http://new:2", Server.OpenList["NAS"].URL)
		assert.Equal(t, "new-pass", Server.OpenList["NAS"].Password)
		assert.Equal(t, "old-user", Server.OpenList["NAS"].Username) // untouched fields survive
	})

	t.Run("optional tuning fields parse", func(t *testing.T) {
		Server.OpenList = nil
		applyOpenListEnv([]string{
			"ND_OPENLIST_NAS_TAGMODE=filename",
			"ND_OPENLIST_NAS_DISABLEREDIRECT=true",
		})
		assert.Equal(t, "filename", Server.OpenList["NAS"].TagMode)
		assert.True(t, Server.OpenList["NAS"].DisableRedirect)
	})

	t.Run("empty values and unknown fields are ignored", func(t *testing.T) {
		Server.OpenList = nil
		applyOpenListEnv([]string{
			"ND_OPENLIST_NAS_URL=",
			"ND_OPENLIST_NAS_BOGUS=x",
			"ND_OPENLIST_=orphan",
			"ND_OPENLIST_NAS_DISABLEREDIRECT=notabool",
			"OTHER_VAR=1",
		})
		assert.Empty(t, Server.OpenList)
	})

	t.Run("case-insensitive prefix and field names", func(t *testing.T) {
		Server.OpenList = nil
		applyOpenListEnv([]string{"nd_openlist_nas_url=http://case:1"})
		assert.Equal(t, "http://case:1", Server.OpenList["nas"].URL)
	})
}
