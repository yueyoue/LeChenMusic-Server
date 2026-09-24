package cloudsource

import (
	"path"
	"regexp"
	"strings"

	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/model/metadata"
)

// trackNoPrefix matches the "01 - ", "01." , "01)" style track number prefix commonly
// used in ripped/downloaded file names.
var trackNoPrefix = regexp.MustCompile(`^\s*(\d{1,3})\s*[-._)\]}]+\s*`)
var trackNoPlain = regexp.MustCompile(`^\s*(\d{1,3})\s+`)

// filenameInfo derives tags from the 艺人/专辑/曲目 folder convention of the design doc
// (docs/网盘媒体源方案设计.md §5: "解析规则按此两级父目录设计：艺人/专辑取自父目录，曲目取自文件名").
//
// It performs zero file reads and zero network calls: it is the fallback used when a tag
// can't be extracted, and the only source of truth when OpenList.<name>.TagMode="filename"
// (build the library without ever touching file contents).
func filenameInfo(p string) metadata.Info {
	p = strings.Trim(path.Clean(p), "/")
	parts := strings.Split(p, "/")
	file := parts[len(parts)-1]
	dirs := parts[:len(parts)-1]

	title := strings.TrimSuffix(file, path.Ext(file))

	tags := map[string][]string{}
	add := func(key, value string) {
		if value != "" {
			tags[key] = []string{value}
		}
	}

	if m := trackNoPrefix.FindStringSubmatch(title); m != nil {
		add("tracknumber", m[1])
		title = title[len(m[0]):]
	} else if m := trackNoPlain.FindStringSubmatch(title); m != nil {
		add("tracknumber", m[1])
		title = title[len(m[0]):]
	}
	add("title", strings.TrimSpace(title))

	switch {
	case len(dirs) >= 2:
		add("artist", dirs[len(dirs)-2])
		add("album", dirs[len(dirs)-1])
	case len(dirs) == 1:
		add("artist", dirs[0])
	}

	return metadata.Info{Tags: model.RawTags(tags)}
}
