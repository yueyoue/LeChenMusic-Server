package consts

import (
	"fmt"
	"strings"
)

var (
	// This will be set in build time. If not, version will be set to "dev"
	GitTag string
	GitSHA string
)

// Version holds the version string, with tag and git sha info.
// Examples:
// dev
// v0.2.0 (5b84188)
// v0.3.2-SNAPSHOT (715f552)
// master (9ed35cb)
var Version = func() string {
	if GitSHA == "" {
		return "dev"
	}
	tag := strings.TrimPrefix(GitTag, "v")
	return fmt.Sprintf("%s (%s)", tag, GitSHA)
}()
