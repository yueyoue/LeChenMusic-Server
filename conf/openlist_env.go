package conf

import (
	"strconv"
	"strings"

	"github.com/navidrome/navidrome/log"
)

// OpenList gateways can be configured entirely through environment variables, so
// Docker users don't have to create a navidrome.toml at all (the friendly path
// for compose deployments):
//
//	ND_OPENLIST_<NAME>_URL=<http://192.168.1.10:5244>
//	ND_OPENLIST_<NAME>_USERNAME=<user>
//	ND_OPENLIST_<NAME>_PASSWORD=<pass>          (or _TOKEN instead of user/pass)
//
// Optional fields: ND_OPENLIST_<NAME>_TAGMODE (scan|filename),
// ND_OPENLIST_<NAME>_DISABLEREDIRECT (true|false).
//
// Format: ND_OPENLIST_<gateway name>_<FIELD>. The gateway name may itself
// contain underscores — the LAST underscore separates the field name.
// Environment values override the same field coming from the config file
// (12-factor convention: env wins).
//
// Credentials still never leave the server process: they are `json:"-"` in
// OpenListOptions and never persisted to the database (design doc §8).
func applyOpenListEnv(env []string) {
	const prefix = "ND_OPENLIST_"
	for _, kv := range env {
		key, value, ok := strings.Cut(kv, "=")
		if !ok {
			continue
		}
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		key = strings.TrimSpace(key)
		if len(key) <= len(prefix) || !strings.EqualFold(key[:len(prefix)], prefix) {
			continue
		}
		rest := key[len(prefix):]
		sep := strings.LastIndex(rest, "_")
		if sep <= 0 || sep == len(rest)-1 {
			continue // no field part, e.g. ND_OPENLIST_FOO
		}
		name, field := rest[:sep], rest[sep+1:]

		if Server.OpenList == nil {
			Server.OpenList = map[string]OpenListOptions{}
		}
		opts := Server.OpenList[name]
		switch strings.ToUpper(field) {
		case "URL":
			opts.URL = value
		case "USERNAME":
			opts.Username = value
		case "PASSWORD":
			opts.Password = value
		case "TOKEN":
			opts.Token = value
		case "TAGMODE":
			opts.TagMode = value
		case "DISABLEREDIRECT":
			b, err := strconv.ParseBool(value)
			if err != nil {
				log.Warn("conf: ignoring ND_OPENLIST variable with non-boolean value", "variable", key, "value", value)
				continue
			}
			opts.DisableRedirect = b
		default:
			log.Warn("conf: ignoring unknown ND_OPENLIST variable", "variable", key)
			continue
		}
		Server.OpenList[name] = opts
	}
}
